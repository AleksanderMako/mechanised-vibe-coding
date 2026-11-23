package verify_test

import (
	"context"
	"strings"
	"testing"

	"github.com/AleksanderMako/machanised-vibe-coding/verify"
)

// flakyTests is written into the flaky fixture at run time; -count=N reuses one process, so the counter persists.
const flakyTests = `package flaky

import "testing"

var flakyRuns int

func TestFlaky(t *testing.T) {
	flakyRuns++
	if flakyRuns%2 == 1 {
		t.Fatalf("odd run %d fails", flakyRuns)
	}
}

func TestStable(t *testing.T) {}

func TestParent(t *testing.T) {
	t.Run("child", func(t *testing.T) {
		t.Fatal("the child always fails")
	})
}

func TestAlwaysFails(t *testing.T) {
	t.Fatal("always fails")
}
`

// decoyTests prints a FAIL line for a longer name so only exact name matching counts it right.
const decoyTests = `package flaky

import (
	"fmt"
	"testing"
)

func TestQuiet(t *testing.T) {
	fmt.Println("--- FAIL: TestQuietly (0.00s)")
	t.Fatal("quiet fails")
}
`

// flakyFixture copies the flaky module and writes the given test file into its package.
func flakyFixture(t *testing.T, testSource string) string {
	t.Helper()
	root := copyFixture(t, "flaky")
	writeFile(t, root, "flaky/flaky_test.go", testSource)
	return root
}

func TestRule22_RunsBelowOneIsAnError(t *testing.T) {
	t.Parallel()
	root := flakyFixture(t, flakyTests)
	for _, runs := range []int{0, -1} {
		if _, err := verify.Flaky(context.Background(), root, "./flaky", "TestStable", runs, false); err == nil {
			t.Errorf("Flaky with runs %d: want an error, got nil", runs)
		}
	}
}

func TestRule23_24_FlakyTestCountsEveryFailingRun(t *testing.T) {
	t.Parallel()
	root := flakyFixture(t, flakyTests)
	report, err := verify.Flaky(context.Background(), root, "./flaky", "TestFlaky", 4, false)
	if err != nil {
		t.Fatalf("Flaky: %v", err)
	}
	if report.Runs != 4 {
		t.Errorf("Runs: want 4, got %d", report.Runs)
	}
	if report.Failures != 2 {
		t.Errorf("Failures: want 2 of 4 runs, got %d\n%s", report.Failures, report.Output)
	}
}

func TestRule23_25_StableTestHasNoFailuresAndNoOutput(t *testing.T) {
	t.Parallel()
	root := flakyFixture(t, flakyTests)
	report, err := verify.Flaky(context.Background(), root, "./flaky", "TestStable", 5, false)
	if err != nil {
		t.Fatalf("Flaky: %v", err)
	}
	if report.Runs != 5 {
		t.Errorf("Runs: want 5, got %d", report.Runs)
	}
	if report.Failures != 0 {
		t.Errorf("Failures: want 0, got %d", report.Failures)
	}
	if report.Output != "" {
		t.Errorf("Output: want empty when nothing failed, got %q", report.Output)
	}
}

func TestRule24_SubTestFailuresDoNotDoubleCount(t *testing.T) {
	t.Parallel()
	root := flakyFixture(t, flakyTests)
	report, err := verify.Flaky(context.Background(), root, "./flaky", "TestParent", 3, false)
	if err != nil {
		t.Fatalf("Flaky: %v", err)
	}
	if report.Failures != 3 {
		t.Errorf("Failures: want 3 (one per run, sub-test lines excluded), got %d\n%s", report.Failures, report.Output)
	}
}

func TestRule24_NameMustMatchExactly(t *testing.T) {
	t.Parallel()
	root := flakyFixture(t, decoyTests)
	report, err := verify.Flaky(context.Background(), root, "./flaky", "TestQuiet", 2, false)
	if err != nil {
		t.Fatalf("Flaky: %v", err)
	}
	if report.Failures != 2 {
		t.Errorf("Failures: want 2 (the TestQuietly line must not count), got %d\n%s", report.Failures, report.Output)
	}
}

func TestRule22_24_EmptyTestNameRunsEveryTestAndCountsTopLevelFailures(t *testing.T) {
	t.Parallel()
	root := flakyFixture(t, flakyTests)
	report, err := verify.Flaky(context.Background(), root, "./flaky", "", 2, false)
	if err != nil {
		t.Fatalf("Flaky: %v", err)
	}
	// Per 2 runs: TestFlaky fails once, TestParent twice, TestAlwaysFails twice; TestParent/child lines do not count.
	if report.Failures != 5 {
		t.Errorf("Failures: want 5 top-level FAIL lines, got %d\n%s", report.Failures, report.Output)
	}
}

func TestRule25_OutputHoldsTheGoTestOutputWhenSomethingFailed(t *testing.T) {
	t.Parallel()
	root := flakyFixture(t, flakyTests)
	report, err := verify.Flaky(context.Background(), root, "./flaky", "TestFlaky", 2, false)
	if err != nil {
		t.Fatalf("Flaky: %v", err)
	}
	if report.Failures != 1 {
		t.Errorf("Failures: want 1, got %d", report.Failures)
	}
	for _, fragment := range []string{"--- FAIL: TestFlaky", "odd run 1 fails"} {
		if !strings.Contains(report.Output, fragment) {
			t.Errorf("Output should contain %q, got:\n%s", fragment, report.Output)
		}
	}
}

func TestRule23_RaceFlagEnablesTheRaceDetector(t *testing.T) {
	t.Parallel()
	root := flakyFixture(t, strings.Replace(racyTestSource, "package racy", "package flaky", 1))

	report, err := verify.Flaky(context.Background(), root, "./flaky", "TestRacy", 1, false)
	if err != nil {
		t.Fatalf("Flaky without race: %v", err)
	}
	if report.Failures != 0 {
		t.Errorf("without race: want 0 failures, got %d\n%s", report.Failures, report.Output)
	}

	report, err = verify.Flaky(context.Background(), root, "./flaky", "TestRacy", 1, true)
	if err != nil {
		t.Fatalf("Flaky with race: %v", err)
	}
	if report.Failures != 1 {
		t.Errorf("with race: want 1 failure, got %d\n%s", report.Failures, report.Output)
	}
	if !strings.Contains(report.Output, "DATA RACE") {
		t.Errorf("with race: Output should report the data race, got:\n%s", report.Output)
	}
}

func TestRule26_BuildFailureIsAnErrorNotAFlake(t *testing.T) {
	t.Parallel()
	root := flakyFixture(t, "package flaky\n\nimport \"testing\"\n\nfunc TestBroken(t *testing.T) { undefinedHelper() }\n")
	if _, err := verify.Flaky(context.Background(), root, "./flaky", "TestBroken", 3, false); err == nil {
		t.Error("Flaky on a package that does not build: want an error, got nil")
	}
}
