package symbols_test

import (
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"
	"testing"

	"github.com/AleksanderMako/mechanised-vibe-coding/symbols"
)

// fixtureModulePath is the module path declared in testdata/module/go.mod.
const fixtureModulePath = "example.com/fixture"

// copyFixture copies testdata/<name> into a fresh temp dir and returns its symlink free absolute path.
func copyFixture(t *testing.T, name string) string {
	t.Helper()
	source := filepath.Join("testdata", name)
	target := filepath.Join(t.TempDir(), name)
	err := filepath.WalkDir(source, func(walkedPath string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		relative, err := filepath.Rel(source, walkedPath)
		if err != nil {
			return err
		}
		destination := filepath.Join(target, relative)
		if entry.IsDir() {
			return os.MkdirAll(destination, 0o755)
		}
		content, err := os.ReadFile(walkedPath)
		if err != nil {
			return err
		}
		return os.WriteFile(destination, content, 0o644)
	})
	if err != nil {
		t.Fatalf("copy fixture %s: %v", name, err)
	}
	// Temp dirs on macOS sit behind a symlink; resolving it keeps path comparisons stable.
	resolved, err := filepath.EvalSymlinks(target)
	if err != nil {
		t.Fatalf("resolve fixture path: %v", err)
	}
	return resolved
}

// sym builds a Symbol in the fixture module from a package directory and a name.
func sym(packageDir, name string) symbols.Symbol {
	return symbols.Symbol{Package: fixtureModulePath + "/" + packageDir, Name: name}
}

// fileLines returns the lines of root/relative so tests can locate markers.
func fileLines(t *testing.T, root, relative string) []string {
	t.Helper()
	content, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(relative)))
	if err != nil {
		t.Fatalf("read %s: %v", relative, err)
	}
	return strings.Split(string(content), "\n")
}

// markerLine returns the 1-based line in root/relative ending with "// ref:<marker>", so fixture edits never break line numbers.
func markerLine(t *testing.T, root, relative, marker string) int {
	t.Helper()
	found := 0
	for index, line := range fileLines(t, root, relative) {
		if strings.HasSuffix(strings.TrimRight(line, " \t\r"), "// ref:"+marker) {
			if found != 0 {
				t.Fatalf("marker %q appears more than once in %s", marker, relative)
			}
			found = index + 1
		}
	}
	if found == 0 {
		t.Fatalf("marker %q not found in %s", marker, relative)
	}
	return found
}

// wantRef is one expected reference: the fixture file, the marker on its line, and the kind.
type wantRef struct {
	file   string // slash separated, relative to the fixture root
	marker string
	kind   symbols.Kind
}

// refKey identifies a reference by file and line; the fixture keeps one use of each symbol per line.
func refKey(file string, line int) string {
	return fmt.Sprintf("%s:%d", file, line)
}

// assertReferences checks got holds exactly the wanted references with every promised field right.
func assertReferences(t *testing.T, root string, got []symbols.Reference, want []wantRef) {
	t.Helper()
	assertSortedAndUnique(t, got)

	wantByKey := make(map[string]wantRef, len(want))
	for _, expected := range want {
		line := markerLine(t, root, expected.file, expected.marker)
		absolute := filepath.Join(root, filepath.FromSlash(expected.file))
		wantByKey[refKey(absolute, line)] = expected
	}

	gotKeys := make(map[string]bool, len(got))
	for _, ref := range got {
		key := refKey(ref.File, ref.Line)
		gotKeys[key] = true
		expected, ok := wantByKey[key]
		if !ok {
			t.Errorf("unexpected reference %s (%s): %q", key, ref.Kind, ref.Context)
			continue
		}
		if ref.Kind != expected.kind {
			t.Errorf("reference at marker %s: want kind %q, got %q", expected.marker, expected.kind, ref.Kind)
		}
		wantPackage := fixtureModulePath + "/" + path.Dir(expected.file)
		if ref.Package != wantPackage {
			t.Errorf("reference at marker %s: want Package %q, got %q", expected.marker, wantPackage, ref.Package)
		}
		wantTest := strings.HasSuffix(expected.file, "_test.go")
		if ref.Test != wantTest {
			t.Errorf("reference at marker %s: want Test %v, got %v", expected.marker, wantTest, ref.Test)
		}
		if ref.Column <= 0 {
			t.Errorf("reference at marker %s: Column must be positive, got %d", expected.marker, ref.Column)
		}
		wantContext := strings.TrimSpace(fileLines(t, root, expected.file)[ref.Line-1])
		if ref.Context != wantContext {
			t.Errorf("reference at marker %s: want Context %q, got %q", expected.marker, wantContext, ref.Context)
		}
	}
	for key, expected := range wantByKey {
		if !gotKeys[key] {
			t.Errorf("missing reference at marker %s (%s)", expected.marker, key)
		}
	}
}

// assertSortedAndUnique checks the order promised by rule 8 and the dedup of rule 9.
func assertSortedAndUnique(t *testing.T, refs []symbols.Reference) {
	t.Helper()
	seen := make(map[string]bool, len(refs))
	for index, ref := range refs {
		position := fmt.Sprintf("%s:%d:%d", ref.File, ref.Line, ref.Column)
		if seen[position] {
			t.Errorf("duplicate reference at %s", position)
		}
		seen[position] = true
		if !filepath.IsAbs(ref.File) {
			t.Errorf("reference File must be absolute, got %q", ref.File)
		}
		if index > 0 && referenceLess(ref, refs[index-1]) {
			t.Errorf("references out of order: %+v sorts before %+v", ref, refs[index-1])
		}
	}
}

// referenceLess orders references by Package, File, Line, Column.
func referenceLess(a, b symbols.Reference) bool {
	if a.Package != b.Package {
		return a.Package < b.Package
	}
	if a.File != b.File {
		return a.File < b.File
	}
	if a.Line != b.Line {
		return a.Line < b.Line
	}
	return a.Column < b.Column
}
