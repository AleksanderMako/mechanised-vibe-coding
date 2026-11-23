package main_test

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/AleksanderMako/machanised-vibe-coding/deadcode"
	"github.com/AleksanderMako/machanised-vibe-coding/symbols"
)

// Symbols and packages of the tolerance fixture: stale is broken and unrelated, brokenuser is broken and calls lib.Hello.
const (
	toleranceHello      = "example.com/fixture/lib.Hello"
	toleranceSpeaker    = "example.com/fixture/lib.Speaker"
	toleranceConfig     = "example.com/fixture/lib.Config"
	toleranceStale      = "example.com/fixture/stale"
	toleranceBrokenUser = "example.com/fixture/brokenuser"
	warningPrefix       = "mvc: warning: "
	brokenPackageText   = "a package that may use the symbol has errors"
)

// copyTolerance copies the fixture with the broken packages.
func copyTolerance(t *testing.T) string {
	t.Helper()
	return copyFixture(t, "tolerance")
}

// expectWarningsOnly checks rule 14: success, every stderr line a warning, the wanted ones present, none on stdout.
func expectWarningsOnly(t *testing.T, result runResult, wantWarningPrefixes ...string) {
	t.Helper()
	if strings.Contains(result.stdout, "warning") {
		t.Errorf("warnings must never appear on stdout:\n%s", result.describe())
	}
	stderrLines := nonEmptyLines(result.stderr)
	for _, line := range stderrLines {
		if !strings.HasPrefix(line, warningPrefix) {
			t.Errorf("stderr line %q is not a warning:\n%s", line, result.describe())
		}
	}
	for _, wantPrefix := range wantWarningPrefixes {
		found := slices.ContainsFunc(stderrLines, func(line string) bool { return strings.HasPrefix(line, wantPrefix) })
		if !found {
			t.Errorf("no stderr line starts with %q:\n%s", wantPrefix, result.describe())
		}
	}
}

// expectSingleErrorLine checks rules 14 and 15 for an error: exit 2, empty stdout, one non-warning `mvc: ` line naming want.
func expectSingleErrorLine(t *testing.T, result runResult, allowWarnings bool, wantFragments ...string) {
	t.Helper()
	expectExit(t, result, 2)
	if strings.TrimSpace(result.stdout) != "" {
		t.Errorf("error run printed to stdout:\n%s", result.describe())
	}
	var errorLines []string
	for _, line := range nonEmptyLines(result.stderr) {
		if allowWarnings && strings.HasPrefix(line, warningPrefix) {
			continue
		}
		errorLines = append(errorLines, line)
	}
	if len(errorLines) != 1 {
		t.Fatalf("want exactly one error line on stderr, got %d:\n%s", len(errorLines), result.describe())
	}
	if !strings.HasPrefix(errorLines[0], "mvc: ") || strings.HasPrefix(errorLines[0], warningPrefix) {
		t.Errorf("error line %q must start with %q and not be a warning", errorLines[0], "mvc: ")
	}
	for _, fragment := range wantFragments {
		if !strings.Contains(errorLines[0], fragment) {
			t.Errorf("error line %q must contain %q", errorLines[0], fragment)
		}
	}
}

// Rule 14: refs without -strict succeeds, prints references on stdout and the warnings on stderr.
func TestToleranceSymbolsRefsText(t *testing.T) {
	module := copyTolerance(t)
	result := runMVC(t, module, "symbols", "refs", "-root", module, toleranceHello)
	expectExit(t, result, 0)
	lines := nonEmptyLines(result.stdout)
	for _, line := range lines {
		if !referenceLine.MatchString(line) {
			t.Errorf("stdout line %q does not match %s", line, referenceLine)
		}
	}
	for _, wantFile := range []string{"app/main.go:", "brokenuser/brokenuser.go:"} {
		if !strings.Contains(result.stdout, wantFile) {
			t.Errorf("no reference in %s:\n%s", wantFile, result.describe())
		}
	}
	expectWarningsOnly(t, result,
		warningPrefix+"symbols: ignoring package "+toleranceStale+": ",
		warningPrefix+"symbols: package "+toleranceBrokenUser+" has errors, results there are best effort: ",
	)
}

// Rule 14: with -json stdout is the JSON alone and the warnings stay on stderr.
func TestToleranceSymbolsRefsJSON(t *testing.T) {
	module := copyTolerance(t)
	result := runMVC(t, module, "symbols", "refs", "-root", module, "-json", toleranceHello)
	expectExit(t, result, 0)
	var refs []symbols.Reference
	decodeJSON(t, result, &refs)
	for _, wantPackage := range []string{"example.com/fixture/app", toleranceBrokenUser} {
		if !slices.Contains(referencePackages(refs), wantPackage) {
			t.Errorf("no reference in %s: %+v", wantPackage, refs)
		}
	}
	expectWarningsOnly(t, result,
		warningPrefix+"symbols: ignoring package "+toleranceStale+": ",
		warningPrefix+"symbols: package "+toleranceBrokenUser+" has errors, results there are best effort: ",
	)
}

// Rule 14: -strict turns a broken package into an ordinary error with exit 2.
func TestToleranceSymbolsStrict(t *testing.T) {
	cases := []struct {
		name string
		args []string
	}{
		{"refs", []string{"symbols", "refs", "-strict", toleranceHello}},
		{"refs json", []string{"symbols", "refs", "-strict", "-json", toleranceHello}},
		{"impls", []string{"symbols", "impls", "-strict", toleranceSpeaker}},
		{"tags", []string{"symbols", "tags", "-strict", toleranceConfig}},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			module := copyTolerance(t)
			args := append(slices.Clone(testCase.args[:2]), append([]string{"-root", module}, testCase.args[2:]...)...)
			result := runMVC(t, module, args...)
			expectSingleErrorLine(t, result, false)
		})
	}
}

// Rule 14: impls and tags without -strict also succeed with warnings on stderr only.
func TestToleranceSymbolsImplsAndTags(t *testing.T) {
	module := copyTolerance(t)

	impls := runMVC(t, module, "symbols", "impls", "-root", module, toleranceSpeaker)
	expectExit(t, impls, 0)
	expectWarningsOnly(t, impls,
		warningPrefix+"symbols: package "+toleranceStale+" has errors, results there are best effort: ",
		warningPrefix+"symbols: package "+toleranceBrokenUser+" has errors, results there are best effort: ",
	)

	tags := runMVC(t, module, "symbols", "tags", "-root", module, toleranceConfig)
	expectExit(t, tags, 0)
	if !strings.Contains(tags.stdout, "Name\tstring\t") {
		t.Errorf("tags must list the Name field:\n%s", tags.describe())
	}
	expectWarningsOnly(t, tags, warningPrefix+"symbols: ignoring package "+toleranceStale+": ")
}

// Rule 15: a rename blocked by a broken dependent exits 2 with the ErrBrokenPackage text on one line.
func TestToleranceRefactorRenameBrokenDependent(t *testing.T) {
	module := copyTolerance(t)
	libBefore := readFile(t, module, "lib/lib.go")
	result := runMVC(t, module, "refactor", "rename", "-root", module, toleranceHello, "Greet")
	expectSingleErrorLine(t, result, true, brokenPackageText, toleranceBrokenUser)
	if readFile(t, module, "lib/lib.go") != libBefore {
		t.Error("lib/lib.go changed although the rename failed")
	}
}

// Rule 14: a rename next to an unrelated broken package succeeds and warns on stderr only.
func TestToleranceRefactorRenameIgnoresUnrelated(t *testing.T) {
	module := copyTolerance(t)
	if err := os.RemoveAll(filepath.Join(module, "brokenuser")); err != nil {
		t.Fatal(err)
	}
	result := runMVC(t, module, "refactor", "rename", "-root", module, toleranceHello, "Greet")
	expectExit(t, result, 0)
	for _, line := range nonEmptyLines(result.stdout) {
		if !filepath.IsAbs(strings.TrimSpace(line)) {
			t.Errorf("stdout line %q is not a written file:\n%s", line, result.describe())
		}
	}
	expectWarningsOnly(t, result, warningPrefix+"refactor: ignoring package "+toleranceStale+": ")

	strict := copyTolerance(t)
	if err := os.RemoveAll(filepath.Join(strict, "brokenuser")); err != nil {
		t.Fatal(err)
	}
	strictResult := runMVC(t, strict, "refactor", "rename", "-root", strict, "-strict", toleranceHello, "Greet")
	expectSingleErrorLine(t, strictResult, false)
}

// Rule 14: dead list prints the warnings on stderr and the list on stdout.
func TestToleranceDeadListText(t *testing.T) {
	module := copyTolerance(t)
	result := runMVC(t, module, "dead", "list", "-root", module)
	expectExit(t, result, 1)
	if !strings.Contains(result.stdout, "example.com/fixture/lib\tUnused\t") {
		t.Errorf("lib.Unused is not listed:\n%s", result.describe())
	}
	for _, excluded := range []string{toleranceStale, toleranceBrokenUser} {
		if strings.Contains(result.stdout, excluded) {
			t.Errorf("excluded package %s must not appear on stdout:\n%s", excluded, result.describe())
		}
	}
	expectWarningsOnly(t, result,
		warningPrefix+"deadcode: ignoring package "+toleranceStale+": ",
		warningPrefix+"deadcode: ignoring package "+toleranceBrokenUser+": ",
	)
}

// Rule 14: dead list -json keeps stdout pure JSON while warning on stderr.
func TestToleranceDeadListJSON(t *testing.T) {
	module := copyTolerance(t)
	result := runMVC(t, module, "dead", "list", "-root", module, "-json")
	expectExit(t, result, 1)
	var functions []deadcode.Func
	decodeJSON(t, result, &functions)
	if !slices.Contains(funcNames(functions), "example.com/fixture/lib.Unused") {
		t.Errorf("dead = %v, want lib.Unused among them", funcNames(functions))
	}
	expectWarningsOnly(t, result, warningPrefix+"deadcode: ignoring package "+toleranceStale+": ")
}

// Rule 14: dead list -strict is an ordinary error.
func TestToleranceDeadListStrict(t *testing.T) {
	module := copyTolerance(t)
	result := runMVC(t, module, "dead", "list", "-root", module, "-strict")
	expectSingleErrorLine(t, result, false)
}

// Rule 14: dead orphaned warns on stderr and keeps its normal stdout.
func TestToleranceDeadOrphaned(t *testing.T) {
	repo := copyTolerance(t)
	runGit(t, repo, "init", "-q", "-b", "main")
	runGit(t, repo, "add", ".")
	runGit(t, repo, "commit", "-q", "-m", "with broken packages")

	result := runMVC(t, repo, "dead", "orphaned", "-root", repo, "HEAD")
	expectExit(t, result, 0)
	if !strings.Contains(result.stdout, "newly dead:") || !strings.Contains(result.stdout, "revived:") {
		t.Errorf("both headers must be printed:\n%s", result.describe())
	}
	expectWarningsOnly(t, result, warningPrefix+"deadcode: ignoring package "+toleranceStale+": ")

	strict := runMVC(t, repo, "dead", "orphaned", "-root", repo, "-strict", "HEAD")
	expectSingleErrorLine(t, strict, false)
}

// Rule 14: the usage of every affected action lists -strict.
func TestToleranceUsageListsStrict(t *testing.T) {
	actions := [][]string{
		{"symbols", "refs"},
		{"symbols", "impls"},
		{"symbols", "tags"},
		{"refactor", "rename"},
		{"refactor", "move"},
		{"refactor", "demethodize"},
		{"dead", "list"},
		{"dead", "orphaned"},
	}
	for _, action := range actions {
		t.Run(strings.Join(action, " "), func(t *testing.T) {
			result := runMVC(t, realTempDir(t), append(slices.Clone(action), "-h")...)
			expectExit(t, result, 0)
			if !strings.Contains(result.stdout, "-strict") {
				t.Errorf("usage does not list -strict:\n%s", result.describe())
			}
		})
	}
}
