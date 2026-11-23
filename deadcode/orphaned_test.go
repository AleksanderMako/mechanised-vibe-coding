package deadcode_test

import (
	"testing"

	"github.com/AleksanderMako/mechanised-vibe-coding/deadcode"
)

// Edits that move lib.Abandoned, lib.Soon and lib.Later down the file so File and Line show which revision they came from.
const (
	abandonedMarker      = "// marker:Abandoned\n"
	firstPadding         = "// padding added by the second commit\n// so the functions below move\n" + abandonedMarker
	uncommittedPadding   = "// padding added without committing\n" + abandonedMarker
	mainCallsSoon        = "total += lib.Soon()"
	mainCallsLater       = "total += lib.Later()"
	mainCallsAbandoned   = "total += lib.Abandoned()"
	brokenLibFile        = "lib/broken.go"
	brokenLibFileContent = "package lib\n\nfunc Broken() int {\n\treturn \"not a number\"\n}\n"
)

// orphanRepo is the module fixture as a repository with two commits and an uncommitted change on top.
type orphanRepo struct {
	root string

	// first commit: main calls Soon; Later and Abandoned are dead.
	firstCommit  string
	firstLibText string
	deadAtFirst  []deadcode.Func

	// second commit: main calls Later instead of Soon, and lib.go gains padding.
	secondCommit  string
	secondLibText string
	deadAtSecond  []deadcode.Func

	// working tree: main calls Abandoned instead of Later, and lib.go gains more padding.
	workingLibText string
	deadInWorking  []deadcode.Func
}

// newOrphanRepo builds the repository and records Dead at each state so Orphaned can be compared against it.
func newOrphanRepo(t *testing.T, options deadcode.Options) orphanRepo {
	t.Helper()
	repo := orphanRepo{root: copyFixture(t, "module")}
	initRepo(t, repo.root)

	repo.firstCommit = commitAll(t, repo.root, "first")
	repo.firstLibText = readFile(t, repo.root, libFile)
	repo.deadAtFirst = deadOrFail(t, repo.root, options, "first commit")

	replaceInFile(t, repo.root, mainFile, mainCallsSoon, mainCallsLater)
	replaceInFile(t, repo.root, libFile, abandonedMarker, firstPadding)
	repo.secondCommit = commitAll(t, repo.root, "second")
	repo.secondLibText = readFile(t, repo.root, libFile)
	repo.deadAtSecond = deadOrFail(t, repo.root, options, "second commit")

	replaceInFile(t, repo.root, mainFile, mainCallsLater, mainCallsAbandoned)
	replaceInFile(t, repo.root, libFile, abandonedMarker, uncommittedPadding)
	repo.workingLibText = readFile(t, repo.root, libFile)
	repo.deadInWorking = deadOrFail(t, repo.root, options, "working tree")
	return repo
}

// deadOrFail runs Dead on root and fails the test on an error.
func deadOrFail(t *testing.T, root string, options deadcode.Options, label string) []deadcode.Func {
	t.Helper()
	dead, err := deadcode.Dead(t.Context(), root, options)
	if err != nil {
		t.Fatalf("Dead at %s: %v", label, err)
	}
	return dead
}

func TestOrphanedBetweenTwoCommits(t *testing.T) {
	repo := newOrphanRepo(t, deadcode.Options{})
	diff, err := deadcode.Orphaned(t.Context(), repo.root, repo.firstCommit, repo.secondCommit, deadcode.Options{})
	if err != nil {
		t.Fatalf("Orphaned: %v", err)
	}
	assertOnlyMainWorktree(t, repo.root)

	assertFuncs(t, "Before", diff.Before, repo.deadAtFirst)
	assertFuncs(t, "After", diff.After, repo.deadAtSecond)

	// Soon became dead, so its position is the one in the second commit, where it is dead.
	wantNewlyDead := []deadcode.Func{expectFuncIn(t, repo.secondLibText, libPackage, libFile, "Soon")}
	// Later came back to life, so its position is the one in the first commit, where it was dead.
	wantRevived := []deadcode.Func{expectFuncIn(t, repo.firstLibText, libPackage, libFile, "Later")}
	assertFuncs(t, "NewlyDead", diff.NewlyDead, wantNewlyDead)
	assertFuncs(t, "Revived", diff.Revived, wantRevived)

	// The fixture really moves the functions, otherwise the position checks above prove nothing.
	if wantNewlyDead[0].Line == expectFuncIn(t, repo.firstLibText, libPackage, libFile, "Soon").Line {
		t.Fatal("fixture bug: Soon does not move between the commits")
	}
	if wantRevived[0].Line == expectFuncIn(t, repo.secondLibText, libPackage, libFile, "Later").Line {
		t.Fatal("fixture bug: Later does not move between the commits")
	}
}

func TestOrphanedEmptyAfterRefUsesTheWorkingTree(t *testing.T) {
	repo := newOrphanRepo(t, deadcode.Options{})
	diff, err := deadcode.Orphaned(t.Context(), repo.root, repo.secondCommit, "", deadcode.Options{})
	if err != nil {
		t.Fatalf("Orphaned: %v", err)
	}
	assertOnlyMainWorktree(t, repo.root)

	assertFuncs(t, "Before", diff.Before, repo.deadAtSecond)
	assertFuncs(t, "After", diff.After, repo.deadInWorking)

	// The uncommitted edit stops calling Later and starts calling Abandoned.
	wantNewlyDead := []deadcode.Func{expectFuncIn(t, repo.workingLibText, libPackage, libFile, "Later")}
	wantRevived := []deadcode.Func{expectFuncIn(t, repo.secondLibText, libPackage, libFile, "Abandoned")}
	assertFuncs(t, "NewlyDead", diff.NewlyDead, wantNewlyDead)
	assertFuncs(t, "Revived", diff.Revived, wantRevived)

	// The working tree is analysed in place, so it must still hold the uncommitted change.
	if readFile(t, repo.root, libFile) != repo.workingLibText {
		t.Error("Orphaned changed the working tree")
	}
}

func TestOrphanedFunctionsDeadInBothRevisionsAreInNeitherDelta(t *testing.T) {
	repo := newOrphanRepo(t, deadcode.Options{})
	diff, err := deadcode.Orphaned(t.Context(), repo.root, repo.firstCommit, repo.secondCommit, deadcode.Options{})
	if err != nil {
		t.Fatalf("Orphaned: %v", err)
	}
	// Abandoned moves lines between the commits but stays dead, so a Key based diff must ignore it.
	deadInBoth := []struct{ packagePath, name string }{
		{appPackage, "unusedHelper"},
		{libPackage, "Abandoned"},
		{libPackage, "Widget.Unused"},
		{libPackage, "NeverInstantiated"},
		{libPackage, "AbandonedWithClosure"},
		{libPackage, "OnlyTests"},
	}
	for _, function := range deadInBoth {
		if _, found := findByName(diff.Before, function.packagePath, function.name); !found {
			t.Errorf("%s.%s missing from Before", function.packagePath, function.name)
		}
		if _, found := findByName(diff.After, function.packagePath, function.name); !found {
			t.Errorf("%s.%s missing from After", function.packagePath, function.name)
		}
		if _, found := findByName(diff.NewlyDead, function.packagePath, function.name); found {
			t.Errorf("%s.%s is dead in both revisions but listed in NewlyDead", function.packagePath, function.name)
		}
		if _, found := findByName(diff.Revived, function.packagePath, function.name); found {
			t.Errorf("%s.%s is dead in both revisions but listed in Revived", function.packagePath, function.name)
		}
	}
}

func TestOrphanedSameRevisionHasEmptyDeltas(t *testing.T) {
	repo := newOrphanRepo(t, deadcode.Options{})
	diff, err := deadcode.Orphaned(t.Context(), repo.root, repo.firstCommit, repo.firstCommit, deadcode.Options{})
	if err != nil {
		t.Fatalf("Orphaned: %v", err)
	}
	assertOnlyMainWorktree(t, repo.root)
	assertFuncs(t, "Before", diff.Before, repo.deadAtFirst)
	assertFuncs(t, "After", diff.After, repo.deadAtFirst)
	assertFuncs(t, "NewlyDead", diff.NewlyDead, nil)
	assertFuncs(t, "Revived", diff.Revived, nil)
}

func TestOrphanedPassesOptionsToBothRevisions(t *testing.T) {
	options := deadcode.Options{Tests: true}
	repo := newOrphanRepo(t, options)
	diff, err := deadcode.Orphaned(t.Context(), repo.root, repo.firstCommit, repo.secondCommit, options)
	if err != nil {
		t.Fatalf("Orphaned: %v", err)
	}
	assertOnlyMainWorktree(t, repo.root)
	assertFuncs(t, "Before", diff.Before, repo.deadAtFirst)
	assertFuncs(t, "After", diff.After, repo.deadAtSecond)
	for _, list := range [][]deadcode.Func{diff.Before, diff.After} {
		if _, found := findByName(list, libPackage, "OnlyTests"); found {
			t.Errorf("with Tests, OnlyTests is live in both revisions but was reported dead")
		}
	}
}

func TestOrphanedDeltasAreSortedLikeDead(t *testing.T) {
	root := copyFixture(t, "module")
	initRepo(t, root)
	firstCommit := commitAll(t, root, "first")
	// Stop calling several functions at once so NewlyDead has several entries to sort.
	replaceInFile(t, root, mainFile, mainCallsSoon, "")
	replaceInFile(t, root, mainFile, "total += lib.WithClosure()", "")
	replaceInFile(t, root, mainFile, "total := lib.Live()", "total := 0")
	diff, err := deadcode.Orphaned(t.Context(), root, firstCommit, "", deadcode.Options{})
	if err != nil {
		t.Fatalf("Orphaned: %v", err)
	}
	assertOnlyMainWorktree(t, root)
	want := sortLikeDead([]deadcode.Func{
		expectFunc(t, root, libPackage, libFile, "Live"),
		expectFunc(t, root, libPackage, libFile, "Soon"),
		expectFunc(t, root, libPackage, libFile, "WithClosure"),
		expectFunc(t, root, libPackage, libFile, "reachedOnlyThroughClosure"),
	})
	assertFuncs(t, "NewlyDead", diff.NewlyDead, want)
	assertSortedLikeDead(t, "Before", diff.Before)
	assertSortedLikeDead(t, "After", diff.After)
}

func TestOrphanedInvalidRefIsAnError(t *testing.T) {
	root := copyFixture(t, "module")
	initRepo(t, root)
	commit := commitAll(t, root, "first")

	cases := []struct{ label, beforeRef, afterRef string }{
		{"invalid before ref", "no-such-branch", commit},
		{"invalid before ref with working tree", "no-such-branch", ""},
		{"invalid after ref", commit, "no-such-branch"},
	}
	for _, testCase := range cases {
		t.Run(testCase.label, func(t *testing.T) {
			_, err := deadcode.Orphaned(t.Context(), root, testCase.beforeRef, testCase.afterRef, deadcode.Options{})
			if err == nil {
				t.Fatal("want an error for an unknown ref, got nil")
			}
			assertOnlyMainWorktree(t, root)
		})
	}
}

func TestOrphanedRemovesWorktreesWhenARevisionDoesNotBuild(t *testing.T) {
	root := copyFixture(t, "module")
	initRepo(t, root)
	goodCommit := commitAll(t, root, "good")
	writeFile(t, root, brokenLibFile, brokenLibFileContent)
	brokenCommit := commitAll(t, root, "broken")

	cases := []struct{ label, beforeRef, afterRef string }{
		{"broken before ref", brokenCommit, goodCommit},
		{"broken after ref", goodCommit, brokenCommit},
		{"broken before ref with working tree", brokenCommit, ""},
	}
	for _, testCase := range cases {
		t.Run(testCase.label, func(t *testing.T) {
			// Strict keeps the broken revision a real error; since stage 9 it would otherwise be ErrNoRoots.
			_, err := deadcode.Orphaned(t.Context(), root, testCase.beforeRef, testCase.afterRef, deadcode.Options{Strict: true})
			if err == nil {
				t.Fatal("want an error when a revision has a type error, got nil")
			}
			assertOnlyMainWorktree(t, root)
		})
	}
}
