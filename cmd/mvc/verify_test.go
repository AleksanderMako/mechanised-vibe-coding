package main_test

import (
	"regexp"
	"strings"
	"testing"

	"github.com/AleksanderMako/machanised-vibe-coding/verify"
)

// verifyStepLine is rule 5's NAME\tOK|FAIL|SKIP\tDURATION.
var verifyStepLine = regexp.MustCompile(`^\S+\t(OK|FAIL|SKIP)\t\S+$`)

// unformattedFile is valid Go that gofmt would rewrite, so the gofmt step fails.
const unformattedFile = "package bad\n\nfunc  Bad( ) int {\nreturn 1\n}\n"

// stepLines returns the stdout lines that match the step pattern, in order.
func stepLines(output string) []string {
	var lines []string
	for _, line := range strings.Split(output, "\n") {
		if verifyStepLine.MatchString(line) {
			lines = append(lines, line)
		}
	}
	return lines
}

// expectStepOrder checks one step line per entry of verify.Order, in that order.
func expectStepOrder(t *testing.T, result runResult) []string {
	t.Helper()
	lines := stepLines(result.stdout)
	if len(lines) != len(verify.Order) {
		t.Fatalf("want %d step lines, got %d:\n%s", len(verify.Order), len(lines), result.describe())
	}
	for index, name := range verify.Order {
		if !strings.HasPrefix(lines[index], string(name)+"\t") {
			t.Errorf("line %d = %q, want step %s", index, lines[index], name)
		}
	}
	return lines
}

// Rules 4 and 5: a passing run exits 0; text and JSON both list every step.
func TestVerifyRunPasses(t *testing.T) {
	module := copyModule(t)

	textResult := runMVC(t, module, "verify", "run", "-root", module, "-pkg", "./...")
	expectExit(t, textResult, 0)
	lines := expectStepOrder(t, textResult)
	if len(nonEmptyLines(textResult.stdout)) != len(verify.Order) {
		t.Errorf("a passing run prints only the step lines:\n%s", textResult.describe())
	}
	if !strings.HasPrefix(lines[len(lines)-1], "lint\tSKIP\t") {
		t.Errorf("lint must be skipped without -lint, got %q", lines[len(lines)-1])
	}

	jsonResult := runMVC(t, module, "verify", "run", "-root", module, "-pkg", "./...", "-json")
	expectExit(t, jsonResult, 0)
	var report verify.Report
	decodeJSON(t, jsonResult, &report)
	if len(report.Steps) != len(verify.Order) || !report.OK() {
		t.Errorf("report = %+v, want %d steps and OK", report, len(verify.Order))
	}
}

// Rules 3 and 5: a failed step exits 1 and its output follows the step lines verbatim.
func TestVerifyRunFailedStep(t *testing.T) {
	module := copyModule(t)
	writeFile(t, module, "bad/bad.go", unformattedFile)

	textResult := runMVC(t, module, "verify", "run", "-root", module, "-pkg", "./...")
	expectExit(t, textResult, 1)
	lines := expectStepOrder(t, textResult)
	if !strings.HasPrefix(lines[0], "gofmt\tFAIL\t") {
		t.Errorf("gofmt step = %q, want FAIL", lines[0])
	}
	for _, line := range lines[1:] {
		if !strings.Contains(line, "\tSKIP\t") {
			t.Errorf("steps after the failure must be skipped, got %q", line)
		}
	}
	failLine := strings.Index(textResult.stdout, lines[0])
	if !strings.Contains(textResult.stdout[failLine:], "bad/bad.go") {
		t.Errorf("failed step output naming bad/bad.go does not follow:\n%s", textResult.describe())
	}

	jsonResult := runMVC(t, module, "verify", "run", "-root", module, "-pkg", "./...", "-json")
	expectExit(t, jsonResult, 1)
	var report verify.Report
	decodeJSON(t, jsonResult, &report)
	if failed := report.Failed(); failed == nil || failed.Name != verify.StepFormat {
		t.Errorf("failed step = %+v, want gofmt", failed)
	}
}

// Rule 2: repeated -skip flags accumulate.
func TestVerifyRunRepeatedSkip(t *testing.T) {
	module := copyModule(t)
	writeFile(t, module, "bad/bad.go", unformattedFile)

	result := runMVC(t, module, "verify", "run", "-root", module, "-pkg", "./...",
		"-skip", "gofmt", "-skip", "test", "-json")
	expectExit(t, result, 0)
	var report verify.Report
	decodeJSON(t, result, &report)
	for _, step := range report.Steps {
		wantSkipped := step.Name == verify.StepFormat || step.Name == verify.StepTest || step.Name == verify.StepLint
		if step.Skipped != wantSkipped {
			t.Errorf("step %s skipped = %v, want %v", step.Name, step.Skipped, wantSkipped)
		}
	}
}

// Rule 2: repeated -pkg flags accumulate into the report's packages.
func TestVerifyRunRepeatedPkg(t *testing.T) {
	module := copyModule(t)
	result := runMVC(t, module, "verify", "run", "-root", module, "-pkg", "./alpha", "-pkg", "./beta", "-json")
	expectExit(t, result, 0)
	var report verify.Report
	decodeJSON(t, result, &report)
	if strings.Join(report.Packages, ",") != "./alpha,./beta" {
		t.Errorf("packages = %v, want [./alpha ./beta]", report.Packages)
	}
}

// Rule 1: -root defaults to the current directory.
func TestVerifyRunDefaultRoot(t *testing.T) {
	module := copyModule(t)
	result := runMVC(t, module, "verify", "run", "-pkg", "./...")
	expectExit(t, result, 0)
	expectStepOrder(t, result)
}

// Rule 4: flaky -json prints a verify.FlakeReport.
func TestVerifyFlakyJSON(t *testing.T) {
	module := copyModule(t)
	result := runMVC(t, module, "verify", "flaky", "-root", module, "-pkg", "./shapes", "-test", "TestArea", "-runs", "2", "-json")
	expectExit(t, result, 0)
	var report verify.FlakeReport
	decodeJSON(t, result, &report)
	if report.Runs != 2 || report.Failures != 0 {
		t.Errorf("report = %+v, want 2 runs and no failures", report)
	}
}
