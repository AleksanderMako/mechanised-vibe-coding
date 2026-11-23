package verify

import (
	"context"
	"fmt"
	"strconv"
	"strings"
)

// FlakeReport is the outcome of Flaky.
type FlakeReport struct {
	Runs     int    // runs requested
	Failures int    // runs that failed
	Output   string // go test output when something failed
}

// Flaky runs one test many times in one process to see whether it is stable.
// pkg is a pattern relative to root; an empty test runs every test of the package.
func Flaky(ctx context.Context, root, pkg, test string, runs int, race bool) (FlakeReport, error) {
	if runs < 1 {
		return FlakeReport{}, fmt.Errorf("verify: runs must be at least 1, got %d", runs)
	}
	if root == "" {
		return FlakeReport{}, fmt.Errorf("verify: root is empty")
	}
	args := []string{"test", "-count=" + strconv.Itoa(runs)}
	if test != "" {
		args = append(args, "-run", "^"+test+"$")
	}
	if race {
		args = append(args, "-race")
	}
	args = append(args, pkg)
	result, err := runCommand(ctx, root, "go", args...)
	if err != nil {
		return FlakeReport{}, fmt.Errorf("verify: go test: %w", err)
	}
	report := FlakeReport{Runs: runs, Failures: countFailures(result.output, test)}
	if report.Failures > 0 {
		report.Output = result.output
		return report, nil
	}
	// A non-zero exit without any failed test means the package did not build or set up.
	if result.exitCode != 0 {
		return FlakeReport{}, fmt.Errorf("verify: go test did not run the tests:\n%s", result.output)
	}
	return report, nil
}

// countFailures counts top-level "--- FAIL: <name>" lines; sub-test lines never count.
func countFailures(output, test string) int {
	const prefix = "--- FAIL: "
	failures := 0
	for _, line := range strings.Split(output, "\n") {
		if !strings.HasPrefix(line, prefix) {
			continue
		}
		name, _, _ := strings.Cut(strings.TrimPrefix(line, prefix), " ")
		if (test == "" && !strings.Contains(name, "/")) || (test != "" && name == test) {
			failures++
		}
	}
	return failures
}
