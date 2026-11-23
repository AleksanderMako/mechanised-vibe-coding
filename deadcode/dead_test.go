package deadcode_test

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/AleksanderMako/machanised-vibe-coding/deadcode"
)

// deadWithoutTests is every function of the module fixture that main, and the init of the lib it imports, do not reach (rules 2, 5, 6).
func deadWithoutTests(t *testing.T, root string) []deadcode.Func {
	t.Helper()
	return sortLikeDead([]deadcode.Func{
		expectFunc(t, root, appPackage, mainFile, "unusedHelper"),
		expectFunc(t, root, libPackage, libFile, "Abandoned"),
		expectFunc(t, root, libPackage, libFile, "Later"),
		expectFunc(t, root, libPackage, libFile, "Widget.Unused"),
		expectFunc(t, root, libPackage, libFile, "NeverInstantiated"),
		expectFunc(t, root, libPackage, libFile, "AbandonedWithClosure"),
		expectFunc(t, root, libPackage, testOnlyFile, "OnlyTests"),
		expectFunc(t, root, libPackage, testOnlyFile, "OnlyBenchmark"),
		expectFunc(t, root, libPackage, testOnlyFile, "OnlyFuzz"),
		expectFunc(t, root, libPackage, testOnlyFile, "OnlyExample"),
	})
}

// deadWithTests drops the helpers that Test, Benchmark, Fuzz and Example functions call (rule 2).
func deadWithTests(t *testing.T, root string) []deadcode.Func {
	t.Helper()
	return sortLikeDead([]deadcode.Func{
		expectFunc(t, root, appPackage, mainFile, "unusedHelper"),
		expectFunc(t, root, libPackage, libFile, "Abandoned"),
		expectFunc(t, root, libPackage, libFile, "Later"),
		expectFunc(t, root, libPackage, libFile, "Widget.Unused"),
		expectFunc(t, root, libPackage, libFile, "NeverInstantiated"),
		expectFunc(t, root, libPackage, libFile, "AbandonedWithClosure"),
	})
}

func TestDeadListsExactlyTheUnreachableFunctionsSorted(t *testing.T) {
	root := copyFixture(t, "module")
	got, err := deadcode.Dead(t.Context(), root, deadcode.Options{})
	if err != nil {
		t.Fatalf("Dead: %v", err)
	}
	assertSortedLikeDead(t, "Dead", got)
	assertFuncs(t, "Dead without tests", got, deadWithoutTests(t, root))
}

func TestDeadWithTestsTreatsTestBenchmarkFuzzAndExampleAsRoots(t *testing.T) {
	root := copyFixture(t, "module")
	got, err := deadcode.Dead(t.Context(), root, deadcode.Options{Tests: true})
	if err != nil {
		t.Fatalf("Dead: %v", err)
	}
	assertSortedLikeDead(t, "Dead with tests", got)
	assertFuncs(t, "Dead with tests", got, deadWithTests(t, root))
}

func TestDeadReportsLiveFunctionsNowhere(t *testing.T) {
	root := copyFixture(t, "module")
	got, err := deadcode.Dead(t.Context(), root, deadcode.Options{})
	if err != nil {
		t.Fatalf("Dead: %v", err)
	}
	// Each of these is reached from main, from init, through a closure, or through a generic instantiation.
	live := []struct{ packagePath, name, why string }{
		{appPackage, "main", "main is a root"},
		{libPackage, "init", "main imports lib, so its init runs"},
		{libPackage, "calledFromInit", "only lib's init calls it, and main imports lib"},
		{libPackage, "Live", "main calls it"},
		{libPackage, "Soon", "main calls it"},
		{libPackage, "Widget.Used", "main calls the method"},
		{libPackage, "Map", "main uses the Map[int] instantiation"},
		{libPackage, "WithClosure", "main calls it"},
		{libPackage, "reachedOnlyThroughClosure", "a literal inside a live function calls it"},
	}
	for _, function := range live {
		if _, found := findByName(got, function.packagePath, function.name); found {
			t.Errorf("%s.%s is reported dead but is live: %s", function.packagePath, function.name, function.why)
		}
	}
}

func TestDeadNamesNoLiteralsInstantiationsOrPointerReceivers(t *testing.T) {
	root := copyFixture(t, "module")
	got, err := deadcode.Dead(t.Context(), root, deadcode.Options{})
	if err != nil {
		t.Fatalf("Dead: %v", err)
	}
	for _, function := range got {
		// Literals look like Outer$1, instantiations like Map[int], pointer methods like (*Widget).Unused.
		if strings.ContainsAny(function.Name, "$[]*()") || strings.Contains(function.Name, "func") {
			t.Errorf("reported name %q is not a plain Name or Type.Method", function.Name)
		}
	}
	if _, found := findByName(got, libPackage, "Widget.Unused"); !found {
		t.Errorf("the dead pointer method must be named %q, got:%s", "Widget.Unused", describeFuncs(got))
	}
}

func TestDeadFilesAreSlashSeparatedAndRelativeToRoot(t *testing.T) {
	root := copyFixture(t, "module")
	got, err := deadcode.Dead(t.Context(), root, deadcode.Options{Tests: true})
	if err != nil {
		t.Fatalf("Dead: %v", err)
	}
	if len(got) == 0 {
		t.Fatal("want dead functions in the fixture, got none")
	}
	for _, function := range got {
		if filepath.IsAbs(function.File) || strings.HasPrefix(function.File, "/") {
			t.Errorf("%s: File %q must be relative to Root", function.Name, function.File)
		}
		if strings.Contains(function.File, `\`) {
			t.Errorf("%s: File %q must use forward slashes", function.Name, function.File)
		}
		if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(function.File))); err != nil {
			t.Errorf("%s: File %q does not exist under Root: %v", function.Name, function.File, err)
		}
	}
}

func TestDeadPatternsLimitTheLoadedPackages(t *testing.T) {
	root := copyFixture(t, "module")
	got, err := deadcode.Dead(t.Context(), root, deadcode.Options{Patterns: []string{"./cmd/app"}})
	if err != nil {
		t.Fatalf("Dead: %v", err)
	}
	// lib is only a dependency of cmd/app here, so its dead functions are not reported.
	want := []deadcode.Func{expectFunc(t, root, appPackage, mainFile, "unusedHelper")}
	assertFuncs(t, "Dead with Patterns ./cmd/app", got, want)
}

func TestDeadWithoutGoModIsAnError(t *testing.T) {
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	writeFile(t, root, "main.go", "package main\n\nfunc main() {}\n")
	_, err = deadcode.Dead(t.Context(), root, deadcode.Options{})
	if err == nil {
		t.Fatal("want an error for a Root without go.mod, got nil")
	}
	if errors.Is(err, deadcode.ErrNoRoots) {
		t.Errorf("a missing go.mod is an ordinary error, not ErrNoRoots: %v", err)
	}
}

func TestDeadTypeErrorIsAnError(t *testing.T) {
	root := copyFixture(t, "module")
	writeFile(t, root, "lib/broken.go", "package lib\n\nfunc Broken() int {\n\treturn \"not a number\"\n}\n")
	_, err := deadcode.Dead(t.Context(), root, deadcode.Options{})
	if err == nil {
		t.Fatal("want an error for a package with a type error, got nil")
	}
	if errors.Is(err, deadcode.ErrNoRoots) {
		t.Errorf("a type error is an ordinary error, not ErrNoRoots: %v", err)
	}
}

func TestDeadSyntaxErrorIsAnError(t *testing.T) {
	root := copyFixture(t, "module")
	writeFile(t, root, "lib/broken.go", "package lib\n\nfunc Broken( {\n")
	_, err := deadcode.Dead(t.Context(), root, deadcode.Options{})
	if err == nil {
		t.Fatal("want an error for a package that does not parse, got nil")
	}
	if errors.Is(err, deadcode.ErrNoRoots) {
		t.Errorf("a load error is an ordinary error, not ErrNoRoots: %v", err)
	}
}

func TestDeadLibraryWithoutMainOrTestsHasNoRoots(t *testing.T) {
	root := copyFixture(t, "libonly")
	for _, options := range []deadcode.Options{{}, {Tests: true}} {
		got, err := deadcode.Dead(t.Context(), root, options)
		if !errors.Is(err, deadcode.ErrNoRoots) {
			t.Errorf("Tests=%v: want an error wrapping ErrNoRoots, got %v and%s", options.Tests, err, describeFuncs(got))
		}
	}
}

func TestDeadLibraryTestsAloneAreRoots(t *testing.T) {
	root := copyFixture(t, "libonly")
	writeFile(t, root, libonlyTestFile, libonlyTestContent)
	got, err := deadcode.Dead(t.Context(), root, deadcode.Options{Tests: true})
	if err != nil {
		t.Fatalf("Dead with Tests: %v", err)
	}
	want := []deadcode.Func{expectFunc(t, root, libonlyPackage, "libonly.go", "Unused")}
	assertFuncs(t, "Dead of a library with a test", got, want)

	// Without Tests the test file is not loaded, so there is still nothing to start from.
	_, err = deadcode.Dead(t.Context(), root, deadcode.Options{})
	if !errors.Is(err, deadcode.ErrNoRoots) {
		t.Errorf("without Tests: want an error wrapping ErrNoRoots, got %v", err)
	}
}

func TestKeyIgnoresFileAndLine(t *testing.T) {
	first := deadcode.Func{Package: "example.com/p", Name: "Run", File: "a.go", Line: 3}
	moved := deadcode.Func{Package: "example.com/p", Name: "Run", File: "sub/b.go", Line: 40}
	if first.Key() != moved.Key() {
		t.Errorf("Key must depend on Package and Name only: %q != %q", first.Key(), moved.Key())
	}
}

func TestKeyDiffersByNameAndPackage(t *testing.T) {
	base := deadcode.Func{Package: "example.com/p", Name: "Run", File: "a.go", Line: 3}
	otherName := deadcode.Func{Package: "example.com/p", Name: "Stop", File: "a.go", Line: 3}
	method := deadcode.Func{Package: "example.com/p", Name: "Job.Run", File: "a.go", Line: 3}
	otherPackage := deadcode.Func{Package: "example.com/q", Name: "Run", File: "a.go", Line: 3}
	for _, other := range []deadcode.Func{otherName, method, otherPackage} {
		if base.Key() == other.Key() {
			t.Errorf("Key of %+v and %+v must differ, both are %q", base, other, base.Key())
		}
	}
}
