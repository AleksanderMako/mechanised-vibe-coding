# Stage 1: editset and astedit

Two packages that make editing Go source deterministic. `editset` is text
level: a list of old-text to new-text replacements across files that either
all land or none do. `astedit` is structural: edits addressed by symbol name,
applied on a comment preserving syntax tree, so the edit cannot hit the wrong
function and the anchor never goes stale.

Both packages work on a module checkout given by its root directory. They
never use the current working directory. Paths in the API are either absolute
or relative to that root.

---

## Package editset

### Purpose

An agent proposing edits writes them as (file, old, new) triples. Today each
triple is applied one at a time, a mismatch half way leaves the tree in a
mixed state, and gofmt, import cleanup and a build are separate manual steps.
`editset.Apply` does the whole thing as one transaction.

### Exported API

```go
package editset

// Edit replaces exactly one occurrence of Old in File with New.
type Edit struct {
	File string // absolute, or relative to Options.Root
	Old  string // must occur exactly once in the file at the time the edit is applied
	New  string // may be empty, which deletes Old
}

// Options controls Apply. The zero value is not usable: Root is required.
type Options struct {
	Root     string // module root; relative Edit.File paths resolve against it
	NoFormat bool   // skip goimports on touched .go files
	NoBuild  bool   // skip building the packages of touched .go files
}

// Result reports what Apply changed.
type Result struct {
	Touched  []string // files written, absolute paths, in first-edit order, no duplicates
	Packages []string // package directories built, absolute paths, sorted; empty when NoBuild or no .go file touched
}

// Problem is one edit that cannot be applied.
type Problem struct {
	Index  int    // position in the edits slice
	File   string // absolute path
	Reason string // one of the Reason* constants
	Count  int    // occurrences of Old found; meaningful for ReasonNotUnique and ReasonNotFound
}

const (
	ReasonMissingFile = "file does not exist or cannot be read"
	ReasonEmptyOld    = "old text is empty"
	ReasonNotFound    = "old text not found"
	ReasonNotUnique   = "old text occurs more than once"
	ReasonOutsideRoot = "file is outside the root"
)

// CheckError is returned when at least one edit cannot be applied. Nothing was written.
type CheckError struct {
	Problems []Problem // every failing edit, in index order
}

func (e *CheckError) Error() string

// BuildError is returned when the touched packages do not build after the edits.
// Every touched file has been restored to its content before Apply.
type BuildError struct {
	Packages []string // package directories that failed, absolute paths
	Output   string   // combined go build output
}

func (e *BuildError) Error() string

// Check validates the edits without writing anything. It returns nil when every
// edit would apply, otherwise a *CheckError listing every problem.
func Check(edits []Edit, opts Options) error

// Apply validates all edits, then writes every touched file, then runs goimports on
// the touched .go files, then builds the packages that contain touched .go files.
// On a validation failure nothing is written and the error is a *CheckError.
// On a build failure every touched file is restored and the error is a *BuildError.
// Any other error (I/O, go tool missing) is returned as is and touched files are restored.
func Apply(ctx context.Context, edits []Edit, opts Options) (Result, error)

// ParseJSON reads edits from JSON: an array of objects with keys "file", "old", "new".
// Unknown keys are an error. "new" may be omitted and means delete.
func ParseJSON(r io.Reader) ([]Edit, error)
```

### Behaviour

Validation, in `Check` and at the start of `Apply`:

1. `Options.Root` empty, or not an existing directory, is an ordinary error, not a `CheckError`.
2. Each `Edit.File` is made absolute against `Root`. A path that, after cleaning, is not inside `Root` is `ReasonOutsideRoot`.
3. The file is read once per distinct path. Unreadable is `ReasonMissingFile`.
4. Edits are checked in slice order. For each edit, `Old` must occur exactly once in the file content as it stands after the earlier edits to the same file have been applied in memory. Zero occurrences is `ReasonNotFound` with `Count 0`, more than one is `ReasonNotUnique` with the count. Empty `Old` is `ReasonEmptyOld`.
5. Every edit is checked even after a failure, so `CheckError.Problems` is complete.
6. Matching is exact byte matching. No whitespace normalisation, no regular expressions.

Writing, in `Apply` after validation passed:

7. Every touched file is written with its final in memory content. File mode is preserved.
8. Unless `NoFormat`, each touched file ending in `.go` is passed through `golang.org/x/tools/imports.Process` with default options and written again if the result differs. A formatting error (the edited file does not parse) is treated like a build failure, even with `NoBuild`: files are restored and a `*BuildError` is returned with every parse error joined in `Output` and the failing files' package directories, sorted and unique, in `Packages`.
9. Unless `NoBuild`, the directories holding touched `.go` files are built from `Root` with the inherited environment: one `go build -o /dev/null ./dir ...` call for the directories without a touched `_test.go` file (so a main package never leaves a binary behind), and one `go vet ./dir ...` call for the directories with one, so test files are compiled too. A non-zero exit is a `*BuildError` with the full combined output and the directories of the failing call listed; the output cannot be attributed to a single directory.
10. Restoration means writing back the exact original bytes of every touched file. Files that did not exist cannot be touched (validation rejects them), so no file is ever deleted.
11. `Result.Touched` lists every written file once, in the order of the first edit that touched it. `Result.Packages` lists the built directories sorted.
12. A cancelled context stops before writing if cancellation is observed during validation, and is passed to the `go` command otherwise.

`ParseJSON`:

13. Input is a JSON array. Each element is an object with `file` and `old` as strings and optional `new`. A missing `file` or `old` key, a wrong type, an unknown key or a non-array top level is an error naming the element index. An empty array is valid and returns an empty slice.

### Out of scope

Regular expressions, fuzzy matching, creating or deleting files, edits outside the module root, running tests.

---

## Package astedit

### Purpose

The edits that recur during a refactoring have stable structural targets: a
new case in a table test, a field on a struct, a doc comment, a constant, a
compile time conformance line. Addressing them by symbol instead of by exact
bytes removes the mismatches and lets the edit never land in the wrong place.
`github.com/dave/dst` is used so comments survive.

### Exported API

```go
package astedit

// Op is one structural edit on one Go file. Ops are built with the constructors
// below and applied with Apply. The interface is closed: only this package implements it.
type Op interface {
	// File is the path the op edits, as given to the constructor.
	File() string
	// Describe is a one line human description, for logs.
	Describe() string
	apply(f *dst.File) error
}

// Sentinel errors. Errors returned by Apply wrap one of these and name the op.
var (
	ErrNotFound  = errors.New("target not found")
	ErrAmbiguous = errors.New("more than one target matches")
	ErrExists    = errors.New("target already exists")
	ErrBadSource = errors.New("source snippet does not parse")
)

// AddTableCase appends caseSrc as a new element of the table in testFunc.
// The table is the single composite literal of slice-of-struct type in the body
// of testFunc, either assigned with := or = or ranged over directly.
// caseSrc is one composite literal element, for example `{name: "x", want: 1}`.
func AddTableCase(file, testFunc, caseSrc string) Op

// AddStructField inserts fieldSrc into struct type typeName. fieldSrc is one field
// line, for example "Name string `json:\"name\"`". With after empty the field is
// appended; otherwise it is inserted after the existing field named after.
func AddStructField(file, typeName, fieldSrc, after string) Op

// RemoveStructField removes the field named fieldName from struct type typeName.
func RemoveStructField(file, typeName, fieldName string) Op

// SetDoc replaces the doc comment of symbol with doc. symbol is "Name" for a
// top level func, type, var or const, or "Type.Method" for a method. doc is plain
// text, one or more lines separated by newlines, without comment markers. An
// empty doc removes the doc comment.
func SetDoc(file, symbol, doc string) Op

// SetConst replaces the value of the top level constant name with valueSrc, a Go
// expression. When doc is not empty the doc comment is replaced as in SetDoc.
func SetConst(file, name, valueSrc, doc string) Op

// AddConformance adds `var _ Iface = (*TypeName)(nil)` directly after the type
// declaration of typeName. ifaceSrc is the interface as written in that file,
// for example "consumers.Consumer" or "io.Closer". When the same line already
// exists anywhere in the file the op is a no-op and the file is not written.
func AddConformance(file, typeName, ifaceSrc string) Op

// Apply loads each file once, applies its ops in order on the tree, then writes
// every changed file formatted with gofmt. All ops are applied in memory before
// any file is written, so a failing op leaves every file untouched. Paths are
// absolute or relative to root. The returned slice holds the files written,
// absolute, in first-op order.
func Apply(ctx context.Context, root string, ops ...Op) ([]string, error)
```

### Behaviour

General:

1. Files are parsed with `dst` via `decorator.ParseFile` with comments. Changed files are printed with `decorator.Fprint` and then formatted with `go/format.Source`. Imports are not managed; a snippet that needs a new import is the caller's responsibility and the result still builds only if the import exists.
2. A file that does not parse is an ordinary error naming the file.
3. Errors from a failing op wrap the sentinel and contain `Op.Describe()`.
4. The written file ends with exactly one newline.
5. Each op addresses a top level declaration. Nested types, local structs inside functions and methods on embedded fields are not searched except as the contract states for `AddTableCase`.

`AddTableCase`:

6. The target function is the top level `func testFunc(...)`; a method is not a target. Missing is `ErrNotFound`.
7. Inside the function body, including nested blocks, the candidate tables are composite literals whose type is written as `[]struct{...}`, `[]Name` or `[]pkg.Name` with a non-predeclared element type (so `[]string{...}` arguments are not tables), or `map[...]struct{...}`, on the right hand side of a `:=` or `=` statement, or as the `range` expression directly. Exactly one candidate must exist; zero is `ErrNotFound`, more is `ErrAmbiguous`.
8. `caseSrc` must parse as exactly one composite literal element: it is wrapped over several lines as `_ = T{\n<caseSrc>,\n}` with the comma placed after the last token and before any trailing comment, so a case ending in `// comment` is fine. Anything but one element, or a non-literal element for a slice table, is `ErrBadSource`. For a map table the element is a `key: value` pair.
9. The new element is appended as the last element on its own line with its comments preserved; every existing element is put on its own line too, so a one-line table becomes multi-line.

`AddStructField`:

10. `typeName` must name a top level `type X struct{...}`. Missing is `ErrNotFound`. A type that is not a struct is `ErrNotFound`.
11. `fieldSrc` must parse as one struct field (parsed inside `struct{ <fieldSrc> }`), with a name, type and optional tag, possibly with a trailing line comment. Failure is `ErrBadSource`. Embedded fields (no name) are allowed.
12. Checks run in this order: the snippet parses (`ErrBadSource`), the name is new (`ErrExists`), `after` exists (`ErrNotFound`).

`RemoveStructField`:

13. Same type lookup as 10. A missing field is `ErrNotFound`. Removing the field also removes its doc and line comments. In a multi-name field `A, B int` only the name is removed and the line stays. A struct field list left empty is written as `struct{}`.

`SetDoc`:

14. `symbol` resolves to a top level `func`, a `type`, or a `var`/`const` spec, or to a method `Type.Method` with pointer or value receiver. Missing is `ErrNotFound`. For a `var`/`const` in a grouped declaration the doc of the spec is set; for a single declaration the doc of the declaration is set.
15. Each line of `doc` becomes `// ` plus the line; an empty line becomes `//`. Existing doc comment lines are removed first. Comments that are not doc comments (line comments after the symbol, comments inside the body) are kept.
16. An empty `doc` removes the doc comment and is not an error when there was none. A `SetDoc` that changes nothing (same doc, or empty doc with none present) is a no-op and the file is not written. More than one matching symbol, such as several `init` functions, is `ErrAmbiguous`. `//go:` and `//line` directives in the doc block are kept after the new doc.

`SetConst`:

17. `name` must be a top level constant, grouped or single. A `var` with that name is `ErrNotFound`. A spec declaring several names at once (`const a, b = 1, 2`) is `ErrAmbiguous`.
18. `valueSrc` must parse as an expression; failure is `ErrBadSource`. A constant with no value (implicit iota repetition) gets the value added.
19. `doc` non-empty follows rules 15. `doc` empty leaves the doc comment as it is.

`AddConformance`:

20. `typeName` must be a top level type declaration of any kind. Missing is `ErrNotFound`. An `ifaceSrc` that does not parse as a type is `ErrBadSource`.
21. The inserted declaration is `var _ <ifaceSrc> = (*<typeName>)(nil)` as its own top level `var` directly after the type's declaration group, with a blank line before and after.
22. No-op detection compares against every existing top level `var _ ... = ...` spec after formatting whitespace away: same interface text and same `(*Type)(nil)` value. On no-op the file is not written and not listed in the result.

`Apply`:

23. Ops are grouped by absolute file path, preserving order within a file. Every file is parsed and all its ops are applied in memory first. The first failing op stops everything with its error and nothing is written.
24. A file whose ops were all no-ops is not written.
25. The returned slice lists written files in the order of the first op that targets them.

### Out of scope

Import management, renaming, edits inside function bodies other than the table case, non-Go files, formatting of untouched files.
