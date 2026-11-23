package verify_test

import (
	"context"
	"path/filepath"
	"slices"
	"testing"

	"github.com/AleksanderMako/mechanised-vibe-coding/verify"
)

// goSource returns a small formatted Go file for package name.
func goSource(name string) string {
	return "package " + name + "\n\n// Extra exists so the directory counts as changed.\nfunc Extra() int { return 0 }\n"
}

func TestRule15_RootOutsideAGitWorkTreeIsAnError(t *testing.T) {
	t.Parallel()
	root := copyFixture(t, "module")
	if _, err := verify.ChangedPackages(context.Background(), root); err == nil {
		t.Error("ChangedPackages outside a git work tree: want an error, got nil")
	}
}

func TestRule16_CleanTreeHasNoChangedPackages(t *testing.T) {
	t.Parallel()
	root := copyFixture(t, "module")
	commitAll(t, root)
	packages, err := verify.ChangedPackages(context.Background(), root)
	if err != nil {
		t.Fatalf("ChangedPackages: %v", err)
	}
	if len(packages) != 0 {
		t.Errorf("clean tree: want no packages, got %v", packages)
	}
}

func TestRule16_StagedUnstagedAndUntrackedFilesCount(t *testing.T) {
	t.Parallel()
	root := copyFixture(t, "module")
	commitAll(t, root)
	// Unstaged edit.
	writeFile(t, root, "lib/greet/greet.go", "// Package greet builds greetings.\npackage greet\n\n// Hello greets name.\nfunc Hello(name string) string { return \"Hi, \" + name }\n")
	// Staged edit.
	writeFile(t, root, "lib/calc/more.go", goSource("calc"))
	runGit(t, root, "add", "lib/calc/more.go")
	// Untracked file inside an untracked directory needs --untracked-files=all to be seen.
	writeFile(t, root, "lib/fresh/deep/fresh.go", goSource("deep"))
	packages, err := verify.ChangedPackages(context.Background(), root)
	if err != nil {
		t.Fatalf("ChangedPackages: %v", err)
	}
	want := []string{"./lib/calc", "./lib/fresh/deep", "./lib/greet"}
	if !slices.Equal(packages, want) {
		t.Errorf("want %v, got %v", want, packages)
	}
}

func TestRule16_DeletedFilesReportTheirDirectoryOnlyWhenItStillExists(t *testing.T) {
	t.Parallel()
	root := copyFixture(t, "module")
	commitAll(t, root)
	removePath(t, root, "lib/gone/gone.go")
	removePath(t, root, "lib/vanish")
	packages, err := verify.ChangedPackages(context.Background(), root)
	if err != nil {
		t.Fatalf("ChangedPackages: %v", err)
	}
	if want := []string{"./lib/gone"}; !slices.Equal(packages, want) {
		t.Errorf("want %v, got %v", want, packages)
	}
}

func TestRule16_StagedDeletionCounts(t *testing.T) {
	t.Parallel()
	root := copyFixture(t, "module")
	commitAll(t, root)
	runGit(t, root, "rm", "-q", "lib/gone/gone.go")
	packages, err := verify.ChangedPackages(context.Background(), root)
	if err != nil {
		t.Fatalf("ChangedPackages: %v", err)
	}
	if want := []string{"./lib/gone"}; !slices.Equal(packages, want) {
		t.Errorf("want %v, got %v", want, packages)
	}
}

func TestRule17_OnlyGoFilesCount(t *testing.T) {
	t.Parallel()
	root := copyFixture(t, "module")
	commitAll(t, root)
	writeFile(t, root, "lib/docs/readme.txt", "Changed notes.\n")
	writeFile(t, root, "lib/docs/new.md", "# New\n")
	writeFile(t, root, "go.mod", "module example.com/fixture\n\ngo 1.26\n\n// touched\n")
	packages, err := verify.ChangedPackages(context.Background(), root)
	if err != nil {
		t.Fatalf("ChangedPackages: %v", err)
	}
	if len(packages) != 0 {
		t.Errorf("non-Go changes: want no packages, got %v", packages)
	}
}

func TestRule17_RootFilesAreReportedAsDot(t *testing.T) {
	t.Parallel()
	root := copyFixture(t, "module")
	commitAll(t, root)
	writeFile(t, root, "extra.go", goSource("fixture"))
	writeFile(t, root, "lib/calc/more.go", goSource("calc"))
	packages, err := verify.ChangedPackages(context.Background(), root)
	if err != nil {
		t.Fatalf("ChangedPackages: %v", err)
	}
	if want := []string{".", "./lib/calc"}; !slices.Equal(packages, want) {
		t.Errorf("want %v, got %v", want, packages)
	}
}

func TestRule17_DirectoriesAreRelativeToRootWhenRootIsBelowTheRepository(t *testing.T) {
	t.Parallel()
	repository, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	root := copyFixtureInto(t, "module", repository)
	commitAll(t, repository)
	writeFile(t, root, "lib/greet/more.go", goSource("greet"))
	writeFile(t, root, "extra.go", goSource("fixture"))
	packages, err := verify.ChangedPackages(context.Background(), root)
	if err != nil {
		t.Fatalf("ChangedPackages: %v", err)
	}
	if want := []string{".", "./lib/greet"}; !slices.Equal(packages, want) {
		t.Errorf("want %v, got %v", want, packages)
	}
}

func TestRule18_TestdataAndVendorAreExcludedAndResultsAreSortedAndUnique(t *testing.T) {
	t.Parallel()
	root := copyFixture(t, "module")
	commitAll(t, root)
	writeFile(t, root, "testdata/case.go", goSource("testdata"))
	writeFile(t, root, "lib/calc/testdata/nested/case.go", goSource("nested"))
	writeFile(t, root, "vendor/example.com/dep/dep.go", goSource("dep"))
	writeFile(t, root, "lib/greet/vendor/dep.go", goSource("dep"))
	// Several changes in one directory must yield one entry.
	writeFile(t, root, "lib/greet/one.go", goSource("greet"))
	writeFile(t, root, "lib/greet/two.go", goSource("greet"))
	writeFile(t, root, "lib/calc/more.go", goSource("calc"))
	packages, err := verify.ChangedPackages(context.Background(), root)
	if err != nil {
		t.Fatalf("ChangedPackages: %v", err)
	}
	if want := []string{"./lib/calc", "./lib/greet"}; !slices.Equal(packages, want) {
		t.Errorf("want %v, got %v", want, packages)
	}
}
