package main

import (
	"context"
	"flag"
	"fmt"
	"time"

	"github.com/AleksanderMako/machanised-vibe-coding/verify"
)

var verifyGroup = group{
	name:    "verify",
	summary: "the pre-push loop and flaky test hunting (package verify)",
	actions: []action{
		{
			name:     "run",
			synopsis: "-root DIR [-pkg P]... [-lint VERSION] [-race] [-all-tests] [-skip STEP]... [-json]",
			help: `Runs gofmt, vendor, build, vet, test and lint in that order, stopping at
the first failure. Each step prints as NAME<TAB>OK|FAIL|SKIP<TAB>DURATION and
the failed step's output follows verbatim. A failed step exits 1; with -json
the verify.Report is printed, still exit 1.`,
			setup: setupVerifyRun,
		},
		{
			name:     "flaky",
			synopsis: "-root DIR -pkg P [-test NAME] [-runs N] [-race] [-json]",
			help: `Runs one test (or every test of P) N times in one process.
Prints runs=N<TAB>failures=M, then the go test output verbatim when a run
failed. Any failure exits 1; with -json the verify.FlakeReport, still exit 1.`,
			setup: setupVerifyFlaky,
		},
		{
			name:     "changed",
			synopsis: "-root DIR [-json]",
			help: `Lists the package patterns of .go files changed against HEAD, one per line.
With -json a JSON array of strings.`,
			setup: setupVerifyChanged,
		},
		{
			name:     "vendor",
			synopsis: "-root DIR",
			help: `Runs go mod vendor when vendor/ drifted from the module graph.
Prints "synced" when it ran, "unchanged" when vendor was in sync or absent.`,
			setup: setupVerifyVendor,
		},
	},
}

func setupVerifyRun(flags *flag.FlagSet) func(context.Context, []string) error {
	root := addRootFlag(flags)
	var packages, skip listFlag
	flags.Var(&packages, "pkg", "package pattern `P` relative to -root (repeatable); none means the changed packages")
	lintVersion := flags.String("lint", "", "golangci-lint `VERSION` such as v2.12.2; empty skips lint")
	race := flags.Bool("race", false, "pass -race to go test")
	allTests := flags.Bool("all-tests", false, "test ./... instead of the packages")
	flags.Var(&skip, "skip", "`STEP` to skip: gofmt, vendor, build, vet, test or lint (repeatable)")
	asJSON := addJSONFlag(flags)
	return func(ctx context.Context, args []string) error {
		if len(args) != 0 {
			return usageErrorf("unexpected arguments: %v", args)
		}
		absolute, err := absRoot(*root)
		if err != nil {
			return err
		}
		var skipSteps []verify.StepName
		for _, name := range skip {
			skipSteps = append(skipSteps, verify.StepName(name))
		}
		report, err := verify.Run(ctx, verify.Options{
			Root:        absolute,
			Packages:    packages,
			LintVersion: *lintVersion,
			Race:        *race,
			AllTests:    *allTests,
			Skip:        skipSteps,
		})
		if err != nil {
			return err
		}
		if *asJSON {
			if err := printJSON(report); err != nil {
				return err
			}
		} else {
			printSteps(report.Steps)
		}
		if !report.OK() {
			return errNegative
		}
		return nil
	}
}

// printSteps prints NAME, OK|FAIL|SKIP and DURATION per step, with a failed step's output after it.
func printSteps(steps []verify.Step) {
	for _, step := range steps {
		status := "FAIL"
		switch {
		case step.Skipped:
			status = "SKIP"
		case step.OK:
			status = "OK"
		}
		fmt.Printf("%s\t%s\t%s\n", step.Name, status, shortDuration(step.Duration))
		if status == "FAIL" {
			printVerbatim(step.Output)
		}
	}
}

// shortDuration rounds to milliseconds for readability but keeps sub-millisecond steps non-zero.
func shortDuration(d time.Duration) time.Duration {
	if d < time.Millisecond {
		return d
	}
	return d.Round(time.Millisecond)
}

func setupVerifyFlaky(flags *flag.FlagSet) func(context.Context, []string) error {
	root := addRootFlag(flags)
	pkg := flags.String("pkg", "", "package pattern `P` relative to -root")
	testName := flags.String("test", "", "test `NAME`; empty runs every test of the package")
	runs := flags.Int("runs", 10, "number of runs `N`")
	race := flags.Bool("race", false, "pass -race to go test")
	asJSON := addJSONFlag(flags)
	return func(ctx context.Context, args []string) error {
		if len(args) != 0 {
			return usageErrorf("unexpected arguments: %v", args)
		}
		if err := requireFlags(flags, "pkg"); err != nil {
			return err
		}
		absolute, err := absRoot(*root)
		if err != nil {
			return err
		}
		report, err := verify.Flaky(ctx, absolute, *pkg, *testName, *runs, *race)
		if err != nil {
			return err
		}
		if *asJSON {
			if err := printJSON(report); err != nil {
				return err
			}
		} else {
			fmt.Printf("runs=%d\tfailures=%d\n", report.Runs, report.Failures)
			printVerbatim(report.Output)
		}
		if report.Failures > 0 {
			return errNegative
		}
		return nil
	}
}

func setupVerifyChanged(flags *flag.FlagSet) func(context.Context, []string) error {
	root := addRootFlag(flags)
	asJSON := addJSONFlag(flags)
	return func(ctx context.Context, args []string) error {
		if len(args) != 0 {
			return usageErrorf("unexpected arguments: %v", args)
		}
		absolute, err := absRoot(*root)
		if err != nil {
			return err
		}
		packages, err := verify.ChangedPackages(ctx, absolute)
		if err != nil {
			return err
		}
		if *asJSON {
			return printJSON(packages)
		}
		printLines(packages)
		return nil
	}
}

func setupVerifyVendor(flags *flag.FlagSet) func(context.Context, []string) error {
	root := addRootFlag(flags)
	return func(ctx context.Context, args []string) error {
		if len(args) != 0 {
			return usageErrorf("unexpected arguments: %v", args)
		}
		absolute, err := absRoot(*root)
		if err != nil {
			return err
		}
		changed, err := verify.SyncVendor(ctx, absolute)
		if err != nil {
			return err
		}
		if changed {
			fmt.Println("synced")
		} else {
			fmt.Println("unchanged")
		}
		return nil
	}
}
