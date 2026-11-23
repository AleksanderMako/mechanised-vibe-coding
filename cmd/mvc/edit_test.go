package main_test

import (
	"encoding/json"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/AleksanderMako/mechanised-vibe-coding/editset"
)

// versionEdits bumps the fixture version, an edit that applies cleanly.
const versionEdits = `[{"file": "shapes/shapes.go", "old": "const Version = \"1.0\"", "new": "const Version = \"1.1\""}]`

// brokenEdits targets text that is not in the file, so checking it gives a CheckError.
const brokenEdits = `[{"file": "shapes/shapes.go", "old": "const Version = \"9.9\"", "new": "const Version = \"1.1\""}]`

// Rule 7: edit check reads the edits file and exits 0 without writing.
func TestEditCheckValidEditsFile(t *testing.T) {
	module := copyModule(t)
	editsFile := writeFile(t, realTempDir(t), "edits.json", versionEdits)
	before := readFile(t, module, "shapes/shapes.go")

	result := runMVC(t, module, "edit", "check", "-root", module, editsFile)
	expectExit(t, result, 0)
	if readFile(t, module, "shapes/shapes.go") != before {
		t.Error("edit check wrote the file")
	}
}

// Rule 7: `-` reads the edits from stdin.
func TestEditCheckFromStdin(t *testing.T) {
	module := copyModule(t)
	result := runMVCWithStdin(t, module, versionEdits, "edit", "check", "-root", module, "-")
	expectExit(t, result, 0)
}

// Rules 3 and 7: a CheckError is a negative result, exit 1, printed in full, nothing written.
func TestEditCheckCheckErrorExitsOne(t *testing.T) {
	module := copyModule(t)
	editsFile := writeFile(t, realTempDir(t), "edits.json", brokenEdits)
	before := readFile(t, module, "shapes/shapes.go")

	result := runMVC(t, module, "edit", "check", "-root", module, editsFile)
	expectExit(t, result, 1)
	if !strings.Contains(result.stdout+result.stderr, "shapes.go") {
		t.Errorf("the failing edit's file is not reported:\n%s", result.describe())
	}
	if readFile(t, module, "shapes/shapes.go") != before {
		t.Error("edit check wrote the file")
	}
}

// Rule 4: with -json a CheckError still leaves nothing but JSON on stdout.
func TestEditCheckCheckErrorJSON(t *testing.T) {
	module := copyModule(t)
	editsFile := writeFile(t, realTempDir(t), "edits.json", brokenEdits)
	result := runMVC(t, module, "edit", "check", "-root", module, "-json", editsFile)
	expectExit(t, result, 1)
	if strings.TrimSpace(result.stdout) != "" && !json.Valid([]byte(result.stdout)) {
		t.Errorf("stdout is not JSON:\n%s", result.describe())
	}
}

// Rules 4 and 7: edit apply from a file with -json prints an editset.Result naming the written file.
func TestEditApplyJSON(t *testing.T) {
	module := copyModule(t)
	editsFile := writeFile(t, realTempDir(t), "edits.json", versionEdits)

	result := runMVC(t, module, "edit", "apply", "-root", module, "-json", editsFile)
	expectExit(t, result, 0)
	var applied editset.Result
	decodeJSON(t, result, &applied)
	wantFile := filepath.Join(module, "shapes", "shapes.go")
	if !slices.Contains(applied.Touched, wantFile) {
		t.Errorf("Touched = %v, want it to contain %s", applied.Touched, wantFile)
	}
	if !strings.Contains(readFile(t, module, "shapes/shapes.go"), `const Version = "1.1"`) {
		t.Error("edit was not written")
	}
}

// Rule 7: edit apply reads `-` from stdin and writes the edit.
func TestEditApplyFromStdin(t *testing.T) {
	module := copyModule(t)
	result := runMVCWithStdin(t, module, versionEdits, "edit", "apply", "-root", module, "-no-build", "-")
	expectExit(t, result, 0)
	if !strings.Contains(readFile(t, module, "shapes/shapes.go"), `const Version = "1.1"`) {
		t.Error("edit from stdin was not written")
	}
}

// Rules 3 and 7: edit apply with a CheckError exits 1 and writes nothing.
func TestEditApplyCheckErrorExitsOne(t *testing.T) {
	module := copyModule(t)
	before := readFile(t, module, "shapes/shapes.go")
	result := runMVCWithStdin(t, module, brokenEdits, "edit", "apply", "-root", module, "-")
	expectExit(t, result, 1)
	if readFile(t, module, "shapes/shapes.go") != before {
		t.Error("edit apply wrote the file despite a CheckError")
	}
}

// Rule 3: an edits file that is not a JSON array is an ordinary error, not a negative result.
func TestEditApplyMalformedJSONIsError(t *testing.T) {
	module := copyModule(t)
	result := runMVCWithStdin(t, module, `{"file": "x"}`, "edit", "apply", "-root", module, "-")
	expectOrdinaryError(t, result)
}
