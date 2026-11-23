package editset_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"testing"

	"github.com/AleksanderMako/mechanised-vibe-coding/editset"
)

// assertBuildError checks err is a *BuildError naming mustInclude and nothing outside allowed.
func assertBuildError(t *testing.T, err error, mustInclude string, allowed ...string) *editset.BuildError {
	t.Helper()
	var buildErr *editset.BuildError
	if !errors.As(err, &buildErr) {
		t.Fatalf("want *editset.BuildError, got %T: %v", err, err)
	}
	if buildErr.Error() == "" {
		t.Errorf("BuildError.Error() is empty")
	}
	if strings.TrimSpace(buildErr.Output) == "" {
		t.Errorf("BuildError.Output is empty")
	}
	if !slices.Contains(buildErr.Packages, mustInclude) {
		t.Errorf("want BuildError.Packages to include %s, got %v", mustInclude, buildErr.Packages)
	}
	for _, dir := range buildErr.Packages {
		if !slices.Contains(allowed, dir) {
			t.Errorf("BuildError.Packages lists %s, which holds no touched .go file", dir)
		}
	}
	return buildErr
}

func TestRule7_WritesFinalContentAndPreservesFileMode(t *testing.T) {
	root := copyFixture(t, "module")
	notes := filepath.Join(root, "notes.txt")
	script := filepath.Join(root, "run.sh")
	if err := os.Chmod(notes, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(script, 0o755); err != nil {
		t.Fatal(err)
	}
	edits := []editset.Edit{
		{File: "notes.txt", Old: "beta", New: "BETA"},
		{File: "run.sh", Old: "echo hi", New: "echo bye"},
		{File: "notes.txt", Old: "gamma\n", New: ""},
		{File: "notes.txt", Old: "BETA", New: "b e t a"},
	}
	result, err := editset.Apply(context.Background(), edits, editset.Options{Root: root})
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if got, want := readFile(t, notes), "alpha\nb e t a\nalpha\nprice (USD) $5.00 [x]*\n"; got != want {
		t.Errorf("notes.txt: want %q, got %q", want, got)
	}
	if got, want := readFile(t, script), "#!/bin/sh\necho bye\n"; got != want {
		t.Errorf("run.sh: want %q, got %q", want, got)
	}
	for path, wantMode := range map[string]os.FileMode{notes: 0o600, script: 0o755} {
		info, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		if got := info.Mode().Perm(); got != wantMode {
			t.Errorf("%s: want mode %v, got %v", path, wantMode, got)
		}
	}
	if want := []string{notes, script}; !slices.Equal(result.Touched, want) {
		t.Errorf("Touched: want %v, got %v", want, result.Touched)
	}
	if len(result.Packages) != 0 {
		t.Errorf("Packages: want empty when no .go file is touched, got %v", result.Packages)
	}
}

func TestRule7_EmptyNewDeletesOld(t *testing.T) {
	root := copyFixture(t, "module")
	notes := filepath.Join(root, "notes.txt")
	edits := []editset.Edit{{File: "notes.txt", Old: "price (USD) $5.00 [x]*\n"}}
	if _, err := editset.Apply(context.Background(), edits, editset.Options{Root: root}); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if got, want := readFile(t, notes), "alpha\nbeta\nalpha\ngamma\n"; got != want {
		t.Errorf("want %q, got %q", want, got)
	}
}

func TestRule8_GoimportsRemovesImportLeftUnusedByEdit(t *testing.T) {
	root := copyFixture(t, "module")
	greet := filepath.Join(root, "greet", "greet.go")
	edits := []editset.Edit{{File: "greet/greet.go", Old: "strings.ToUpper(name)", New: "name"}}
	result, err := editset.Apply(context.Background(), edits, editset.Options{Root: root})
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	got := readFile(t, greet)
	if strings.Contains(got, `"strings"`) {
		t.Errorf("unused import \"strings\" was not removed:\n%s", got)
	}
	if !strings.Contains(got, `"fmt"`) {
		t.Errorf("used import \"fmt\" disappeared:\n%s", got)
	}
	if !strings.Contains(got, `return fmt.Sprintf("Hello, %s!", name)`) {
		t.Errorf("edit is missing:\n%s", got)
	}
	if want := []string{filepath.Join(root, "greet")}; !slices.Equal(result.Packages, want) {
		t.Errorf("Packages: want %v, got %v", want, result.Packages)
	}
}

func TestRule8_GoimportsFormatsTouchedGoFiles(t *testing.T) {
	root := copyFixture(t, "module")
	calc := filepath.Join(root, "calc", "calc.go")
	edits := []editset.Edit{{File: "calc/calc.go", Old: "func Add(a, b int) int {", New: "func Add(a,b    int)   int {"}}
	if _, err := editset.Apply(context.Background(), edits, editset.Options{Root: root}); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if got := readFile(t, calc); !strings.Contains(got, "func Add(a, b int) int {") {
		t.Errorf("file was not formatted:\n%s", got)
	}
}

func TestRule8_NonGoFilesAreNotFormatted(t *testing.T) {
	root := copyFixture(t, "module")
	notes := filepath.Join(root, "notes.txt")
	edits := []editset.Edit{{File: "notes.txt", Old: "beta\n", New: "func  Odd( ) {   }\t \n"}}
	if _, err := editset.Apply(context.Background(), edits, editset.Options{Root: root}); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if got, want := readFile(t, notes), "alpha\nfunc  Odd( ) {   }\t \nalpha\ngamma\nprice (USD) $5.00 [x]*\n"; got != want {
		t.Errorf("want %q, got %q", want, got)
	}
}

func TestRule8_NoFormatKeepsExactBytes(t *testing.T) {
	root := copyFixture(t, "module")
	calc := filepath.Join(root, "calc", "calc.go")
	original := readFile(t, calc)
	edits := []editset.Edit{{File: "calc/calc.go", Old: "func Add(a, b int) int {", New: "func Add(a,b    int)   int {"}}
	if _, err := editset.Apply(context.Background(), edits, editset.Options{Root: root, NoFormat: true}); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	want := strings.Replace(original, "func Add(a, b int) int {", "func Add(a,b    int)   int {", 1)
	if got := readFile(t, calc); got != want {
		t.Errorf("want exact edited bytes:\n%s\ngot:\n%s", want, got)
	}
}

func TestRule8_NoFormatLeavesUnusedImportSoBuildFails(t *testing.T) {
	root := copyFixture(t, "module")
	greetDir := filepath.Join(root, "greet")
	greet := filepath.Join(greetDir, "greet.go")
	snapshot := snapshotFiles(t, greet)
	edits := []editset.Edit{{File: "greet/greet.go", Old: "strings.ToUpper(name)", New: "name"}}
	_, err := editset.Apply(context.Background(), edits, editset.Options{Root: root, NoFormat: true})
	assertBuildError(t, err, greetDir, greetDir)
	assertUnchanged(t, snapshot)
}

func TestRule8_UnparsableEditIsBuildErrorAndRestores(t *testing.T) {
	for _, noBuild := range []bool{false, true} {
		name := "with build"
		if noBuild {
			name = "with NoBuild"
		}
		t.Run(name, func(t *testing.T) {
			root := copyFixture(t, "module")
			calcDir := filepath.Join(root, "calc")
			calc := filepath.Join(calcDir, "calc.go")
			notes := filepath.Join(root, "notes.txt")
			snapshot := snapshotFiles(t, calc, notes)
			edits := []editset.Edit{
				{File: "notes.txt", Old: "beta", New: "BETA"},
				{File: "calc/calc.go", Old: "return a + b\n}", New: "return a + b\n"},
			}
			_, err := editset.Apply(context.Background(), edits, editset.Options{Root: root, NoBuild: noBuild})
			assertBuildError(t, err, calcDir, calcDir)
			assertUnchanged(t, snapshot)
		})
	}
}

func TestRule9_BuildFailureRestoresOriginalBytesOfEveryTouchedFile(t *testing.T) {
	root := copyFixture(t, "module")
	// A deliberately unformatted file proves restoration writes the original bytes, not a formatted copy.
	messyDir := filepath.Join(root, "messy")
	if err := os.MkdirAll(messyDir, 0o755); err != nil {
		t.Fatal(err)
	}
	messy := filepath.Join(messyDir, "messy.go")
	if err := os.WriteFile(messy, []byte("package messy\nfunc  Messy( ) int {return    1}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	notes := filepath.Join(root, "notes.txt")
	greetDir := filepath.Join(root, "greet")
	greet := filepath.Join(greetDir, "greet.go")
	calcDir := filepath.Join(root, "calc")
	calc := filepath.Join(calcDir, "calc.go")
	snapshot := snapshotFiles(t, notes, greet, calc, messy)

	edits := []editset.Edit{
		{File: "notes.txt", Old: "beta", New: "BETA"},
		{File: "greet/greet.go", Old: "strings.ToUpper(name)", New: "name"},
		{File: "messy/messy.go", Old: "return    1", New: "return    2"},
		{File: "calc/calc.go", Old: "return a + b", New: "return a + undefinedName"},
	}
	_, err := editset.Apply(context.Background(), edits, editset.Options{Root: root})
	assertBuildError(t, err, calcDir, calcDir, greetDir, messyDir)
	assertUnchanged(t, snapshot)
}

func TestRule9_SeveralFailingPackages(t *testing.T) {
	root := copyFixture(t, "module")
	calcDir := filepath.Join(root, "calc")
	greetDir := filepath.Join(root, "greet")
	snapshot := snapshotFiles(t, filepath.Join(calcDir, "calc.go"), filepath.Join(greetDir, "greet.go"))
	edits := []editset.Edit{
		{File: "calc/calc.go", Old: "return a + b", New: "return a + undefinedInCalc"},
		{File: "greet/greet.go", Old: "strings.ToUpper(name)", New: "undefinedInGreet"},
	}
	_, err := editset.Apply(context.Background(), edits, editset.Options{Root: root})
	buildErr := assertBuildError(t, err, calcDir, calcDir, greetDir)
	if !slices.Contains(buildErr.Packages, greetDir) {
		t.Errorf("want BuildError.Packages to include %s, got %v", greetDir, buildErr.Packages)
	}
	assertUnchanged(t, snapshot)
}

func TestRule9_TestFileIsCompiledWithVet(t *testing.T) {
	root := copyFixture(t, "module")
	greetDir := filepath.Join(root, "greet")
	greetTest := filepath.Join(greetDir, "greet_test.go")
	snapshot := snapshotFiles(t, greetTest)
	// A type error only inside a _test.go file: go build would miss it, go vet does not.
	edits := []editset.Edit{{File: "greet/greet_test.go", Old: `Hello("bob")`, New: `Hello(42)`}}
	_, err := editset.Apply(context.Background(), edits, editset.Options{Root: root})
	assertBuildError(t, err, greetDir, greetDir)
	assertUnchanged(t, snapshot)
}

func TestRule9_TestFileTouchCountsAsItsDirectory(t *testing.T) {
	root := copyFixture(t, "module")
	greetDir := filepath.Join(root, "greet")
	greetTest := filepath.Join(greetDir, "greet_test.go")
	edits := []editset.Edit{{File: "greet/greet_test.go", Old: `Hello("bob")`, New: `Hello("BOB")`}}
	result, err := editset.Apply(context.Background(), edits, editset.Options{Root: root})
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if want := []string{greetDir}; !slices.Equal(result.Packages, want) {
		t.Errorf("Packages: want %v, got %v", want, result.Packages)
	}
	if want := []string{greetTest}; !slices.Equal(result.Touched, want) {
		t.Errorf("Touched: want %v, got %v", want, result.Touched)
	}
	if !strings.Contains(readFile(t, greetTest), `Hello("BOB")`) {
		t.Errorf("edit is missing from the test file")
	}
}

func TestRule9_RootPackageIsBuilt(t *testing.T) {
	root := copyFixture(t, "module")
	edits := []editset.Edit{{File: "root.go", Old: `return "root"`, New: `return "top"`}}
	result, err := editset.Apply(context.Background(), edits, editset.Options{Root: root})
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if want := []string{root}; !slices.Equal(result.Packages, want) {
		t.Errorf("Packages: want %v, got %v", want, result.Packages)
	}

	t.Run("a broken root package fails the build", func(t *testing.T) {
		rootFile := filepath.Join(root, "root.go")
		snapshot := snapshotFiles(t, rootFile)
		edits := []editset.Edit{{File: "root.go", Old: `return "top"`, New: `return 7`}}
		_, err := editset.Apply(context.Background(), edits, editset.Options{Root: root})
		assertBuildError(t, err, root, root)
		assertUnchanged(t, snapshot)
	})
}

func TestRule9_NoBuildSkipsTheBuild(t *testing.T) {
	root := copyFixture(t, "module")
	calc := filepath.Join(root, "calc", "calc.go")
	// Parses fine, so formatting succeeds, but it would not compile.
	edits := []editset.Edit{{File: "calc/calc.go", Old: "return a + b", New: "return a + undefinedName"}}
	result, err := editset.Apply(context.Background(), edits, editset.Options{Root: root, NoBuild: true})
	if err != nil {
		t.Fatalf("Apply with NoBuild: %v", err)
	}
	if !strings.Contains(readFile(t, calc), "return a + undefinedName") {
		t.Errorf("edit was not kept")
	}
	if len(result.Packages) != 0 {
		t.Errorf("Packages: want empty with NoBuild, got %v", result.Packages)
	}
	if want := []string{calc}; !slices.Equal(result.Touched, want) {
		t.Errorf("Touched: want %v, got %v", want, result.Touched)
	}
}

func TestRule11_TouchedInFirstEditOrderAndPackagesSorted(t *testing.T) {
	root := copyFixture(t, "module")
	greetDir := filepath.Join(root, "greet")
	calcDir := filepath.Join(root, "calc")
	greet := filepath.Join(greetDir, "greet.go")
	greetTest := filepath.Join(greetDir, "greet_test.go")
	notes := filepath.Join(root, "notes.txt")
	calc := filepath.Join(calcDir, "calc.go")
	rootFile := filepath.Join(root, "root.go")
	edits := []editset.Edit{
		{File: "greet/greet.go", Old: "Hello, %s!", New: "Hi, %s!"},
		{File: "notes.txt", Old: "beta", New: "BETA"},
		{File: calc, Old: "a + b", New: "b + a"},
		{File: "greet/greet.go", Old: "Hi, %s!", New: "Hey, %s!"},
		{File: "greet/greet_test.go", Old: `"Hello, BOB!"`, New: `"Hey, BOB!"`},
		{File: "root.go", Old: `"root"`, New: `"top"`},
		{File: "calc/calc.go", Old: "a - b", New: "-b + a"},
	}
	result, err := editset.Apply(context.Background(), edits, editset.Options{Root: root})
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if want := []string{greet, notes, calc, greetTest, rootFile}; !slices.Equal(result.Touched, want) {
		t.Errorf("Touched: want %v, got %v", want, result.Touched)
	}
	wantPackages := []string{greetDir, calcDir, root}
	sort.Strings(wantPackages)
	if !slices.Equal(result.Packages, wantPackages) {
		t.Errorf("Packages: want %v, got %v", wantPackages, result.Packages)
	}
	if got := readFile(t, greet); !strings.Contains(got, `"Hey, %s!"`) {
		t.Errorf("greet.go is missing the second edit:\n%s", got)
	}
}

func TestRule12_CancelledContextLeavesFilesUnchanged(t *testing.T) {
	root := copyFixture(t, "module")
	calc := filepath.Join(root, "calc", "calc.go")
	notes := filepath.Join(root, "notes.txt")
	snapshot := snapshotFiles(t, calc, notes)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	edits := []editset.Edit{
		{File: "notes.txt", Old: "beta", New: "BETA"},
		{File: "calc/calc.go", Old: "a + b", New: "b + a"},
	}
	// Either validation sees the cancellation and writes nothing, or the go command fails and files are restored.
	_, err := editset.Apply(ctx, edits, editset.Options{Root: root})
	if err == nil {
		t.Fatal("want an error from a cancelled context, got nil")
	}
	assertUnchanged(t, snapshot)
}
