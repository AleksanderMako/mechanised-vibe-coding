package mergeinfo_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/AleksanderMako/machanised-vibe-coding/mergeinfo"
)

// greetFunc is the function both sides of most scenarios fight over; it starts on line 3 of baseApp.
const greetFunc = `// Greet says hello.
func Greet(name string) string {
	prefix := "hello "
	return prefix + name
}
`

// counterType and countMethod follow greetFunc so a deletion of greetFunc lands next to them.
const counterType = `// Counter counts.
type Counter struct {
	n int
}
`

const countMethod = `// Count returns the count.
func (c *Counter) Count() int {
	return c.n
}
`

// baseApp is the shared starting file; greetFunc covers lines 3-7 and line 8 is blank.
const baseApp = "package app\n\n" + greetFunc + "\n" + counterType + "\n" + countMethod

// gitIdentity pins author, signing and conflict style so the user's global git config cannot change the markers.
var gitIdentity = []string{
	"-c", "user.name=t",
	"-c", "user.email=t@example.com",
	"-c", "commit.gpgsign=false",
	"-c", "core.autocrlf=false",
	"-c", "merge.conflictStyle=merge",
	"-c", "advice.mergeConflict=false",
}

// gitCommand builds a git command in dir with the pinned identity in front of args.
func gitCommand(dir string, args ...string) *exec.Cmd {
	command := exec.Command("git", append(append([]string{}, gitIdentity...), args...)...)
	command.Dir = dir
	// No editor may open during rebase or cherry-pick.
	command.Env = append(os.Environ(), "GIT_EDITOR=true")
	return command
}

// runGit runs git in dir, fails the test on a non-zero exit and returns trimmed stdout.
func runGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	output, err := gitCommand(dir, args...).Output()
	if err != nil {
		stderr := ""
		if exitErr, ok := err.(*exec.ExitError); ok {
			stderr = string(exitErr.Stderr)
		}
		t.Fatalf("git %s: %v\n%s%s", strings.Join(args, " "), err, output, stderr)
	}
	return strings.TrimSpace(string(output))
}

// runGitExpectingConflict runs a merge, rebase or cherry-pick that must stop on a conflict, so a zero exit is a setup bug.
func runGitExpectingConflict(t *testing.T, dir string, args ...string) {
	t.Helper()
	output, err := gitCommand(dir, args...).CombinedOutput()
	if err == nil {
		t.Fatalf("git %s: expected a conflict, but it succeeded\n%s", strings.Join(args, " "), output)
	}
	if !strings.Contains(string(output), "CONFLICT") {
		t.Fatalf("git %s: failed without a conflict: %v\n%s", strings.Join(args, " "), err, output)
	}
}

// newRepo makes an empty repository on branch main in a symlink free temp dir.
func newRepo(t *testing.T) string {
	t.Helper()
	// Temp dirs on macOS sit behind a symlink; resolving it keeps path comparisons stable.
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatalf("resolve temp dir: %v", err)
	}
	runGit(t, root, "init", "-q", "-b", "main")
	return root
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
		t.Fatalf("%s: want exactly one %q, found %d in:\n%s", relative, old, count, content)
	}
	writeFile(t, root, relative, strings.Replace(content, old, replacement, 1))
}

// commitAll stages everything and commits it with subject, returning the new commit hash.
func commitAll(t *testing.T, root, subject string) string {
	t.Helper()
	runGit(t, root, "add", "-A")
	runGit(t, root, "commit", "-q", "--no-verify", "-m", subject)
	return runGit(t, root, "rev-parse", "HEAD")
}

// newRepoWithBaseApp makes a repository whose first commit on main holds app.go as baseApp.
func newRepoWithBaseApp(t *testing.T) string {
	t.Helper()
	root := newRepo(t)
	writeFile(t, root, "app.go", baseApp)
	commitAll(t, root, "base")
	return root
}

// mergeTheirsIntoMain branches "theirs" off main, lets each side commit its edits, then merges theirs into main without committing.
func mergeTheirsIntoMain(t *testing.T, root string, theirEdits, ourEdits func(), mergeConfig ...string) {
	t.Helper()
	runGit(t, root, "checkout", "-q", "-b", "theirs")
	theirEdits()
	runGit(t, root, "checkout", "-q", "main")
	ourEdits()
	args := append(append([]string{}, mergeConfig...), "merge", "--no-commit", "theirs")
	runGitExpectingConflict(t, root, args...)
}

// containsLineNumber reports whether message mentions line as a standalone number.
func containsLineNumber(message string, line int) bool {
	return regexp.MustCompile(`(^|\D)` + strconv.Itoa(line) + `(\D|$)`).MatchString(message)
}

// markerFields is the part of a Hunk that ParseHunks fills in, used to compare without git-only fields.
type markerFields struct {
	File                       string
	Start, End                 int
	Ours, Base, Theirs         string
	OursDeleted, TheirsDeleted bool
}

// markersOf extracts the marker fields of a hunk.
func markersOf(hunk mergeinfo.Hunk) markerFields {
	return markerFields{
		File:          hunk.File,
		Start:         hunk.Start,
		End:           hunk.End,
		Ours:          hunk.Ours,
		Base:          hunk.Base,
		Theirs:        hunk.Theirs,
		OursDeleted:   hunk.OursDeleted,
		TheirsDeleted: hunk.TheirsDeleted,
	}
}

// hunksInFile returns the hunks whose File is path, in the order they were reported.
func hunksInFile(hunks []mergeinfo.Hunk, path string) []mergeinfo.Hunk {
	var found []mergeinfo.Hunk
	for _, hunk := range hunks {
		if hunk.File == path {
			found = append(found, hunk)
		}
	}
	return found
}

// onlyHunk asserts hunks holds exactly one hunk and returns it.
func onlyHunk(t *testing.T, hunks []mergeinfo.Hunk) mergeinfo.Hunk {
	t.Helper()
	if len(hunks) != 1 {
		t.Fatalf("want exactly 1 hunk, got %d: %+v", len(hunks), hunks)
	}
	return hunks[0]
}

// wantCommit is what a test expects of one reported commit; Date and Author are checked separately.
type wantCommit struct {
	Hash    string
	Subject string
	PR      int
}

// assertCommits compares reported commits, in order, to want and checks hash length, author and date.
func assertCommits(t *testing.T, got []mergeinfo.Commit, want []wantCommit) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("want %d commits %+v, got %d: %+v", len(want), want, len(got), got)
	}
	for index, commit := range got {
		if len(commit.Hash) != 40 {
			t.Errorf("commit %d: want a 40 character hash, got %q", index, commit.Hash)
		}
		if commit.Hash != want[index].Hash {
			t.Errorf("commit %d: want hash %s, got %s", index, want[index].Hash, commit.Hash)
		}
		if commit.Subject != want[index].Subject {
			t.Errorf("commit %d: want subject %q, got %q", index, want[index].Subject, commit.Subject)
		}
		if commit.PR != want[index].PR {
			t.Errorf("commit %d: want PR %d, got %d", index, want[index].PR, commit.PR)
		}
		if commit.Author != "t" {
			t.Errorf("commit %d: want author %q, got %q", index, "t", commit.Author)
		}
		if commit.Date.IsZero() {
			t.Errorf("commit %d: Date is zero", index)
		}
	}
}
