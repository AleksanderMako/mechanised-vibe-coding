# Stage 2: verify

One package that runs the pre-push checks in a fixed order and stops at the
first failure, finds and fixes vendor drift, scopes lint to the packages that
changed, and hunts flaky tests by running one test many times.

Works on a module checkout given by `Root`. Never uses the current working
directory. Shells out to `go`, `gofmt` and `git` only, with the inherited
environment.

---

## Package verify

### Exported API

```go
package verify

// StepName identifies one check.
type StepName string

const (
	StepFormat StepName = "gofmt"  // gofmt -l on the package directories
	StepVendor StepName = "vendor" // vendor drift check and fix
	StepBuild  StepName = "build"  // go build
	StepVet    StepName = "vet"    // go vet
	StepTest   StepName = "test"   // go test
	StepLint   StepName = "lint"   // golangci-lint via go run
)

// Order is the fixed order the steps run in.
var Order = []StepName{StepFormat, StepVendor, StepBuild, StepVet, StepTest, StepLint}

// Options controls Run. Root is required.
type Options struct {
	Root        string     // module root
	Packages    []string   // package patterns relative to Root such as "./lib/foo"; empty means the packages of changed files
	LintVersion string     // golangci-lint version such as "v2.12.2"; empty skips the lint step
	Race        bool       // pass -race to go test
	AllTests    bool       // test ./... instead of Packages
	Skip        []StepName // steps to skip
}

// Step is the outcome of one check.
type Step struct {
	Name     StepName
	Skipped  bool          // not run: listed in Skip, lint without a version, or after an earlier failure
	OK       bool          // ran and passed; false when Skipped
	Duration time.Duration // zero when Skipped
	Output   string        // combined output; empty when passed with nothing to say
}

// Report is the outcome of Run.
type Report struct {
	Packages []string // the package patterns the steps ran on, sorted
	Steps    []Step   // one per entry of Order, in that order
}

// OK is true when no step failed.
func (r Report) OK() bool

// Failed returns the first failed step, or nil.
func (r Report) Failed() *Step

// Run executes the steps in Order on the packages and stops at the first failure.
// Check failures live in the Report; the error is only for setup problems such
// as a missing Root or a missing go tool.
func Run(ctx context.Context, opts Options) (Report, error)

// ChangedPackages lists the package patterns of .go files changed against HEAD:
// staged, unstaged and untracked files, as "./relative/dir", sorted and unique,
// excluding directories under testdata or vendor. A clean tree returns an empty slice.
func ChangedPackages(ctx context.Context, root string) ([]string, error)

// SyncVendor fixes vendor drift. Without a vendor directory it returns false.
// With one, it runs `go mod vendor` when the module graph and vendor/modules.txt
// disagree, and reports whether it ran.
func SyncVendor(ctx context.Context, root string) (changed bool, err error)

// FlakeReport is the outcome of Flaky.
type FlakeReport struct {
	Runs     int    // runs requested
	Failures int    // runs that failed
	Output   string // the go test output of the first failing run, or empty
}

// Flaky runs one test many times to see whether it is stable.
func Flaky(ctx context.Context, root, pkg, test string, runs int, race bool) (FlakeReport, error)
```

### Behaviour

Setup, in `Run`:

1. `Root` empty or not a directory containing `go.mod` is an ordinary error.
2. With `Packages` empty, `ChangedPackages` is used. When that is also empty, the packages are `["./..."]`.
3. `Report.Packages` holds the patterns used, sorted and unique.
4. `Report.Steps` always has one entry per element of `Order`, in that order, even when steps are skipped.

Steps:

5. A step named in `Skip` has `Skipped true`, `OK false`, zero `Duration`, empty `Output`, and does not stop the run.
6. After the first failed step every later step is `Skipped true`.
7. `StepFormat` runs `gofmt -l` on the directories of the packages. For `./...` the whole root is listed except `vendor` and `testdata` directories. Any listed file is a failure with `Output` holding the file paths relative to `Root`, one per line, sorted.
8. `StepVendor` calls `SyncVendor`. It passes. When it changed something, `Output` is `vendor synced`. An error from `SyncVendor` fails the step with the error text as `Output`.
9. `StepBuild` runs `go build` with the patterns. Non-zero exit fails with the combined output.
10. `StepVet` runs `go vet` with the patterns. Non-zero exit fails with the combined output.
11. `StepTest` runs `go test` with the patterns, or with `./...` when `AllTests`, adding `-race` when `Race`. Non-zero exit fails with the combined output. Passing keeps `Output` empty.
12. `StepLint` is `Skipped` when `LintVersion` is empty. Otherwise it runs `go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@<LintVersion> run <patterns>` from `Root`. Non-zero exit fails with the combined output.
13. Every step that runs has `Duration` greater than zero.
14. A cancelled context stops the current step with a failure whose `Output` names the cancellation and skips the rest; `Run` returns `ctx.Err()` as well.

`ChangedPackages`:

15. The root must be inside a git work tree; otherwise an ordinary error.
16. Changed files come from `git status --porcelain --untracked-files=all` and deleted files are included: their directory is reported when it still exists.
17. Only `.go` files count. The directory is relative to `Root` and formatted as `./dir`, or `.` for files in the root itself.
18. Directories with a path element `testdata` or `vendor` are excluded. Results are sorted and unique.

`SyncVendor`:

19. No `vendor` directory under `Root` returns `false, nil` without running anything.
20. Drift is detected by running `go list -m all` from `Root`; an exit with output containing `inconsistent vendoring` or a missing `vendor/modules.txt` is drift. Any other failure is an error.
21. On drift `go mod vendor` runs; a non-zero exit is an error with the output. Success returns `true, nil`.

`Flaky`:

22. `runs` below 1 is an error. `pkg` is a pattern relative to `Root`. `test` is a test name used as `-run ^<test>$`; an empty `test` runs every test of the package.
23. The command is `go test -count=<runs> -run ^<test>$ [-race] <pkg>` from `Root`, so the test runs `runs` times in one process.
24. `Failures` counts the lines starting with `--- FAIL: <test>` exactly matching the test name (with the sub-test separator excluded, so sub-test failures do not double count); with empty `test` it counts every top-level `--- FAIL:` line (no slash in the name).
25. `Output` is the full combined go test output when `Failures` is above zero, and empty otherwise.
26. A failing build of the test package is an error, not a flake count.

### Out of scope

Running on multiple modules, fixing formatting, posting results anywhere, caching.
