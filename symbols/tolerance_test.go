package symbols_test

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/AleksanderMako/mechanised-vibe-coding/symbols"
)

// Import paths of the stage 9 tolerance fixture packages.
const (
	toleranceLib        = fixtureModulePath + "/lib"
	toleranceApp        = fixtureModulePath + "/app"
	toleranceStale      = fixtureModulePath + "/stale"
	toleranceBrokenUser = fixtureModulePath + "/brokenuser"
)

// The two log line prefixes stage 9 rules 3 and 4 define, with the package ID filled in.
func ignoringLinePrefix(packageID string) string {
	return "symbols: ignoring package " + packageID + ": "
}

func bestEffortLinePrefix(packageID string) string {
	return "symbols: package " + packageID + " has errors, results there are best effort: "
}

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

// linesMentioning returns the recorded lines that name packageID as a whole word.
func linesMentioning(lines []string, packageID string) []string {
	var matching []string
	for _, line := range lines {
		if strings.Contains(line, " "+packageID+" ") || strings.Contains(line, " "+packageID+":") {
			matching = append(matching, line)
		}
	}
	return matching
}

// assertOneLine checks exactly one line names packageID, it starts with wantPrefix and carries the first error.
func assertOneLine(t *testing.T, lines []string, packageID, wantPrefix, wantErrorFragment string) {
	t.Helper()
	matching := linesMentioning(lines, packageID)
	if len(matching) != 1 {
		t.Errorf("want exactly one log line for %s, got %d: %q", packageID, len(matching), lines)
		return
	}
	line := matching[0]
	rest, ok := strings.CutPrefix(line, wantPrefix)
	if !ok {
		t.Errorf("log line for %s: want prefix %q, got %q", packageID, wantPrefix, line)
		return
	}
	if !strings.Contains(rest, wantErrorFragment) {
		t.Errorf("log line for %s: want the package's first error mentioning %q, got %q", packageID, wantErrorFragment, line)
	}
}

// assertLinesWellFormed checks no line ends in a newline and no healthy package is mentioned.
func assertLinesWellFormed(t *testing.T, lines []string, healthy ...string) {
	t.Helper()
	for _, line := range lines {
		if strings.HasSuffix(line, "\n") {
			t.Errorf("log line must not end in a newline: %q", line)
		}
	}
	for _, packageID := range healthy {
		if mentioned := linesMentioning(lines, packageID); len(mentioned) != 0 {
			t.Errorf("healthy package %s must get no log line, got %q", packageID, mentioned)
		}
	}
}

// Rule 1: Strict makes any broken package an error naming it, with no partial result.
func TestToleranceStrictFailsWithoutPartialResult(t *testing.T) {
	root := copyFixture(t, "tolerance")
	recorder := &logRecorder{}
	opts := symbols.Options{Strict: true, Logf: recorder.logf}
	errs := callAll(t, root, sym("lib", "Hello"), sym("lib", "Speaker"), sym("lib", "Config"), opts)
	for function, err := range errs {
		if err == nil {
			t.Errorf("%s with Strict on a module with broken packages: want error", function)
			continue
		}
		message := err.Error()
		if !strings.Contains(message, toleranceStale) && !strings.Contains(message, toleranceBrokenUser) {
			t.Errorf("%s with Strict: error must name a broken package (%s or %s), got %v", function, toleranceStale, toleranceBrokenUser, err)
		}
	}
}

// Rules 3, 4 and 6: References skips stale, scans brokenuser best effort and logs once per broken package.
func TestToleranceReferencesOfFunction(t *testing.T) {
	root := copyFixture(t, "tolerance")
	recorder := &logRecorder{}
	refs, err := symbols.References(t.Context(), root, sym("lib", "Hello"), symbols.Options{Logf: recorder.logf})
	if err != nil {
		t.Fatalf("References(lib.Hello) without Strict: %v", err)
	}
	assertReferences(t, root, refs, []wantRef{
		{"app/main.go", "app-hello", symbols.KindCall},
		{"brokenuser/brokenuser.go", "broken-hello", symbols.KindCall},
	})
	lines := recorder.snapshot()
	assertOneLine(t, lines, toleranceStale, ignoringLinePrefix(toleranceStale), "missingThing")
	assertOneLine(t, lines, toleranceBrokenUser, bestEffortLinePrefix(toleranceBrokenUser), "undefinedName")
	assertLinesWellFormed(t, lines, toleranceLib, toleranceApp)
	if len(lines) != 2 {
		t.Errorf("want exactly two log lines, one per broken package, got %q", lines)
	}
}

// Rules 3 and 4 hold for a type symbol too: the struct is used in a type position in both users.
func TestToleranceReferencesOfType(t *testing.T) {
	root := copyFixture(t, "tolerance")
	recorder := &logRecorder{}
	refs, err := symbols.References(t.Context(), root, sym("lib", "Config"), symbols.Options{Logf: recorder.logf})
	if err != nil {
		t.Fatalf("References(lib.Config) without Strict: %v", err)
	}
	assertReferences(t, root, refs, []wantRef{
		{"app/main.go", "app-config", symbols.KindType},
		{"brokenuser/brokenuser.go", "broken-config", symbols.KindType},
	})
	lines := recorder.snapshot()
	assertOneLine(t, lines, toleranceStale, ignoringLinePrefix(toleranceStale), "missingThing")
	assertOneLine(t, lines, toleranceBrokenUser, bestEffortLinePrefix(toleranceBrokenUser), "undefinedName")
	assertLinesWellFormed(t, lines, toleranceLib, toleranceApp)
}

// Rule 3: StructTags ignores stale with the ignoring line and answers from the healthy declaring package.
func TestToleranceStructTags(t *testing.T) {
	root := copyFixture(t, "tolerance")
	recorder := &logRecorder{}
	fields, err := symbols.StructTags(t.Context(), root, sym("lib", "Config"), symbols.Options{Logf: recorder.logf})
	if err != nil {
		t.Fatalf("StructTags(lib.Config) without Strict: %v", err)
	}
	want := []symbols.FieldTag{
		{Field: "Name", Type: "string", Exported: true, Tag: `json:"name"`, JSON: "name"},
		{Field: "Port", Type: "int", Exported: true, Tag: `json:"port,omitempty"`, JSON: "port", OmitEmpty: true},
	}
	if !slices.Equal(fields, want) {
		t.Errorf("StructTags(lib.Config):\n got %+v\nwant %+v", fields, want)
	}
	lines := recorder.snapshot()
	assertOneLine(t, lines, toleranceStale, ignoringLinePrefix(toleranceStale), "missingThing")
	assertLinesWellFormed(t, lines, toleranceLib, toleranceApp)
	// Rule 3: StructTags reads only the declaring package, so every other broken package is ignored.
	assertOneLine(t, lines, toleranceBrokenUser, ignoringLinePrefix(toleranceBrokenUser), "undefinedName")
	if len(lines) != 2 {
		t.Errorf("want exactly two log lines, one per broken package, got %q", lines)
	}
}

// Rule 5: every broken package may hold an implementation, so both are scanned best effort.
func TestToleranceImplementationsScanBrokenPackages(t *testing.T) {
	root := copyFixture(t, "tolerance")
	recorder := &logRecorder{}
	impls, err := symbols.Implementations(t.Context(), root, sym("lib", "Speaker"), symbols.Options{Logf: recorder.logf})
	if err != nil {
		t.Fatalf("Implementations(lib.Speaker) without Strict: %v", err)
	}
	want := []symbols.Symbol{
		sym("brokenuser", "UserSpeaker"),
		sym("lib", "LibSpeaker"),
		sym("stale", "StaleSpeaker"),
	}
	if !slices.Equal(impls, want) {
		t.Errorf("Implementations(lib.Speaker):\n got %v\nwant %v", impls, want)
	}
	lines := recorder.snapshot()
	assertOneLine(t, lines, toleranceStale, bestEffortLinePrefix(toleranceStale), "missingThing")
	assertOneLine(t, lines, toleranceBrokenUser, bestEffortLinePrefix(toleranceBrokenUser), "undefinedName")
	assertLinesWellFormed(t, lines, toleranceLib, toleranceApp)
	if len(lines) != 2 {
		t.Errorf("want exactly two log lines, one per broken package, got %q", lines)
	}
}

// Rule 2: a broken declaring package is an error even without Strict, for every lookup.
func TestToleranceBrokenDeclaringPackageIsAnError(t *testing.T) {
	root := copyFixture(t, "tolerance")
	writeFixtureFile(t, root, "lib/broken.go", "package lib\n\nvar libBroken int = missingInLib\n")
	recorder := &logRecorder{}
	errs := callAll(t, root, sym("lib", "Hello"), sym("lib", "Speaker"), sym("lib", "Config"), symbols.Options{Logf: recorder.logf})
	for function, err := range errs {
		if err == nil {
			t.Errorf("%s with a broken declaring package: want error", function)
			continue
		}
		if !strings.Contains(err.Error(), toleranceLib) {
			t.Errorf("%s: error must name the declaring package %s, got %v", function, toleranceLib, err)
		}
	}
}

// Rule 2: a broken test variant of the declaring package is an error too; without tests it is not loaded.
func TestToleranceBrokenTestVariantOfDeclaringPackage(t *testing.T) {
	root := copyFixture(t, "tolerance")
	writeFixtureFile(t, root, "lib/lib_test.go", "package lib\n\nvar testBroken int = missingInTest\n")

	_, err := symbols.References(t.Context(), root, sym("lib", "Hello"), symbols.Options{})
	if err == nil {
		t.Error("References with a broken test variant of the declaring package: want error")
	} else if !strings.Contains(err.Error(), toleranceLib) {
		t.Errorf("error must name the declaring package %s, got %v", toleranceLib, err)
	}

	refs, err := symbols.References(t.Context(), root, sym("lib", "Hello"), symbols.Options{ExcludeTests: true})
	if err != nil {
		t.Fatalf("References with ExcludeTests leaves the broken test file out, want success, got %v", err)
	}
	assertReferences(t, root, refs, []wantRef{
		{"app/main.go", "app-hello", symbols.KindCall},
		{"brokenuser/brokenuser.go", "broken-hello", symbols.KindCall},
	})
}

// Definitions: Logf nil means silent, so tolerated packages must not panic the call.
func TestToleranceNilLogfIsSilent(t *testing.T) {
	root := copyFixture(t, "tolerance")
	opts := symbols.Options{}
	if _, err := symbols.References(t.Context(), root, sym("lib", "Hello"), opts); err != nil {
		t.Errorf("References with nil Logf: %v", err)
	}
	if _, err := symbols.StructTags(t.Context(), root, sym("lib", "Config"), opts); err != nil {
		t.Errorf("StructTags with nil Logf: %v", err)
	}
	if _, err := symbols.Implementations(t.Context(), root, sym("lib", "Speaker"), opts); err != nil {
		t.Errorf("Implementations with nil Logf: %v", err)
	}
}

// writeFixtureFile adds a file to a fixture copy, creating parent directories.
func writeFixtureFile(t *testing.T, root, relative, content string) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(relative))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
