package main_test

import (
	"strings"
	"testing"
)

// Rule 6: asking for usage at any level prints it to stdout and exits 0.
func TestUsageRequestedExitsZeroOnStdout(t *testing.T) {
	cases := []struct {
		name        string
		args        []string
		mustMention []string // names the usage of that level lists
	}{
		{name: "no arguments", args: nil, mustMention: []string{"edit", "ast", "verify", "symbols", "refactor", "env", "merge", "dead"}},
		{name: "group alone", args: []string{"edit"}, mustMention: []string{"check", "apply"}},
		{name: "top level -h", args: []string{"-h"}, mustMention: []string{"edit", "verify"}},
		{name: "top level -help", args: []string{"-help"}, mustMention: []string{"edit", "verify"}},
		{name: "group -help", args: []string{"verify", "-help"}, mustMention: []string{"run", "flaky"}},
		{name: "group -h", args: []string{"symbols", "-h"}, mustMention: []string{"refs", "impls", "tags"}},
		{name: "action -h", args: []string{"symbols", "refs", "-h"}, mustMention: []string{"-root"}},
		{name: "action -help", args: []string{"env", "conflicts", "-help"}, mustMention: []string{"-distinct"}},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			result := runMVC(t, realTempDir(t), testCase.args...)
			expectExit(t, result, 0)
			if strings.TrimSpace(result.stdout) == "" {
				t.Fatalf("usage not printed to stdout:\n%s", result.describe())
			}
			for _, name := range testCase.mustMention {
				if !strings.Contains(result.stdout, name) {
					t.Errorf("usage does not mention %q:\n%s", name, result.describe())
				}
			}
		})
	}
}

// Rule 6: an unknown group or action prints usage to stderr, nothing to stdout, and exits 2.
func TestUnknownGroupOrActionExitsTwoOnStderr(t *testing.T) {
	cases := []struct {
		name string
		args []string
	}{
		{name: "unknown group", args: []string{"nosuch"}},
		{name: "unknown action", args: []string{"edit", "nosuch"}},
		{name: "unknown action in another group", args: []string{"dead", "nosuch"}},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			result := runMVC(t, realTempDir(t), testCase.args...)
			expectExit(t, result, 2)
			if result.stdout != "" {
				t.Errorf("stdout must be empty:\n%s", result.describe())
			}
			if strings.TrimSpace(result.stderr) == "" {
				t.Errorf("usage not printed to stderr:\n%s", result.describe())
			}
		})
	}
}

// Rules 3 and 4: a missing required flag or argument is a usage error with exit 2.
func TestMissingRequiredInputIsUsageError(t *testing.T) {
	module := copyModule(t)
	cases := []struct {
		name string
		args []string
	}{
		{name: "edit check without edits file", args: []string{"edit", "check", "-root", module}},
		{name: "edit apply without edits file", args: []string{"edit", "apply", "-root", module}},
		{name: "ast set-doc without -file", args: []string{"ast", "set-doc", "-root", module, "-symbol", "NewSquare", "-doc", "x"}},
		{name: "ast remove-field without -name", args: []string{"ast", "remove-field", "-root", module, "-file", "shapes/shapes.go", "-type", "Square"}},
		{name: "verify flaky without -pkg", args: []string{"verify", "flaky", "-root", module}},
		{name: "symbols refs without symbol", args: []string{"symbols", "refs", "-root", module}},
		{name: "refactor rename without new name", args: []string{"refactor", "rename", "-root", module, "example.com/fixture/shapes.NewSquare"}},
		{name: "env vars without -dir", args: []string{"env", "vars", "-type", "Config"}},
		{name: "dead orphaned without BEFORE", args: []string{"dead", "orphaned", "-root", module}},
		{name: "unknown flag", args: []string{"symbols", "refs", "-root", module, "-nosuchflag", "example.com/fixture/shapes.NewSquare"}},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			expectErrorReported(t, runMVC(t, module, testCase.args...))
		})
	}
}

// Rules 3 and 4: a root without go.mod is an ordinary error with exit 2 and one `mvc: ` line.
func TestRootWithoutGoModIsOrdinaryError(t *testing.T) {
	noModule := emptyDir(t)
	cases := []struct {
		name string
		args []string
	}{
		{name: "verify run", args: []string{"verify", "run", "-root", noModule, "-pkg", "./..."}},
		{name: "verify run json", args: []string{"verify", "run", "-root", noModule, "-pkg", "./...", "-json"}},
		{name: "symbols refs", args: []string{"symbols", "refs", "-root", noModule, "example.com/fixture/shapes.NewSquare"}},
		{name: "symbols tags json", args: []string{"symbols", "tags", "-root", noModule, "-json", "example.com/fixture/shapes.Square"}},
		{name: "refactor rename", args: []string{"refactor", "rename", "-root", noModule, "example.com/fixture/shapes.NewSquare", "MakeSquare"}},
		{name: "dead list", args: []string{"dead", "list", "-root", noModule}},
		{name: "dead list json", args: []string{"dead", "list", "-root", noModule, "-json"}},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			expectOrdinaryError(t, runMVC(t, noModule, testCase.args...))
		})
	}
}
