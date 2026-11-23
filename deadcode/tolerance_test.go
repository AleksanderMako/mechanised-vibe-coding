package deadcode_test

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/AleksanderMako/machanised-vibe-coding/deadcode"
)

// Import paths of the stage 9 tolerance fixture packages.
const (
	toleranceAppPackage       = "example.com/fixture/app"
	toleranceLibPackage       = "example.com/fixture/lib"
	toleranceStalePackage     = "example.com/fixture/stale"
	toleranceDependentPackage = "example.com/fixture/dependent"
)

// The exact line rule 12 gives a package excluded because it imports stale.
const dependentIgnoredLine = "deadcode: ignoring package " + toleranceDependentPackage + ": it imports broken package " + toleranceStalePackage

// staleIgnoredPrefix starts rule 12's line for the broken package itself; its first error follows.
const staleIgnoredPrefix = "deadcode: ignoring package " + toleranceStalePackage + ": "

// logRecorder collects every Logf call as one rendered line.
type logRecorder struct {
	mutex sync.Mutex
	lines []string
}

// logf is the Options.Logf the tests install; one call is one line.
func (recorder *logRecorder) logf(format string, args ...any) {
	recorder.mutex.Lock()
	defer recorder.mutex.Unlock()
	recorder.lines = append(recorder.lines, fmt.Sprintf(format, args...))
}

// snapshot returns the lines recorded so far.
func (recorder *logRecorder) snapshot() []string {
	recorder.mutex.Lock()
	defer recorder.mutex.Unlock()
	return slices.Clone(recorder.lines)
}

// countStaleLines counts rule 12 lines for stale, checking each carries stale's first error.
func countStaleLines(t *testing.T, lines []string) int {
	t.Helper()
	count := 0
	for _, line := range lines {
		rest, ok := strings.CutPrefix(line, staleIgnoredPrefix)
		if !ok {
			continue
		}
		count++
		if !strings.Contains(rest, "missingThing") {
			t.Errorf("stale's log line must carry its first error mentioning missingThing, got %q", line)
		}
	}
	return count
}

// assertNoExcludedFuncs checks rule 12: functions of excluded packages are never listed.
func assertNoExcludedFuncs(t *testing.T, label string, funcs []deadcode.Func) {
	t.Helper()
	for _, function := range funcs {
		if function.Package == toleranceStalePackage || function.Package == toleranceDependentPackage {
			t.Errorf("%s: function of an excluded package listed: %+v", label, function)
		}
	}
}

// Rule 12: stale and its importer are excluded and logged; what only they reached is dead.
func TestToleranceDeadExcludesBrokenAndDependentPackages(t *testing.T) {
	root := copyFixture(t, "tolerance")
	recorder := &logRecorder{}

	got, err := deadcode.Dead(t.Context(), root, deadcode.Options{Logf: recorder.logf})
	if err != nil {
		t.Fatalf("Dead with a broken package nobody's main imports: %v", err)
	}
	assertNoExcludedFuncs(t, "Dead", got)
	want := []deadcode.Func{expectFunc(t, root, toleranceLibPackage, "lib/lib.go", "OnlyDependent")}
	assertFuncs(t, "Dead", got, want)

	lines := recorder.snapshot()
	if count := countStaleLines(t, lines); count != 1 {
		t.Errorf("want exactly one line %q..., got %d in %q", staleIgnoredPrefix, count, lines)
	}
	if count := countLine(lines, dependentIgnoredLine); count != 1 {
		t.Errorf("want exactly one line %q, got %d in %q", dependentIgnoredLine, count, lines)
	}
	for _, line := range lines {
		if strings.Contains(line, toleranceAppPackage+" ") || strings.Contains(line, toleranceAppPackage+":") ||
			strings.Contains(line, toleranceLibPackage+" ") || strings.Contains(line, toleranceLibPackage+":") {
			t.Errorf("healthy packages get no log line, got %q", line)
		}
		if strings.HasSuffix(line, "\n") {
			t.Errorf("log line must not end in a newline: %q", line)
		}
	}
	if len(lines) != 2 {
		t.Errorf("want exactly two log lines, got %q", lines)
	}
}

// Definitions: Logf nil means silent, so excluding packages must not panic.
func TestToleranceDeadWithNilLogf(t *testing.T) {
	root := copyFixture(t, "tolerance")
	got, err := deadcode.Dead(t.Context(), root, deadcode.Options{})
	if err != nil {
		t.Fatalf("Dead with nil Logf: %v", err)
	}
	assertNoExcludedFuncs(t, "Dead", got)
}

// Rule 12: when the only main package depends on a broken package no root remains.
func TestToleranceDeadWithOnlyMainExcludedHasNoRoots(t *testing.T) {
	root := copyFixture(t, "tolerance")
	writeFile(t, root, "lib/broken.go", "package lib\n\nvar libBroken int = missingInLib\n")

	got, err := deadcode.Dead(t.Context(), root, deadcode.Options{Logf: func(string, ...any) {}})
	if !errors.Is(err, deadcode.ErrNoRoots) {
		t.Errorf("want an error wrapping ErrNoRoots, got %v and%s", err, describeFuncs(got))
	}
}

// Rule 11: with Strict a broken package is an ordinary error naming it.
func TestToleranceDeadStrictIsAnError(t *testing.T) {
	root := copyFixture(t, "tolerance")
	got, err := deadcode.Dead(t.Context(), root, deadcode.Options{Strict: true})
	if err == nil {
		t.Fatalf("want an error with Strict, got nil and%s", describeFuncs(got))
	}
	if errors.Is(err, deadcode.ErrNoRoots) {
		t.Errorf("a type error with Strict is an ordinary error, not ErrNoRoots: %v", err)
	}
	if !strings.Contains(err.Error(), "stale") {
		t.Errorf("error must name the broken package stale, got %v", err)
	}
}

// Rule 13: Orphaned forwards Options, so each revision's Dead logs its excluded packages once.
func TestToleranceOrphanedForwardsOptions(t *testing.T) {
	root := copyFixture(t, "tolerance")
	initRepo(t, root)
	firstCommit := commitAll(t, root, "first")
	replaceInFile(t, root, "app/main.go", "println(lib.Used())", "println(lib.Used() + lib.OnlyDependent())")

	recorder := &logRecorder{}
	diff, err := deadcode.Orphaned(t.Context(), root, firstCommit, "", deadcode.Options{Logf: recorder.logf})
	if err != nil {
		t.Fatalf("Orphaned with a broken package in both revisions: %v", err)
	}
	assertOnlyMainWorktree(t, root)

	lines := recorder.snapshot()
	if count := countStaleLines(t, lines); count != 2 {
		t.Errorf("want stale's line once per revision (2), got %d in %q", count, lines)
	}
	if count := countLine(lines, dependentIgnoredLine); count != 2 {
		t.Errorf("want %q once per revision (2), got %d in %q", dependentIgnoredLine, count, lines)
	}

	assertNoExcludedFuncs(t, "Before", diff.Before)
	assertNoExcludedFuncs(t, "After", diff.After)
	if _, found := findByName(diff.Before, toleranceLibPackage, "OnlyDependent"); !found {
		t.Errorf("before the change only dependent called OnlyDependent, so it is dead:%s", describeFuncs(diff.Before))
	}
	if _, found := findByName(diff.Revived, toleranceLibPackage, "OnlyDependent"); !found {
		t.Errorf("main calls OnlyDependent after the change, so it is revived:%s", describeFuncs(diff.Revived))
	}
}

// Rule 13: Strict is forwarded too, so Orphaned fails on a broken package.
func TestToleranceOrphanedForwardsStrict(t *testing.T) {
	root := copyFixture(t, "tolerance")
	initRepo(t, root)
	firstCommit := commitAll(t, root, "first")

	_, err := deadcode.Orphaned(t.Context(), root, firstCommit, "", deadcode.Options{Strict: true})
	if err == nil {
		t.Fatal("want an error with Strict and a broken package, got nil")
	}
	assertOnlyMainWorktree(t, root)
}

// countLine counts lines equal to want.
func countLine(lines []string, want string) int {
	count := 0
	for _, line := range lines {
		if line == want {
			count++
		}
	}
	return count
}
