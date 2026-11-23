// Package verify runs the pre-push checks in a fixed order, stopping at the
// first failure, and hunts flaky tests by repeating one test many times.
package verify

import (
	"context"
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
func (r Report) OK() bool { panic("not implemented") }

// Failed returns the first failed step, or nil.
func (r Report) Failed() *Step { panic("not implemented") }

// Run executes the steps in Order and stops at the first failure.
func Run(ctx context.Context, opts Options) (Report, error) { panic("not implemented") }

// ChangedPackages lists the "./dir" patterns of .go files changed against HEAD.
func ChangedPackages(ctx context.Context, root string) ([]string, error) {
	panic("not implemented")
}

// SyncVendor runs go mod vendor when the vendor directory drifted from go.mod.
func SyncVendor(ctx context.Context, root string) (changed bool, err error) {
	panic("not implemented")
}

// FlakeReport is the outcome of Flaky.
type FlakeReport struct {
	Runs     int    // runs requested
	Failures int    // runs that failed
	Output   string // go test output when something failed
}

// Flaky runs one test many times in one process to see whether it is stable.
func Flaky(ctx context.Context, root, pkg, test string, runs int, race bool) (FlakeReport, error) {
	panic("not implemented")
}
