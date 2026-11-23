package mergeinfo_test

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/AleksanderMako/mechanised-vibe-coding/mergeinfo"
)

// conflictsOf runs Conflicts on root and fails the test on error.
func conflictsOf(t *testing.T, root string) []mergeinfo.Hunk {
	t.Helper()
	hunks, err := mergeinfo.Conflicts(context.Background(), root)
	if err != nil {
		t.Fatalf("Conflicts: %v", err)
	}
	return hunks
}

// assertDecls checks the declaration each side of a hunk reports.
func assertDecls(t *testing.T, hunk mergeinfo.Hunk, wantOurs, wantTheirs string) {
	t.Helper()
	if hunk.OursDecl != wantOurs {
		t.Errorf("OursDecl: want %q, got %q", wantOurs, hunk.OursDecl)
	}
	if hunk.TheirsDecl != wantTheirs {
		t.Errorf("TheirsDecl: want %q, got %q", wantTheirs, hunk.TheirsDecl)
	}
}

// assertHunkMarkers checks the marker fields of a hunk reported by Conflicts.
func assertHunkMarkers(t *testing.T, hunk mergeinfo.Hunk, want markerFields) {
	t.Helper()
	if got := markersOf(hunk); got != want {
		t.Errorf("hunk markers differ\nwant %+v\ngot  %+v", want, got)
	}
}

// buildSameBodyConflict is scenario (a): both sides rewrite the same line of Greet.
func buildSameBodyConflict(t *testing.T) (root, theirCommit string) {
	root = newRepoWithBaseApp(t)
	mergeTheirsIntoMain(t, root, func() {
		replaceInFile(t, root, "app.go", `"hello "`, `"hey "`)
		theirCommit = commitAll(t, root, "their greet (#42)")
	}, func() {
		replaceInFile(t, root, "app.go", `"hello "`, `"hi "`)
		commitAll(t, root, "our greet")
	})
	return root, theirCommit
}

func TestRule11_12_13_14_BothSidesEditTheSameFunctionBody(t *testing.T) {
	t.Parallel()
	root, theirCommit := buildSameBodyConflict(t)
	hunk := onlyHunk(t, conflictsOf(t, root))
	assertHunkMarkers(t, hunk, markerFields{
		File:   filepath.Join(root, "app.go"),
		Start:  5,
		End:    9,
		Ours:   "\tprefix := \"hi \"\n",
		Theirs: "\tprefix := \"hey \"\n",
	})
	assertDecls(t, hunk, "func Greet", "func Greet")
	assertCommits(t, hunk.TheirCommits, []wantCommit{{Hash: theirCommit, Subject: "their greet (#42)", PR: 42}})
}

// buildOurDeletionConflict is scenario (b): we delete Greet while they edit its body.
func buildOurDeletionConflict(t *testing.T) (root, theirCommit string) {
	root = newRepoWithBaseApp(t)
	mergeTheirsIntoMain(t, root, func() {
		replaceInFile(t, root, "app.go", `"hello "`, `"hey "`)
		theirCommit = commitAll(t, root, "their greet (#42)")
	}, func() {
		replaceInFile(t, root, "app.go", greetFunc+"\n", "")
		commitAll(t, root, "drop greet")
	})
	return root, theirCommit
}

func TestRule6_12_13_OurSideDeletesAFunctionTheyEdited(t *testing.T) {
	t.Parallel()
	root, theirCommit := buildOurDeletionConflict(t)
	hunk := onlyHunk(t, conflictsOf(t, root))
	assertHunkMarkers(t, hunk, markerFields{
		File:        filepath.Join(root, "app.go"),
		Start:       3,
		End:         11,
		Ours:        "",
		Theirs:      "// Greet says hello.\nfunc Greet(name string) string {\n\tprefix := \"hey \"\n\treturn prefix + name\n}\n\n",
		OursDeleted: true,
	})
	// Our resolved file has "// Counter counts." right where the hunk was, so the deletion sits on type Counter.
	assertDecls(t, hunk, "type Counter", "func Greet")
	assertCommits(t, hunk.TheirCommits, []wantCommit{{Hash: theirCommit, Subject: "their greet (#42)", PR: 42}})
}

// buildTheirDeletionConflict is scenario (c): they delete Greet while we edit its body.
func buildTheirDeletionConflict(t *testing.T) (root, theirCommit string) {
	root = newRepoWithBaseApp(t)
	mergeTheirsIntoMain(t, root, func() {
		replaceInFile(t, root, "app.go", greetFunc+"\n", "")
		theirCommit = commitAll(t, root, "drop greet (#9)")
	}, func() {
		replaceInFile(t, root, "app.go", `"hello "`, `"hi "`)
		commitAll(t, root, "our greet")
	})
	return root, theirCommit
}

func TestRule6_12_13_TheirSideDeletesAFunctionWeEdited(t *testing.T) {
	t.Parallel()
	root, theirCommit := buildTheirDeletionConflict(t)
	hunk := onlyHunk(t, conflictsOf(t, root))
	assertHunkMarkers(t, hunk, markerFields{
		File:          filepath.Join(root, "app.go"),
		Start:         3,
		End:           11,
		Ours:          "// Greet says hello.\nfunc Greet(name string) string {\n\tprefix := \"hi \"\n\treturn prefix + name\n}\n\n",
		Theirs:        "",
		TheirsDeleted: true,
	})
	assertDecls(t, hunk, "func Greet", "type Counter")
	// Their side is deleted, so the commit is kept because it removed lines of our side.
	assertCommits(t, hunk.TheirCommits, []wantCommit{{Hash: theirCommit, Subject: "drop greet (#9)", PR: 9}})
}

// buildGoAndTextConflict is scenario (d): a Go conflict plus a conflict in a text file in a subdirectory.
func buildGoAndTextConflict(t *testing.T) (root, theirCommit string) {
	root = newRepoWithBaseApp(t)
	writeFile(t, root, "docs/notes.txt", "alpha\nbeta\ngamma\n")
	commitAll(t, root, "add notes")
	mergeTheirsIntoMain(t, root, func() {
		replaceInFile(t, root, "app.go", `"hello "`, `"hey "`)
		replaceInFile(t, root, "docs/notes.txt", "beta", "their beta")
		theirCommit = commitAll(t, root, "their notes (#77)")
	}, func() {
		replaceInFile(t, root, "app.go", `"hello "`, `"hi "`)
		replaceInFile(t, root, "docs/notes.txt", "beta", "our beta")
		commitAll(t, root, "our notes")
	})
	return root, theirCommit
}

func TestRule10_11_12_NonGoFileHasNoDeclarations(t *testing.T) {
	t.Parallel()
	root, theirCommit := buildGoAndTextConflict(t)
	hunks := conflictsOf(t, root)
	if len(hunks) != 2 {
		t.Fatalf("want 2 hunks (app.go and docs/notes.txt), got %d: %+v", len(hunks), hunks)
	}
	notesHunk := onlyHunk(t, hunksInFile(hunks, filepath.Join(root, "docs", "notes.txt")))
	assertHunkMarkers(t, notesHunk, markerFields{
		File:   filepath.Join(root, "docs", "notes.txt"),
		Start:  2,
		End:    6,
		Ours:   "our beta\n",
		Theirs: "their beta\n",
	})
	assertDecls(t, notesHunk, "", "")
	assertCommits(t, notesHunk.TheirCommits, []wantCommit{{Hash: theirCommit, Subject: "their notes (#77)", PR: 77}})

	goHunk := onlyHunk(t, hunksInFile(hunks, filepath.Join(root, "app.go")))
	assertDecls(t, goHunk, "func Greet", "func Greet")
}

func TestRule4_6_11_Diff3MarkersFillBase(t *testing.T) {
	t.Parallel()
	root := newRepoWithBaseApp(t)
	var theirCommit string
	mergeTheirsIntoMain(t, root, func() {
		replaceInFile(t, root, "app.go", `"hello "`, `"hey "`)
		theirCommit = commitAll(t, root, "their greet (#42)")
	}, func() {
		replaceInFile(t, root, "app.go", `"hello "`, `"hi "`)
		commitAll(t, root, "our greet")
	}, "-c", "merge.conflictStyle=diff3")
	hunk := onlyHunk(t, conflictsOf(t, root))
	assertHunkMarkers(t, hunk, markerFields{
		File:   filepath.Join(root, "app.go"),
		Start:  5,
		End:    11,
		Ours:   "\tprefix := \"hi \"\n",
		Base:   "\tprefix := \"hello \"\n",
		Theirs: "\tprefix := \"hey \"\n",
	})
	assertDecls(t, hunk, "func Greet", "func Greet")
	assertCommits(t, hunk.TheirCommits, []wantCommit{{Hash: theirCommit, Subject: "their greet (#42)", PR: 42}})
}

func TestRule13_14_OnlyTheirCommitsThatTouchTheHunkNewestFirst(t *testing.T) {
	t.Parallel()
	root := newRepoWithBaseApp(t)
	var prefixCommit, askCommit string
	mergeTheirsIntoMain(t, root, func() {
		replaceInFile(t, root, "app.go", `"hello "`, `"hey "`)
		prefixCommit = commitAll(t, root, "set prefix")
		// This commit changes the file but nothing inside the hunk, so it must be filtered out.
		replaceInFile(t, root, "app.go", "return c.n", "return c.n + 0")
		commitAll(t, root, "tweak count (#7)")
		replaceInFile(t, root, "app.go", "return prefix + name", `return prefix + name + "?"`)
		askCommit = commitAll(t, root, "revert (#3) and ask (#42)")
	}, func() {
		replaceInFile(t, root, "app.go", `"hello "`, `"hi "`)
		replaceInFile(t, root, "app.go", "return prefix + name", `return prefix + name + "!"`)
		commitAll(t, root, "our greet")
	})
	hunk := onlyHunk(t, conflictsOf(t, root))
	assertHunkMarkers(t, hunk, markerFields{
		File:   filepath.Join(root, "app.go"),
		Start:  5,
		End:    11,
		Ours:   "\tprefix := \"hi \"\n\treturn prefix + name + \"!\"\n",
		Theirs: "\tprefix := \"hey \"\n\treturn prefix + name + \"?\"\n",
	})
	// The PR is the last (#N) of the subject, and the untouched commit is gone.
	assertCommits(t, hunk.TheirCommits, []wantCommit{
		{Hash: askCommit, Subject: "revert (#3) and ask (#42)", PR: 42},
		{Hash: prefixCommit, Subject: "set prefix", PR: 0},
	})
}

func TestRule13_NoMatchingCommitFallsBackToEveryCommitOfTheFile(t *testing.T) {
	t.Parallel()
	root := newRepoWithBaseApp(t)
	var dropCommit, countCommit string
	mergeTheirsIntoMain(t, root, func() {
		// Their side keeps only an unchanged base line inside the hunk, so no commit added any line of it.
		replaceInFile(t, root, "app.go", "\tprefix := \"hello \"\n", "")
		dropCommit = commitAll(t, root, "drop prefix line")
		replaceInFile(t, root, "app.go", "return c.n", "return c.n + 0")
		countCommit = commitAll(t, root, "count (#5)")
	}, func() {
		replaceInFile(t, root, "app.go", "return prefix + name", `return prefix + name + "!"`)
		commitAll(t, root, "our greet")
	})
	hunk := onlyHunk(t, conflictsOf(t, root))
	assertHunkMarkers(t, hunk, markerFields{
		File:   filepath.Join(root, "app.go"),
		Start:  5,
		End:    10,
		Ours:   "\tprefix := \"hello \"\n\treturn prefix + name + \"!\"\n",
		Theirs: "\treturn prefix + name\n",
	})
	assertCommits(t, hunk.TheirCommits, []wantCommit{
		{Hash: countCommit, Subject: "count (#5)", PR: 5},
		{Hash: dropCommit, Subject: "drop prefix line", PR: 0},
	})
}

func TestRule12_TwoHunksResolveEachSideBeforeLookingUpDeclarations(t *testing.T) {
	t.Parallel()
	root := newRepoWithBaseApp(t)
	var theirCommit string
	mergeTheirsIntoMain(t, root, func() {
		replaceInFile(t, root, "app.go", `"hello "`, `"hey "`)
		replaceInFile(t, root, "app.go", "return c.n", "return c.n + 2")
		theirCommit = commitAll(t, root, "their edits (#8)")
	}, func() {
		// Our first hunk is three lines longer than theirs, so line numbers differ per side after it.
		replaceInFile(t, root, "app.go", "\tprefix := \"hello \"\n", "\tgreeting := \"hi \"\n\tgreeting += \"\"\n\tprefix := greeting\n")
		replaceInFile(t, root, "app.go", "return c.n", "return c.n + 1")
		commitAll(t, root, "our edits")
	})
	hunks := conflictsOf(t, root)
	if len(hunks) != 2 {
		t.Fatalf("want 2 hunks, got %d: %+v", len(hunks), hunks)
	}
	greetHunk, countHunk := hunks[0], hunks[1]
	if greetHunk.Start > countHunk.Start {
		greetHunk, countHunk = countHunk, greetHunk
	}
	assertHunkMarkers(t, greetHunk, markerFields{
		File:   filepath.Join(root, "app.go"),
		Start:  5,
		End:    11,
		Ours:   "\tgreeting := \"hi \"\n\tgreeting += \"\"\n\tprefix := greeting\n",
		Theirs: "\tprefix := \"hey \"\n",
	})
	assertDecls(t, greetHunk, "func Greet", "func Greet")
	assertHunkMarkers(t, countHunk, markerFields{
		File:   filepath.Join(root, "app.go"),
		Start:  22,
		End:    26,
		Ours:   "\treturn c.n + 1\n",
		Theirs: "\treturn c.n + 2\n",
	})
	assertDecls(t, countHunk, "func (c *Counter) Count", "func (c *Counter) Count")
	want := []wantCommit{{Hash: theirCommit, Subject: "their edits (#8)", PR: 8}}
	assertCommits(t, greetHunk.TheirCommits, want)
	assertCommits(t, countHunk.TheirCommits, want)
}

func TestRule12_UnparsableResolvedSideGivesEmptyDeclaration(t *testing.T) {
	t.Parallel()
	root := newRepoWithBaseApp(t)
	mergeTheirsIntoMain(t, root, func() {
		replaceInFile(t, root, "app.go", `"hello "`, `(("hey "`)
		commitAll(t, root, "their broken greet")
	}, func() {
		replaceInFile(t, root, "app.go", `"hello "`, `"hi "`)
		commitAll(t, root, "our greet")
	})
	hunk := onlyHunk(t, conflictsOf(t, root))
	assertDecls(t, hunk, "func Greet", "")
}

func TestRule13_RebaseConflictListsTheCommitBeingApplied(t *testing.T) {
	t.Parallel()
	root, featureCommit := newFeatureAndMainDiverged(t)
	runGit(t, root, "checkout", "-q", "feature")
	runGitExpectingConflict(t, root, "rebase", "main")
	hunk := onlyHunk(t, conflictsOf(t, root))
	assertHunkMarkers(t, hunk, markerFields{
		File:   filepath.Join(root, "app.go"),
		Start:  5,
		End:    9,
		Ours:   "\tprefix := \"hi \"\n",
		Theirs: "\tprefix := \"hey \"\n",
	})
	assertCommits(t, hunk.TheirCommits, []wantCommit{{Hash: featureCommit, Subject: "feature greet (#42)", PR: 42}})
}

func TestRule10_CleanRepositoryReturnsErrNoConflict(t *testing.T) {
	t.Parallel()
	root := newRepoWithBaseApp(t)
	_, err := mergeinfo.Conflicts(context.Background(), root)
	if !errors.Is(err, mergeinfo.ErrNoConflict) {
		t.Errorf("clean repository: want ErrNoConflict, got %v", err)
	}
}

func TestRule10_OutsideAGitWorkTreeReturnsAnError(t *testing.T) {
	t.Parallel()
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := mergeinfo.Conflicts(context.Background(), root); err == nil {
		t.Error("Conflicts outside a git work tree: want an error, got nil")
	}
}

func TestRule10_MergeWithoutConflictedFilesReturnsNoHunks(t *testing.T) {
	t.Parallel()
	root := newRepoWithBaseApp(t)
	runGit(t, root, "checkout", "-q", "-b", "theirs")
	replaceInFile(t, root, "app.go", "return c.n", "return c.n + 0")
	commitAll(t, root, "their count")
	runGit(t, root, "checkout", "-q", "main")
	replaceInFile(t, root, "app.go", `"hello "`, `"hi "`)
	commitAll(t, root, "our greet")
	// The edits are far apart, so the merge stops only because of --no-commit and MERGE_HEAD exists.
	runGit(t, root, "merge", "-q", "--no-commit", "--no-ff", "theirs")
	hunks, err := mergeinfo.Conflicts(context.Background(), root)
	if err != nil {
		t.Fatalf("Conflicts: %v", err)
	}
	if len(hunks) != 0 {
		t.Errorf("want no hunks, got %+v", hunks)
	}
}
