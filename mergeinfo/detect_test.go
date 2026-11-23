package mergeinfo_test

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/AleksanderMako/mechanised-vibe-coding/mergeinfo"
)

// greetConflictEdits returns edits that make both sides rewrite the same line of Greet differently.
func greetConflictEdits(t *testing.T, root string) (theirEdits, ourEdits func()) {
	theirEdits = func() {
		replaceInFile(t, root, "app.go", `"hello "`, `"hey "`)
		commitAll(t, root, "their greet (#42)")
	}
	ourEdits = func() {
		replaceInFile(t, root, "app.go", `"hello "`, `"hi "`)
		commitAll(t, root, "our greet")
	}
	return theirEdits, ourEdits
}

// newFeatureAndMainDiverged leaves main and a "feature" branch with conflicting edits of Greet, on main, returning the feature commit.
func newFeatureAndMainDiverged(t *testing.T) (root, featureCommit string) {
	t.Helper()
	root = newRepoWithBaseApp(t)
	runGit(t, root, "checkout", "-q", "-b", "feature")
	replaceInFile(t, root, "app.go", `"hello "`, `"hey "`)
	featureCommit = commitAll(t, root, "feature greet (#42)")
	runGit(t, root, "checkout", "-q", "main")
	replaceInFile(t, root, "app.go", `"hello "`, `"hi "`)
	commitAll(t, root, "main greet")
	return root, featureCommit
}

func TestRule1_RootOutsideAGitWorkTreeIsAnOrdinaryError(t *testing.T) {
	t.Parallel()
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	_, err = mergeinfo.Detect(context.Background(), root)
	if err == nil {
		t.Fatal("Detect outside a git work tree: want an error, got nil")
	}
	if errors.Is(err, mergeinfo.ErrNoConflict) {
		t.Errorf("Detect outside a git work tree: want an ordinary error, got ErrNoConflict")
	}
}

func TestRule2_CleanRepositoryHasNoConflict(t *testing.T) {
	t.Parallel()
	root := newRepoWithBaseApp(t)
	_, err := mergeinfo.Detect(context.Background(), root)
	if !errors.Is(err, mergeinfo.ErrNoConflict) {
		t.Errorf("clean repository: want ErrNoConflict, got %v", err)
	}
}

func TestRule2_3_MergeIsDetected(t *testing.T) {
	t.Parallel()
	root := newRepoWithBaseApp(t)
	theirEdits, ourEdits := greetConflictEdits(t, root)
	mergeTheirsIntoMain(t, root, theirEdits, ourEdits)
	state, err := mergeinfo.Detect(context.Background(), root)
	if err != nil {
		t.Fatalf("Detect: %v", err)
	}
	want := mergeinfo.State{
		Kind:      "merge",
		OurHead:   runGit(t, root, "rev-parse", "main"),
		TheirHead: runGit(t, root, "rev-parse", "theirs"),
	}
	if state != want {
		t.Errorf("want %+v, got %+v", want, state)
	}
}

func TestRule1_DetectWorksFromASubdirectory(t *testing.T) {
	t.Parallel()
	root := newRepoWithBaseApp(t)
	writeFile(t, root, "sub/readme.txt", "sub\n")
	commitAll(t, root, "add sub")
	theirEdits, ourEdits := greetConflictEdits(t, root)
	mergeTheirsIntoMain(t, root, theirEdits, ourEdits)
	state, err := mergeinfo.Detect(context.Background(), filepath.Join(root, "sub"))
	if err != nil {
		t.Fatalf("Detect: %v", err)
	}
	if state.Kind != "merge" {
		t.Errorf("want Kind merge, got %+v", state)
	}
	if want := runGit(t, root, "rev-parse", "theirs"); state.TheirHead != want {
		t.Errorf("want TheirHead %s, got %s", want, state.TheirHead)
	}
}

func TestRule2_3_RebaseIsDetected(t *testing.T) {
	t.Parallel()
	root, featureCommit := newFeatureAndMainDiverged(t)
	runGit(t, root, "checkout", "-q", "feature")
	runGitExpectingConflict(t, root, "rebase", "main")
	state, err := mergeinfo.Detect(context.Background(), root)
	if err != nil {
		t.Fatalf("Detect: %v", err)
	}
	want := mergeinfo.State{
		Kind:      "rebase",
		OurHead:   runGit(t, root, "rev-parse", "HEAD"),
		TheirHead: featureCommit,
	}
	if want.OurHead != runGit(t, root, "rev-parse", "main") {
		t.Fatalf("setup: during the rebase HEAD should be main's tip")
	}
	if state != want {
		t.Errorf("want %+v, got %+v", want, state)
	}
}

func TestRule2_3_CherryPickIsDetected(t *testing.T) {
	t.Parallel()
	root, featureCommit := newFeatureAndMainDiverged(t)
	runGitExpectingConflict(t, root, "cherry-pick", "feature")
	state, err := mergeinfo.Detect(context.Background(), root)
	if err != nil {
		t.Fatalf("Detect: %v", err)
	}
	want := mergeinfo.State{
		Kind:      "cherry-pick",
		OurHead:   runGit(t, root, "rev-parse", "main"),
		TheirHead: featureCommit,
	}
	if state != want {
		t.Errorf("want %+v, got %+v", want, state)
	}
}
