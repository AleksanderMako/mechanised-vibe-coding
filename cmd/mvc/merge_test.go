package main_test

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/AleksanderMako/mechanised-vibe-coding/mergeinfo"
)

// greetingFile returns a small Go file whose single return line both branches change.
func greetingFile(greeting string) string {
	return "package app\n\n// Greet says hello.\nfunc Greet() string {\n\treturn \"" + greeting + "\"\n}\n"
}

// newConflictedRepo builds a repo with a merge stopped on a conflict in app.go and returns its root.
func newConflictedRepo(t *testing.T) string {
	t.Helper()
	repo := newCleanRepo(t)
	runGit(t, repo, "checkout", "-q", "-b", "feature")
	writeFile(t, repo, "app.go", greetingFile("hi from feature"))
	runGit(t, repo, "commit", "-q", "-am", "feature greeting")
	runGit(t, repo, "checkout", "-q", "main")
	writeFile(t, repo, "app.go", greetingFile("hi from main"))
	runGit(t, repo, "commit", "-q", "-am", "main greeting")

	ctx, cancel := context.WithTimeout(context.Background(), commandTimeout)
	defer cancel()
	output, err := gitCommand(ctx, repo, "merge", "--no-commit", "feature").CombinedOutput()
	if err == nil || !strings.Contains(string(output), "CONFLICT") {
		t.Fatalf("merge did not stop on a conflict: %v\n%s", err, output)
	}
	return repo
}

// newCleanRepo builds a repo on branch main with one commit of app.go and no merge in progress.
func newCleanRepo(t *testing.T) string {
	t.Helper()
	repo := realTempDir(t)
	runGit(t, repo, "init", "-q", "-b", "main")
	writeFile(t, repo, "go.mod", "module example.com/app\n\ngo 1.26\n")
	writeFile(t, repo, "app.go", greetingFile("hello"))
	runGit(t, repo, "add", ".")
	runGit(t, repo, "commit", "-q", "-m", "base")
	return repo
}

// Rules 3 and 5: conflicts found exit 1 with a line per hunk naming the file and both sides.
func TestMergeExplainText(t *testing.T) {
	repo := newConflictedRepo(t)
	result := runMVC(t, repo, "merge", "explain", "-root", repo)
	expectExit(t, result, 1)
	for _, line := range nonEmptyLines(result.stdout) {
		if strings.Contains(line, "app.go") && strings.Contains(line, "ours=") && strings.Contains(line, "theirs=") {
			return
		}
	}
	t.Errorf("no line names app.go with ours= and theirs=:\n%s", result.describe())
}

// Rule 4: merge explain -json decodes into []mergeinfo.Hunk.
func TestMergeExplainJSON(t *testing.T) {
	repo := newConflictedRepo(t)
	result := runMVC(t, repo, "merge", "explain", "-root", repo, "-json")
	expectExit(t, result, 1)
	var hunks []mergeinfo.Hunk
	decodeJSON(t, result, &hunks)
	if len(hunks) != 1 || hunks[0].File != filepath.Join(repo, "app.go") {
		t.Fatalf("hunks = %+v, want one in app.go", hunks)
	}
	if !strings.Contains(hunks[0].Ours, "hi from main") || !strings.Contains(hunks[0].Theirs, "hi from feature") {
		t.Errorf("hunk sides = %q / %q", hunks[0].Ours, hunks[0].Theirs)
	}
}

// Rule 3: no conflict in progress is an ordinary error, exit 2.
func TestMergeExplainCleanRepoIsError(t *testing.T) {
	repo := newCleanRepo(t)
	expectOrdinaryError(t, runMVC(t, repo, "merge", "explain", "-root", repo))
}

// Rule 1: merge explain defaults -root to the current directory.
func TestMergeExplainDefaultRoot(t *testing.T) {
	repo := newConflictedRepo(t)
	result := runMVC(t, repo, "merge", "explain", "-json")
	expectExit(t, result, 1)
	var hunks []mergeinfo.Hunk
	decodeJSON(t, result, &hunks)
	if len(hunks) != 1 {
		t.Errorf("hunks = %+v, want one", hunks)
	}
}
