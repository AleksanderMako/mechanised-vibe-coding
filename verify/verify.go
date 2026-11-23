// Package verify runs the pre-push checks in a fixed order, stopping at the
// first failure, and hunts flaky tests by repeating one test many times.
package verify

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"time"
)

// StepName identifies one check.
type StepName string

// The steps, in the names Run reports them under.
const (
	StepFormat StepName = "gofmt"
	StepVendor StepName = "vendor"
	StepBuild  StepName = "build"
	StepVet    StepName = "vet"
	StepTest   StepName = "test"
	StepLint   StepName = "lint"
)

// Order is the fixed order the steps run in.
var Order = []StepName{StepFormat, StepVendor, StepBuild, StepVet, StepTest, StepLint}

// Options controls Run. Root is required.
type Options struct {
	Root        string     // module root
	Packages    []string   // package patterns relative to Root; empty means the packages of changed files
	LintVersion string     // golangci-lint version such as "v2.12.2"; empty skips lint
	Race        bool       // pass -race to go test
	AllTests    bool       // test ./... instead of Packages
	Skip        []StepName // steps to skip
}

// Step is the outcome of one check.
type Step struct {
	Name     StepName
	Skipped  bool          // not run
	OK       bool          // ran and passed
	Duration time.Duration // zero when Skipped
	Output   string        // combined output; empty when passed with nothing to say
}

// Report is the outcome of Run.
type Report struct {
	Packages []string // the package patterns the steps ran on, sorted
	Steps    []Step   // one per entry of Order, in that order
}

// OK is true when no step failed.
func (r Report) OK() bool { return r.Failed() == nil }

// Failed returns the first failed step, or nil.
func (r Report) Failed() *Step {
	for index := range r.Steps {
		if !r.Steps[index].Skipped && !r.Steps[index].OK {
			return &r.Steps[index]
		}
	}
	return nil
}

// lintModule is the golangci-lint command run through go run.
const lintModule = "github.com/golangci/golangci-lint/v2/cmd/golangci-lint"

// Run executes the steps in Order and stops at the first failure.
func Run(ctx context.Context, opts Options) (Report, error) {
	report := Report{Steps: skippedSteps()}
	if err := checkSetup(opts.Root); err != nil {
		return report, err
	}
	packages, err := resolvePackages(ctx, opts)
	if err != nil {
		return report, err
	}
	report.Packages = packages

	stopped := false
	for index, name := range Order {
		if stopped || slices.Contains(opts.Skip, name) || name == StepLint && opts.LintVersion == "" {
			continue
		}
		step := runStep(ctx, name, opts, packages)
		report.Steps[index] = step
		if !step.OK {
			stopped = true
			if ctx.Err() != nil {
				return report, ctx.Err()
			}
		}
	}
	return report, nil
}

// skippedSteps returns one skipped step per entry of Order, which Run then overwrites as steps run.
func skippedSteps() []Step {
	steps := make([]Step, len(Order))
	for index, name := range Order {
		steps[index] = Step{Name: name, Skipped: true}
	}
	return steps
}

// checkSetup rejects a root without go.mod and a machine without the go tool.
func checkSetup(root string) error {
	if root == "" {
		return fmt.Errorf("verify: root is empty")
	}
	info, err := os.Stat(filepath.Join(root, "go.mod"))
	if err != nil || info.IsDir() {
		return fmt.Errorf("verify: %s does not contain go.mod", root)
	}
	if _, err := exec.LookPath("go"); err != nil {
		return fmt.Errorf("verify: go tool not found: %w", err)
	}
	return nil
}

// resolvePackages picks the explicit patterns, else the changed packages, else ./...
func resolvePackages(ctx context.Context, opts Options) ([]string, error) {
	packages := sortedUnique(opts.Packages)
	if len(packages) > 0 {
		return packages, nil
	}
	changed, err := ChangedPackages(ctx, opts.Root)
	if err != nil {
		if ctx.Err() != nil {
			// Let the first step report the cancellation like any other cancelled step.
			return []string{"./..."}, nil
		}
		return nil, err
	}
	if len(changed) == 0 {
		return []string{"./..."}, nil
	}
	return changed, nil
}

// runStep runs one check and times it.
func runStep(ctx context.Context, name StepName, opts Options, packages []string) Step {
	started := time.Now()
	ok, output := checkStep(ctx, name, opts, packages)
	if ctx.Err() != nil {
		ok, output = false, "cancelled: "+ctx.Err().Error()
	}
	duration := time.Since(started)
	if duration <= 0 {
		// A coarse clock can report zero, yet a step that ran must show it took time.
		duration = time.Nanosecond
	}
	return Step{Name: name, OK: ok, Duration: duration, Output: output}
}

// checkStep performs the work of one step and returns whether it passed and what it has to say.
func checkStep(ctx context.Context, name StepName, opts Options, packages []string) (bool, string) {
	if ctx.Err() != nil {
		return false, ""
	}
	switch name {
	case StepFormat:
		files, failed, output, err := unformattedFiles(ctx, opts.Root, packages)
		if err != nil {
			return false, err.Error()
		}
		if failed {
			return false, output
		}
		if len(files) > 0 {
			return false, strings.Join(files, "\n")
		}
		return true, ""
	case StepVendor:
		changed, err := SyncVendor(ctx, opts.Root)
		if err != nil {
			return false, err.Error()
		}
		if changed {
			return true, "vendor synced"
		}
		return true, ""
	case StepBuild:
		return goStep(ctx, opts.Root, append([]string{"build"}, packages...))
	case StepVet:
		return goStep(ctx, opts.Root, append([]string{"vet"}, packages...))
	case StepTest:
		args := []string{"test"}
		if opts.Race {
			args = append(args, "-race")
		}
		if opts.AllTests {
			args = append(args, "./...")
		} else {
			args = append(args, packages...)
		}
		ok, output := goStep(ctx, opts.Root, args)
		if ok {
			// A passing test run lists every package, which is noise.
			output = ""
		}
		return ok, output
	case StepLint:
		args := append([]string{"run", lintModule + "@" + opts.LintVersion, "run"}, packages...)
		return goStep(ctx, opts.Root, args)
	}
	return false, fmt.Sprintf("unknown step %q", name)
}

// goStep runs the go tool and fails on a non-zero exit with the combined output.
func goStep(ctx context.Context, root string, args []string) (bool, string) {
	result, err := runCommand(ctx, root, "go", args...)
	if err != nil {
		return false, strings.TrimSpace(result.output + "\n" + err.Error())
	}
	if result.exitCode != 0 {
		return false, result.output
	}
	return true, strings.TrimSpace(result.output)
}
