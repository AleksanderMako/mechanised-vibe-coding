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
// the main functions and init functions of the loaded packages, and the tests when
// Options.Tests. Sorted by Package, then File, then Line.
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
2. Roots are every `main.main` and every `init` of the loaded packages, plus, when `Tests`, every `Test*`, `Benchmark*`, `Fuzz*` and `Example*` function of the test packages, and the generated test main.
3. No roots at all wraps `ErrNoRoots`; this is what a library module without tests gives.
4. Load or type errors are ordinary errors naming the first one.
5. A function is dead when it is declared in source in one of the loaded packages (not a dependency, not synthetic, not a generic instantiation) and is not reachable. Methods are reported with `Name` `Type.Method`. Anonymous functions are reported through their enclosing function only: an unreachable enclosing function is listed, its literals are not.
6. Generic functions count as reachable when any instantiation is reachable.
7. `File` is relative to `Root`, `Line` is the line of the `func` keyword.

`Orphaned`:

8. `beforeRef` must resolve with `git rev-parse --verify`; an invalid ref is an ordinary error. It is checked out with `git worktree add --detach <tmp> <ref>` under the system temporary directory and removed with `git worktree remove --force` afterwards, also on error.
9. `afterRef` empty means `Root` itself is analysed as it stands. Non-empty is handled like `beforeRef`.
10. `Dead` runs at both with the same `Options`. `NewlyDead` and `Revived` are computed on `Key()` and sorted like `Dead` output. `File` and `Line` in those slices come from the revision where the function is dead.
11. A function dead in both revisions appears in neither delta.

### Out of scope

Unreachable types, variables or constants, deleting code, reporting on dependencies, reflection based reachability beyond what rta models.
