package refactor_test

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/AleksanderMako/mechanised-vibe-coding/refactor"
	"github.com/AleksanderMako/mechanised-vibe-coding/symbols"
)

// The broken stale package and the file holding its type error.
const (
	staleFile         = "stale/stale.go"
	staleImportPath   = fixtureModulePath + "/stale"
	staleErrorMention = "missingThing"
)

// brokenUserSource imports lib and calls Hello but has a type error, so it depends on the source package.
const brokenUserSource = `package brokenuser

import "example.com/fixture/lib"

// Use calls the function a rename would have to rewrite.
func Use() string {
	return lib.Hello("user")
}

// Oops has a type error on purpose.
var Oops = undefinedName
`

// healthyStaleSource is stale.go without its type error, importing lib and using Summer with Adder, for the control case of rule 9.
const healthyStaleSource = `package stale

import "example.com/fixture/lib"

// Summer has the method lib.Adder has.
type Summer interface {
	Sum() int
}

// Total uses Summer as a type.
func Total(summer Summer) int {
	return summer.Sum()
}

// lib.Adder is assigned to Summer here, so stale can see the type and Summer counts.
var _ Summer = lib.Adder{}

// Value is fine here.
var Value int = 1
`

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

// assertIgnoredStale checks rule 9's line for stale appears exactly once and carries stale's first error.
func assertIgnoredStale(t *testing.T, lines []string) {
	t.Helper()
	prefix := "refactor: ignoring package " + staleImportPath + ": "
	var matching []string
	for _, line := range lines {
		if strings.Contains(line, staleImportPath) {
			matching = append(matching, line)
		}
	}
	if len(matching) != 1 {
		t.Fatalf("want exactly one log line naming %s, got %q", staleImportPath, lines)
	}
	rest, ok := strings.CutPrefix(matching[0], prefix)
	if !ok {
		t.Fatalf("log line: want prefix %q, got %q", prefix, matching[0])
	}
	if !strings.Contains(rest, staleErrorMention) {
		t.Errorf("log line must carry stale's first error mentioning %q, got %q", staleErrorMention, matching[0])
	}
	if strings.HasSuffix(matching[0], "\n") {
		t.Errorf("log line must not end in a newline: %q", matching[0])
	}
}

// Rules 9 and 10: Rename ignores an unrelated broken package, logs it, and never touches its files.
func TestToleranceRenameIgnoresUnrelatedBrokenPackage(t *testing.T) {
	root := copyFixture(t, "tolerance")
	before := readTree(t, root)
	recorder := &logRecorder{}

	written, err := refactor.Rename(context.Background(), root, sym("lib", "Hello"), "Greet", refactor.Options{Logf: recorder.logf})
	if err != nil {
		t.Fatalf("Rename with an unrelated broken package: %v", err)
	}
	assertWritten(t, root, before, written, []string{"app/main.go", "lib/lib.go"}, nil)
	assertContains(t, root, "lib/lib.go", "func Greet(name string) string")
	assertContains(t, root, "app/main.go", "lib.Greet(\"app\")")
	if readFile(t, root, staleFile) != before[staleFile] {
		t.Errorf("%s must stay byte for byte identical", staleFile)
	}
	assertIgnoredStale(t, recorder.snapshot())
}

// Definitions: Logf nil means silent, so tolerating a package must not panic.
func TestToleranceRenameWithNilLogf(t *testing.T) {
	root := copyFixture(t, "tolerance")
	if _, err := refactor.Rename(context.Background(), root, sym("lib", "Hello"), "Greet", refactor.Options{}); err != nil {
		t.Fatalf("Rename with nil Logf: %v", err)
	}
}

// Rule 8: a broken package depending on the source package fails every operation with ErrBrokenPackage.
func TestToleranceBrokenDependentWrapsErrBrokenPackage(t *testing.T) {
	cases := []struct {
		name string
		run  func(root string) ([]string, error)
	}{
		{"Rename", func(root string) ([]string, error) {
			return refactor.Rename(context.Background(), root, sym("lib", "Hello"), "Greet", refactor.Options{})
		}},
		{"Move", func(root string) ([]string, error) {
			return refactor.Move(context.Background(), root, []symbols.Symbol{sym("lib", "Hello")}, importPath("util"), "util.go", refactor.Options{})
		}},
		{"Demethodize", func(root string) ([]string, error) {
			return refactor.Demethodize(context.Background(), root, sym("lib", "Adder.Sum"), "", refactor.Options{})
		}},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			root := copyFixture(t, "tolerance")
			writeFile(t, root, "brokenuser/brokenuser.go", brokenUserSource)
			before := readTree(t, root)

			_, err := testCase.run(root)

			assertErrorIs(t, err, refactor.ErrBrokenPackage)
			if !strings.Contains(err.Error(), importPath("brokenuser")) {
				t.Errorf("error must name %s, got %v", importPath("brokenuser"), err)
			}
			assertNotBuildError(t, err)
			assertTreeUnchanged(t, root, before)
		})
	}
}

// Rule 8: a broken source package fails with ErrBrokenPackage naming it.
func TestToleranceBrokenSourcePackageWrapsErrBrokenPackage(t *testing.T) {
	root := copyFixture(t, "tolerance")
	writeFile(t, root, "lib/broken.go", "package lib\n\nvar libBroken int = missingInLib\n")
	before := readTree(t, root)

	_, err := refactor.Rename(context.Background(), root, sym("lib", "Hello"), "Greet", refactor.Options{})

	assertErrorIs(t, err, refactor.ErrBrokenPackage)
	if !strings.Contains(err.Error(), importPath("lib")) {
		t.Errorf("error must name %s, got %v", importPath("lib"), err)
	}
	assertTreeUnchanged(t, root, before)
}

// Rule 8: a broken destination package of Move fails with ErrBrokenPackage naming it.
func TestToleranceMoveToBrokenDestinationWrapsErrBrokenPackage(t *testing.T) {
	root := copyFixture(t, "tolerance")
	before := readTree(t, root)

	_, err := refactor.Move(context.Background(), root, []symbols.Symbol{sym("lib", "Hello")}, staleImportPath, "moved.go", refactor.Options{})

	assertErrorIs(t, err, refactor.ErrBrokenPackage)
	if !strings.Contains(err.Error(), staleImportPath) {
		t.Errorf("error must name %s, got %v", staleImportPath, err)
	}
	assertNotBuildError(t, err)
	assertTreeUnchanged(t, root, before)
}

// Rule 7: with Strict any broken package is an ordinary error and nothing is written.
func TestToleranceStrictFailsOnAnyBrokenPackage(t *testing.T) {
	strict := refactor.Options{Strict: true}
	cases := []struct {
		name string
		run  func(root string) ([]string, error)
	}{
		{"Rename", func(root string) ([]string, error) {
			return refactor.Rename(context.Background(), root, sym("lib", "Hello"), "Greet", strict)
		}},
		{"Move", func(root string) ([]string, error) {
			return refactor.Move(context.Background(), root, []symbols.Symbol{sym("lib", "Hello")}, importPath("util"), "util.go", strict)
		}},
		{"Demethodize", func(root string) ([]string, error) {
			return refactor.Demethodize(context.Background(), root, sym("lib", "Adder.Sum"), "", strict)
		}},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			root := copyFixture(t, "tolerance")
			before := readTree(t, root)

			_, err := testCase.run(root)

			if err == nil {
				t.Fatal("want an error with Strict and a broken package, got nil")
			}
			assertNotBuildError(t, err)
			assertTreeUnchanged(t, root, before)
		})
	}
}

// Rule 9 control: with stale healthy and assigning lib.Adder to Summer, demethodizing Adder.Sum breaks it (stage 4 rule 22).
func TestToleranceDemethodizeBreaksInterfaceWhenStaleIsHealthy(t *testing.T) {
	root := copyFixture(t, "tolerance")
	writeFile(t, root, staleFile, healthyStaleSource)
	before := readTree(t, root)

	_, err := refactor.Demethodize(context.Background(), root, sym("lib", "Adder.Sum"), "", refactor.Options{})

	assertErrorIs(t, err, refactor.ErrBreaksInterface)
	assertTreeUnchanged(t, root, before)
}

// Rule 9: an ignored broken package is never scanned, so its Summer interface cannot trigger ErrBreaksInterface.
func TestToleranceDemethodizeIgnoresInterfaceInBrokenPackage(t *testing.T) {
	root := copyFixture(t, "tolerance")
	before := readTree(t, root)
	recorder := &logRecorder{}

	written, err := refactor.Demethodize(context.Background(), root, sym("lib", "Adder.Sum"), "", refactor.Options{Logf: recorder.logf})
	if errors.Is(err, refactor.ErrBreaksInterface) {
		t.Fatalf("stale is ignored, so its Summer interface must not trigger ErrBreaksInterface: %v", err)
	}
	if err != nil {
		t.Fatalf("Demethodize with an unrelated broken package: %v", err)
	}
	assertWritten(t, root, before, written, []string{"lib/lib.go"}, nil)
	assertContains(t, root, "lib/lib.go", "func Sum() int")
	if readFile(t, root, staleFile) != before[staleFile] {
		t.Errorf("%s must stay byte for byte identical", staleFile)
	}
	assertIgnoredStale(t, recorder.snapshot())
}
