package deadcode_test

import (
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/AleksanderMako/mechanised-vibe-coding/deadcode"
)

// Import paths of the fixture packages.
const (
	appPackage     = "example.com/fixture/cmd/app"
	libPackage     = "example.com/fixture/lib"
	libonlyPackage = "example.com/libonly"
)

// Fixture files, relative to the module root with forward slashes.
const (
	mainFile     = "cmd/app/main.go"
	libFile      = "lib/lib.go"
	testOnlyFile = "lib/testonly.go"
)

// A test file for the libonly fixture whose only test calls Helper, so tests alone give the module a root.
const (
	libonlyTestFile    = "libonly_test.go"
	libonlyTestContent = `package libonly

import "testing"

func TestHelper(t *testing.T) {
	if Helper() != 1 {
		t.Fatal("Helper changed")
	}
}
`
)

// copyFixture copies testdata/<name> into a fresh temp dir and returns its symlink free absolute path.
func copyFixture(t *testing.T, name string) string {
	t.Helper()
	source := filepath.Join("testdata", name)
	target := filepath.Join(t.TempDir(), name)
	err := filepath.WalkDir(source, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		relative, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}
		destination := filepath.Join(target, relative)
		if entry.IsDir() {
			return os.MkdirAll(destination, 0o755)
		}
		content, err := os.ReadFile(path)
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

// writeFile writes content to root/relative, creating parent directories.
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

// readFile returns the content of root/relative.
func readFile(t *testing.T, root, relative string) string {
	t.Helper()
	content, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(relative)))
	if err != nil {
		t.Fatal(err)
	}
	return string(content)
}

// replaceInFile swaps the single occurrence of old for replacement, failing when old is not there exactly once.
func replaceInFile(t *testing.T, root, relative, old, replacement string) {
	t.Helper()
	content := readFile(t, root, relative)
	if count := strings.Count(content, old); count != 1 {
		t.Fatalf("%s: want exactly one %q, found %d", relative, old, count)
	}
	writeFile(t, root, relative, strings.Replace(content, old, replacement, 1))
}

// runGit runs git in dir with a fixed identity, fails the test on error and returns trimmed stdout.
func runGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	fullArgs := append([]string{
		"-c", "user.name=t",
		"-c", "user.email=t@example.com",
		"-c", "commit.gpgsign=false",
	}, args...)
	command := exec.Command("git", fullArgs...)
	command.Dir = dir
	output, err := command.Output()
	if err != nil {
		stderr := ""
		if exitErr, ok := err.(*exec.ExitError); ok {
			stderr = string(exitErr.Stderr)
		}
		t.Fatalf("git %s: %v\n%s%s", strings.Join(args, " "), err, output, stderr)
	}
	return strings.TrimSpace(string(output))
}

// initRepo turns root into a git repository on branch main without committing.
func initRepo(t *testing.T, root string) {
	t.Helper()
	runGit(t, root, "init", "-q", "-b", "main")
}

// commitAll stages everything and commits it, returning the new commit hash.
func commitAll(t *testing.T, root, subject string) string {
	t.Helper()
	runGit(t, root, "add", "-A")
	runGit(t, root, "commit", "-q", "--no-verify", "-m", subject)
	return runGit(t, root, "rev-parse", "HEAD")
}

// assertOnlyMainWorktree checks Orphaned left no temporary worktree registered in the repository.
func assertOnlyMainWorktree(t *testing.T, root string) {
	t.Helper()
	listing := runGit(t, root, "worktree", "list", "--porcelain")
	var worktrees []string
	for _, line := range strings.Split(listing, "\n") {
		if path, ok := strings.CutPrefix(line, "worktree "); ok {
			worktrees = append(worktrees, path)
		}
	}
	if len(worktrees) != 1 {
		t.Fatalf("want exactly one worktree after Orphaned, got %d:\n%s", len(worktrees), listing)
	}
	resolved, err := filepath.EvalSymlinks(worktrees[0])
	if err != nil {
		t.Fatalf("resolve worktree path: %v", err)
	}
	if resolved != root {
		t.Errorf("the remaining worktree is %s, want the repository root %s", resolved, root)
	}
}

// funcLineIn returns the line of the func keyword that follows "// marker:<name>" in content.
func funcLineIn(t *testing.T, content, relative, name string) int {
	t.Helper()
	marker := "// marker:" + name
	lines := strings.Split(content, "\n")
	found := 0
	for index, line := range lines {
		if strings.TrimSpace(line) != marker {
			continue
		}
		if found != 0 {
			t.Fatalf("%s: marker %q appears twice", relative, marker)
		}
		if index+1 >= len(lines) || !strings.HasPrefix(lines[index+1], "func") {
			t.Fatalf("%s: marker %q is not followed by a func line", relative, marker)
		}
		// Slice index i holds line i+1, so the func line right after the marker is i+2.
		found = index + 2
	}
	if found == 0 {
		t.Fatalf("%s: marker %q not found", relative, marker)
	}
	return found
}

// expectFunc builds the Func the analysis should report for the marked function in root/relative as it is on disk now.
func expectFunc(t *testing.T, root, packagePath, relative, name string) deadcode.Func {
	t.Helper()
	return expectFuncIn(t, readFile(t, root, relative), packagePath, relative, name)
}

// expectFuncIn builds the expected Func from a saved copy of a file's content, for files that change between revisions.
func expectFuncIn(t *testing.T, content, packagePath, relative, name string) deadcode.Func {
	t.Helper()
	return deadcode.Func{
		Package: packagePath,
		Name:    name,
		File:    relative,
		Line:    funcLineIn(t, content, relative, name),
	}
}

// sortLikeDead orders funcs by Package, then File, then Line, the order the contract promises.
func sortLikeDead(funcs []deadcode.Func) []deadcode.Func {
	sorted := append([]deadcode.Func(nil), funcs...)
	sort.SliceStable(sorted, func(i, j int) bool { return lessLikeDead(sorted[i], sorted[j]) })
	return sorted
}

// lessLikeDead is the Package, File, Line ordering.
func lessLikeDead(a, b deadcode.Func) bool {
	if a.Package != b.Package {
		return a.Package < b.Package
	}
	if a.File != b.File {
		return a.File < b.File
	}
	return a.Line < b.Line
}

// assertSortedLikeDead checks funcs are in Package, File, Line order.
func assertSortedLikeDead(t *testing.T, label string, funcs []deadcode.Func) {
	t.Helper()
	for index := 1; index < len(funcs); index++ {
		if lessLikeDead(funcs[index], funcs[index-1]) {
			t.Errorf("%s: not sorted by Package, File, Line at %d: %+v before %+v", label, index, funcs[index-1], funcs[index])
		}
	}
}

// assertFuncs compares funcs to want exactly, including order, File and Line.
func assertFuncs(t *testing.T, label string, got, want []deadcode.Func) {
	t.Helper()
	if len(got) == len(want) {
		same := true
		for index := range got {
			if got[index] != want[index] {
				same = false
			}
		}
		if same {
			return
		}
	}
	t.Errorf("%s:\n got: %s\nwant: %s", label, describeFuncs(got), describeFuncs(want))
}

// describeFuncs prints one Func per line so a failing comparison is easy to read.
func describeFuncs(funcs []deadcode.Func) string {
	if len(funcs) == 0 {
		return "(none)"
	}
	var builder strings.Builder
	for _, function := range funcs {
		builder.WriteString("\n  " + function.Package + " " + function.Name + " " + function.File + ":" + strconv.Itoa(function.Line))
	}
	return builder.String()
}

// findByName returns the Func in funcs with the given package and name, and whether it was there.
func findByName(funcs []deadcode.Func, packagePath, name string) (deadcode.Func, bool) {
	for _, function := range funcs {
		if function.Package == packagePath && function.Name == name {
			return function, true
		}
	}
	return deadcode.Func{}, false
}
