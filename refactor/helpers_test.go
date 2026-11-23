package refactor_test

import (
	"errors"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"testing"

	"github.com/AleksanderMako/machanised-vibe-coding/editset"
	"github.com/AleksanderMako/machanised-vibe-coding/symbols"
)

// fixtureModulePath is the module path every testdata module declares.
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

// importPath gives the fixture import path of a package directory.
func importPath(packageDir string) string {
	return fixtureModulePath + "/" + packageDir
}

// tree maps slash separated paths relative to the fixture root to file contents.
type tree map[string]string

// readTree reads every file under root so a later state can be compared against it.
func readTree(t *testing.T, root string) tree {
	t.Helper()
	files := tree{}
	err := filepath.WalkDir(root, func(walkedPath string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil || entry.IsDir() {
			return walkErr
		}
		relative, err := filepath.Rel(root, walkedPath)
		if err != nil {
			return err
		}
		content, err := os.ReadFile(walkedPath)
		if err != nil {
			return err
		}
		files[filepath.ToSlash(relative)] = string(content)
		return nil
	})
	if err != nil {
		t.Fatalf("read tree %s: %v", root, err)
	}
	return files
}

// readFile returns the content of root/relative.
func readFile(t *testing.T, root, relative string) string {
	t.Helper()
	content, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(relative)))
	if err != nil {
		t.Fatalf("read %s: %v", relative, err)
	}
	return string(content)
}

// writeFile creates root/relative with content, used to add a file to a fixture copy.
func writeFile(t *testing.T, root, relative, content string) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(relative))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// absolutePaths turns slash relative fixture paths into sorted absolute paths.
func absolutePaths(root string, relatives []string) []string {
	var paths []string
	for _, relative := range relatives {
		paths = append(paths, filepath.Join(root, filepath.FromSlash(relative)))
	}
	sort.Strings(paths)
	return paths
}

// assertTreeUnchanged checks that a failed operation left every file byte for byte as before.
func assertTreeUnchanged(t *testing.T, root string, before tree) {
	t.Helper()
	after := readTree(t, root)
	for path, content := range before {
		afterContent, exists := after[path]
		if !exists {
			t.Errorf("%s was deleted but the operation failed", path)
			continue
		}
		if afterContent != content {
			t.Errorf("%s changed but the operation failed:\n%s", path, afterContent)
		}
	}
	for path := range after {
		if _, existed := before[path]; !existed {
			t.Errorf("%s was created but the operation failed", path)
		}
	}
}

// assertReturnedPathsShape checks rule 3: absolute, sorted and without duplicates.
func assertReturnedPathsShape(t *testing.T, got []string) {
	t.Helper()
	for index, path := range got {
		if !filepath.IsAbs(path) {
			t.Errorf("returned path %q is not absolute", path)
		}
		if index > 0 && got[index-1] >= path {
			t.Errorf("returned paths are not sorted and unique: %q before %q", got[index-1], path)
		}
	}
}

// assertWritten checks the returned list against what really changed on disk.
// wantChanged are the files created or modified, wantDeleted the files removed; a deleted file may or may not be listed.
func assertWritten(t *testing.T, root string, before tree, got []string, wantChanged, wantDeleted []string) {
	t.Helper()
	assertReturnedPathsShape(t, got)
	after := readTree(t, root)
	var changed, deleted []string
	for path, content := range after {
		if beforeContent, existed := before[path]; !existed || beforeContent != content {
			changed = append(changed, path)
		}
	}
	for path := range before {
		if _, exists := after[path]; !exists {
			deleted = append(deleted, path)
		}
	}
	sort.Strings(changed)
	sort.Strings(deleted)
	wantChangedSorted := slices.Sorted(slices.Values(wantChanged))
	wantDeletedSorted := slices.Sorted(slices.Values(wantDeleted))
	if !slices.Equal(changed, wantChangedSorted) {
		t.Errorf("files changed on disk: want %v, got %v", wantChangedSorted, changed)
	}
	if !slices.Equal(deleted, wantDeletedSorted) {
		t.Errorf("files deleted on disk: want %v, got %v", wantDeletedSorted, deleted)
	}
	changedAbsolute := absolutePaths(root, changed)
	deletedAbsolute := absolutePaths(root, deleted)
	for _, path := range changedAbsolute {
		if !slices.Contains(got, path) {
			t.Errorf("changed file %s is missing from the returned list %v", path, got)
		}
	}
	for _, path := range got {
		if !slices.Contains(changedAbsolute, path) && !slices.Contains(deletedAbsolute, path) {
			t.Errorf("returned file %s did not change on disk", path)
		}
	}
}

// assertModuleBuilds runs go build and go vet over the whole fixture module.
func assertModuleBuilds(t *testing.T, root string) {
	t.Helper()
	for _, args := range [][]string{{"build", "./..."}, {"vet", "./..."}} {
		command := exec.Command("go", args...)
		command.Dir = root
		command.Env = append(os.Environ(), "GOWORK=off", "GOFLAGS=")
		output, err := command.CombinedOutput()
		if err != nil {
			t.Errorf("go %s in the rewritten module failed: %v\n%s", strings.Join(args, " "), err, output)
		}
	}
}

// normalizeSpace collapses every run of whitespace so gofmt alignment does not matter.
func normalizeSpace(text string) string {
	return strings.Join(strings.Fields(text), " ")
}

// assertContains checks each fragment occurs in root/relative, ignoring whitespace differences.
func assertContains(t *testing.T, root, relative string, fragments ...string) {
	t.Helper()
	content := normalizeSpace(readFile(t, root, relative))
	for _, fragment := range fragments {
		if !strings.Contains(content, normalizeSpace(fragment)) {
			t.Errorf("%s: want it to contain %q, got:\n%s", relative, fragment, readFile(t, root, relative))
		}
	}
}

// assertNotContains checks no fragment occurs in root/relative, ignoring whitespace differences.
func assertNotContains(t *testing.T, root, relative string, fragments ...string) {
	t.Helper()
	content := normalizeSpace(readFile(t, root, relative))
	for _, fragment := range fragments {
		if strings.Contains(content, normalizeSpace(fragment)) {
			t.Errorf("%s: want it not to contain %q, got:\n%s", relative, fragment, readFile(t, root, relative))
		}
	}
}

// parseFile parses root/relative and fails the test when it is not valid Go.
func parseFile(t *testing.T, root, relative string) *ast.File {
	t.Helper()
	file, err := parser.ParseFile(token.NewFileSet(), filepath.Join(root, filepath.FromSlash(relative)), nil, 0)
	if err != nil {
		t.Fatalf("parse %s: %v", relative, err)
	}
	return file
}

// identifierCount counts identifiers called name in a file; comments and strings are not identifiers.
func identifierCount(t *testing.T, root, relative, name string) int {
	t.Helper()
	count := 0
	ast.Inspect(parseFile(t, root, relative), func(node ast.Node) bool {
		if identifier, isIdentifier := node.(*ast.Ident); isIdentifier && identifier.Name == name {
			count++
		}
		return true
	})
	return count
}

// assertPackageClause checks the package name declared by root/relative.
func assertPackageClause(t *testing.T, root, relative, want string) {
	t.Helper()
	if got := parseFile(t, root, relative).Name.Name; got != want {
		t.Errorf("%s: want package clause %q, got %q", relative, want, got)
	}
}

// assertNotBuildError checks a failure that must happen before any write is not reported as a build failure.
func assertNotBuildError(t *testing.T, err error) {
	t.Helper()
	var buildErr *editset.BuildError
	if errors.As(err, &buildErr) {
		t.Errorf("want an error raised before writing, got a build error: %v", err)
	}
}

// assertErrorIs checks err wraps want.
func assertErrorIs(t *testing.T, err, want error) {
	t.Helper()
	if !errors.Is(err, want) {
		t.Fatalf("want error wrapping %q, got %v", want, err)
	}
}
