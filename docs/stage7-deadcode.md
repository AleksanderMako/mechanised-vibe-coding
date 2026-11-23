# Stage 7: deadcode

One package that finds unreachable functions in a module and, more usefully,
the functions a change made unreachable: run the analysis at two git
revisions and diff the results. It does in process what
`golang.org/x/tools/cmd/deadcode` does, so no download happens at run time.

Works on a module checkout given by `Root`. Shells out to `git` only, to
create temporary worktrees for the two revisions.

---

## Package deadcode

### Exported API

```go
package deadcode

// Func is one function or method.
type Func struct {
	Package string // import path
	Name    string // "Name" or "Type.Method" (the receiver type without pointer)
	File    string // path relative to the module root
	Line    int
}

// Key is the stable identity of a Func across revisions: Package and Name.
func (f Func) Key() string

// Options controls the analysis.
type Options struct {
	Patterns []string // package patterns relative to Root; empty means ./...
	Tests    bool     // treat test functions as roots too, so helpers only tests use are live
}

// Dead lists the functions of the loaded packages that no root reaches. Roots are
// main and init of the main packages, and the tests when Options.Tests; other
// packages are reached only through their importers. Sorted by Package, then File, then Line.
func Dead(ctx context.Context, root string, opts Options) ([]Func, error)

// Diff is what changed between two revisions.
type Diff struct {
	Before, After []Func // the full lists
	NewlyDead     []Func // dead after and not dead before, by Key
	Revived       []Func // dead before and not dead after, by Key
}

// Orphaned runs Dead at two git revisions, in temporary worktrees, and diffs the results.
// afterRef may be "" for the working tree, including uncommitted changes.
func Orphaned(ctx context.Context, root, beforeRef, afterRef string, opts Options) (Diff, error)

// ErrNoRoots is wrapped when the loaded packages contain no main package and no tests to start from.
var ErrNoRoots = errors.New("no main packages or tests to use as roots")
```

### Behaviour

`Dead`:

1. `Root` without `go.mod` is an ordinary error. Packages are loaded with `golang.org/x/tools/go/packages` including tests when `Tests`, built into SSA with `golang.org/x/tools/go/ssa/ssautil`, and reachability is computed with `golang.org/x/tools/go/callgraph/rta` from the roots.
2. Roots are `main.main` and `init` of every main package among the loaded packages, plus, when `Tests`, every `Test*`, `Benchmark*`, `Fuzz*` and `Example*` function declared in `_test.go` files and the generated test main. The `init` of a non-main package is not a root: it is reached only through an importer, exactly as `golang.org/x/tools/cmd/deadcode` does it, so a package that no root imports is entirely dead, including whatever its `init` would have run (proto type registration, for example). An `init` declaration is listed when nothing reaches it, which happens when no root imports its package. Its `Name` is the SSA name `init#N`, where N counts the package's `init` declarations from 1 in source order. A `func _` declaration can never be called and is always listed, with `Name` `_`.
3. No `main.main` and no test root wraps `ErrNoRoots`, even when the packages have `init` functions; this is what a library module without tests gives. A pattern matching no packages is an ordinary error.
4. Load or type errors are ordinary errors naming the first one when `Options.Strict`; otherwise stage 9 (`docs/stage9-tolerance.md`) applies.
5. A function is dead when it is declared in source in one of the loaded packages (not a dependency, not synthetic, not a generic instantiation) and is not reachable. With `Tests`, the package and its test variant are the same package: a function reached in either copy is live, a dead one is listed once with the plain import path. Methods are reported with `Name` `Type.Method`. Anonymous functions are reported through their enclosing function only: an unreachable enclosing function is listed, its literals are not. Marker methods are never listed: an unexported method with no parameters, no results and an empty body whose receiver type implements some named interface type declared in the same package. This is the marker rule of `cmd/deadcode`; it hides protoc's `isOneof_Field` and `mustEmbedUnimplemented...Server` methods, which exist only to satisfy an interface. Of several copies of one declaration (package and test variant), the first one reached by walking the loaded packages with `packages.Visit` is the one checked for the marker rule.
6. Generic functions count as reachable when any instantiation is reachable.
7. `File` is relative to `Root` (symlinks resolved) with forward slashes, `Line` is the line of the `func` keyword. Ties in sorting are broken by `Name`.

`Orphaned`:

8. `beforeRef` must resolve with `git rev-parse --verify`; an invalid ref is an ordinary error. It is checked out with `git worktree add --detach <tmp> <ref>` under the system temporary directory and removed with `git worktree remove --force` afterwards, also on error.
9. `afterRef` empty means `Root` itself is analysed as it stands. Non-empty is handled like `beforeRef`.
10. `Dead` runs at both with the same `Options`. `NewlyDead` and `Revived` are computed on `Key()` and sorted like `Dead` output. `File` and `Line` in those slices come from the revision where the function is dead.
11. A function dead in both revisions appears in neither delta.

### Reference tool

12. `golang.org/x/tools/cmd/deadcode` (v0.53.0) is the oracle. On any module, the set of `Package`, `Name`, `File` and `Line` that `Dead` returns equals what `deadcode -generated ./...` prints (and `deadcode -generated -test ./...` for `Options.Tests`), after mapping the reference's `pkg/file.go:LINE:COL: unreachable func: Name` lines to the same fields. `Dead` always includes generated files; the reference hides them unless `-generated` is given. The equivalence holds for `./...` only: with narrower `Patterns`, `Dead` reports just the matched packages (rule 5) while the reference also reports their dependencies inside the module.
13. The test suite contains an opt-in oracle test: when the environment variable `MVC_DEADCODE_REFERENCE` holds the path of a `deadcode` binary, the test runs it on every fixture module with `-generated` (and `-test` where the fixture is also analysed with `Tests`) and fails on any difference with `Dead`. Without the variable the test is skipped, so the ordinary suite needs no download.

### Out of scope

Unreachable types, variables or constants, deleting code, reporting on dependencies, reflection based reachability beyond what rta models.
