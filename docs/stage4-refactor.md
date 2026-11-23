# Stage 4: refactor

One package of type aware source rewrites over a module: rename a symbol,
move declarations to another package, and turn a method whose receiver is
unused into a function. Every operation rewrites all references including
tests, writes all files or none, runs goimports on the touched files, builds
the touched packages, and restores everything when the build fails.

Built on `symbols` (stage 3) for resolution and `golang.org/x/tools/go/packages`
for type information. Works on a module checkout given by `Root`.

---

## Package refactor

### Exported API

```go
package refactor

// Options controls loading and the final build.
type Options struct {
	Patterns []string // package patterns relative to Root; empty means ./...
	NoBuild  bool     // skip the build after writing
}

// Sentinel errors, wrapped in the returned errors.
var (
	ErrConflict        = errors.New("name already in use")
	ErrReceiverUsed    = errors.New("receiver is used in the method body")
	ErrUnsupportedUse  = errors.New("a use of the symbol cannot be rewritten")
	ErrLeftBehind      = errors.New("moved code depends on unexported symbols left behind")
	ErrBreaksInterface = errors.New("the type would stop satisfying an interface")
	ErrBadName         = errors.New("not a valid Go identifier")
)

// Rename renames sym everywhere it is declared and used. Returns the files written, absolute, sorted.
func Rename(ctx context.Context, root string, sym symbols.Symbol, newName string, opts Options) ([]string, error)

// Move moves top level declarations to the package at toImportPath, into the file
// toFile (relative to that package's directory), rewriting every importer.
func Move(ctx context.Context, root string, syms []symbols.Symbol, toImportPath, toFile string, opts Options) ([]string, error)

// Demethodize turns method Type.Method into the package level function newName
// (the method name when empty) and rewrites every call site.
func Demethodize(ctx context.Context, root string, method symbols.Symbol, newName string, opts Options) ([]string, error)
```

### Behaviour

Common:

1. `Root` without `go.mod` is an ordinary error. Resolution failures wrap `symbols.ErrNotFound`. Load or type errors in the module are handled as stage 9 (`docs/stage9-tolerance.md`) describes; with `Options.Strict` they are ordinary errors and nothing is written.
2. All rewrites are computed in memory first. Then every changed file is written, needed imports are added by the tool itself (goimports resolves module packages from the process working directory, which the tools never rely on), `golang.org/x/tools/imports.Process` is run on each to format and drop unused imports, and unless `NoBuild` the directories of the written files are built with `go build -o /dev/null` (or `go vet` when a `_test.go` file is among them) from `Root`. Only those directories are built, so a breakage in an importer that was not itself edited is not caught. A failing build restores every written file to its original bytes, recreates deleted files, removes created files and directories, and the error is a `*editset.BuildError`.
3. The returned slice holds the written files as absolute paths, sorted, and is complete even when the build later fails (the error carries the failure). Deleted files are not listed.
4. Doc comments move with their declaration and the first word of a doc comment that equals the old name is rewritten to the new name.

`Rename`:

5. `newName` must be a valid Go identifier that is not a keyword and not `_`, else wraps `ErrBadName`. Equal to the current name is a no-op returning an empty slice. Renaming an embedded field wraps `ErrUnsupportedUse`.
6. Every identifier whose object is the symbol's object, declaration included, is replaced in place. Struct fields are renamed in composite literal keys and selectors too. Methods are renamed on the declaration and every selector; a method implementing an interface method from a loaded package renames only the type's method and wraps `ErrBreaksInterface` when the type then stops implementing an interface that is used as a type somewhere in the loaded packages (an assignment, conversion or declaration of that interface type referencing the concrete type).
7. Conflicts wrap `ErrConflict`: for package level symbols, `newName` already declared at package level in the same package or in any file scope of it (an import name counts), or a predeclared identifier of that name used anywhere in the package; for methods and fields, `newName` already in the type's method set or field list; for any symbol, a reference site where `newName` is shadowed by a local declaration between the reference and the package scope.
8. Renaming an exported symbol to an unexported name is `ErrConflict` when a reference exists in another package.
9. Identifiers in comments and strings are not touched except rule 4.

`Move`:

10. Every symbol in `syms` must be a package level declaration (`Name` without a dot) in one and the same package, the source package; mixing packages is an ordinary error. An empty `syms` is an ordinary error.
11. `toImportPath` must be inside the module. When its directory does not exist it is created and `toFile` is created with the package clause named after the last path element. When it exists, `toFile` is created or appended to; the package name is the existing one.
12. The moved declarations are removed from their files in the source package together with their doc comments; a moved type takes its methods along, since Go keeps methods in the type's package. A single spec lifted out of a `var`/`const`/`type` group gets its own keyword; lifting one spec out of a const group that uses `iota` or implicit values, or one name out of a multi-name spec, wraps `ErrUnsupportedUse`. A source file left with no declarations is deleted. Imports that became unused are removed by goimports.
13. Each moved declaration is checked for identifiers resolving to unexported package level symbols, fields or methods of the source package that are not themselves moved; any such dependency wraps `ErrLeftBehind` and the error names them.
14. Moved unexported symbols, and unexported fields and methods of moved types, that are referenced from outside the destination package after the move are renamed to their exported form (first letter upper cased) and that rename is applied everywhere; a resulting clash in the destination wraps `ErrConflict`. Moved symbols that are not referenced outside the destination keep their name.
15. Every reference to a moved symbol outside the destination package is rewritten to the qualified form with the destination package name, and the destination import is added by goimports. References inside the destination package drop their qualifier. References inside the source package become qualified.
16. A symbol already declared in the destination with the same name wraps `ErrConflict`. Moving into a package that the moved code imports creates an import cycle; this is caught by the build, so with `NoBuild` it is the caller's risk.

`Demethodize`:

17. `method` must resolve to a method declared in the loaded packages. The receiver identifier, when named, must not be used in the body; a use wraps `ErrReceiverUsed`. A blank or unnamed receiver passes.
18. `newName` defaults to the method name and must be a valid identifier, else wraps `ErrBadName`. A package level declaration with that name in the method's package wraps `ErrConflict`. A method of a generic type wraps `ErrUnsupportedUse`.
19. The declaration loses its receiver and becomes `func newName(params) results`.
20. Every call `expr.Method(args)` whose selector resolves to this method becomes `newName(args)` in the method's package and `pkg.newName(args)` elsewhere, with the receiver expression dropped. A receiver expression with side effects (a call, a channel receive, an index expression) wraps `ErrUnsupportedUse` because dropping it would change behaviour; identifiers, selectors of identifiers, and pointer dereferences are fine. Dropping the receiver can leave a local variable unused; the build then fails and the operation is restored with a `*editset.BuildError`, which is the intended outcome, not a bug.
21. A method value or method expression (`x.Method` not called, `T.Method`, `(*T).Method`) wraps `ErrUnsupportedUse`.
22. When the type satisfies an interface in the loaded packages that has this method and that interface is used as a type anywhere in the loaded packages, the operation wraps `ErrBreaksInterface`.
23. When `newName` is unexported and a call site is in another package, `newName` is exported automatically (first letter upper cased) and the error is not raised; the returned files reflect that name.

### Out of scope

Renaming packages, moving methods, moving files, changing signatures, gopls.
