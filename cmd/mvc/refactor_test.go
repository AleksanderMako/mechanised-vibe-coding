package main_test

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// Rename success prints the written files, one absolute path per line, and the module still builds.
func TestRefactorRename(t *testing.T) {
	module := copyModule(t)
	result := runMVC(t, module, "refactor", "rename", "-root", module, newSquareSymbol, "MakeSquare")
	expectExit(t, result, 0)

	var printed []string
	for _, line := range nonEmptyLines(result.stdout) {
		printed = append(printed, strings.TrimSpace(line))
	}
	for _, relative := range []string{"shapes/shapes.go", "alpha/alpha.go", "beta/beta.go", "gamma/gamma.go"} {
		wantPath := filepath.Join(module, filepath.FromSlash(relative))
		if !slices.Contains(printed, wantPath) {
			t.Errorf("written file %s not printed:\n%s", wantPath, result.describe())
		}
	}
	for _, path := range printed {
		if !filepath.IsAbs(path) {
			t.Errorf("printed path %q is not absolute", path)
		}
		if _, err := os.Stat(path); err != nil {
			t.Errorf("printed path %q does not exist: %v", path, err)
		}
	}
	if !strings.Contains(readFile(t, module, "shapes/shapes.go"), "func MakeSquare(") {
		t.Error("declaration was not renamed")
	}
	runGo(t, module, "vet", "./...")
}

// Rule 4: rename -json prints the written files as a JSON array of strings.
func TestRefactorRenameJSON(t *testing.T) {
	module := copyModule(t)
	result := runMVC(t, module, "refactor", "rename", "-root", module, "-json", newSquareSymbol, "MakeSquare")
	expectExit(t, result, 0)
	var written []string
	decodeJSON(t, result, &written)
	if !slices.Contains(written, filepath.Join(module, "alpha", "alpha.go")) {
		t.Errorf("written = %v, want alpha/alpha.go among them", written)
	}
}

// Rule 3: a rename conflict is an ordinary error with exit 2 and nothing written.
func TestRefactorRenameConflict(t *testing.T) {
	module := copyModule(t)
	before := readFile(t, module, "shapes/shapes.go")
	result := runMVC(t, module, "refactor", "rename", "-root", module, newSquareSymbol, "Abandoned")
	expectOrdinaryError(t, result)
	if readFile(t, module, "shapes/shapes.go") != before {
		t.Error("file changed despite the conflict")
	}
}
