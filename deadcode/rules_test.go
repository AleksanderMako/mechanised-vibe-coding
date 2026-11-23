package deadcode_test

import (
	"strings"
	"testing"

	"github.com/AleksanderMako/machanised-vibe-coding/deadcode"
)

// Import paths and files of the initroots fixture: main imports registry and, for its init only, plugin; nothing imports orphan.
const (
	registryPackage = "example.com/fixture/registry"
	pluginPackage   = "example.com/fixture/plugin"
	orphanPackage   = "example.com/fixture/orphan"
	registryFile    = "registry/registry.go"
	pluginFile      = "plugin/plugin.go"
	orphanFile      = "orphan/orphan.go"
)

// Import path and file of the markers fixture, where main imports shapes but calls no marker method.
const (
	shapesPackage = "example.com/fixture/shapes"
	shapesFile    = "shapes/shapes.go"
)

func TestDeadPackageThatNoMainImportsIsDeadEvenWhatItsInitRuns(t *testing.T) {
	root := copyFixture(t, "initroots")
	got, err := deadcode.Dead(t.Context(), root, deadcode.Options{})
	if err != nil {
		t.Fatalf("Dead: %v", err)
	}
	// Under an "every init is a root" rule, setUp, describe and ghost.Name would look live (rule 2).
	want := sortLikeDead([]deadcode.Func{
		expectFunc(t, root, appPackage, mainFile, "_"),
		expectFunc(t, root, orphanPackage, orphanFile, "ghost.Name"),
		expectFunc(t, root, orphanPackage, orphanFile, "init#1"),
		expectFunc(t, root, orphanPackage, orphanFile, "init#2"),
		expectFunc(t, root, orphanPackage, orphanFile, "setUp"),
		expectFunc(t, root, orphanPackage, orphanFile, "describe"),
		expectFunc(t, root, orphanPackage, orphanFile, "Exported"),
		expectFunc(t, root, pluginPackage, pluginFile, "NotRegistered"),
		expectFunc(t, root, registryPackage, registryFile, "_"),
		expectFunc(t, root, registryPackage, registryFile, "Forget"),
	})
	assertSortedLikeDead(t, "Dead", got)
	assertFuncs(t, "Dead of initroots", got, want)
}

func TestDeadListsTheInitsOfAPackageNoMainImportsBySSAName(t *testing.T) {
	root := copyFixture(t, "initroots")
	got, err := deadcode.Dead(t.Context(), root, deadcode.Options{})
	if err != nil {
		t.Fatalf("Dead: %v", err)
	}
	// orphan has two init declarations, numbered from 1 in source order (rule 2).
	for _, name := range []string{"init#1", "init#2"} {
		want := expectFunc(t, root, orphanPackage, orphanFile, name)
		function, found := findByName(got, orphanPackage, name)
		if !found {
			t.Errorf("orphan's %s is reached by nothing and must be listed, got:%s", name, describeFuncs(got))
			continue
		}
		if function != want {
			t.Errorf("%s: got %+v, want %+v", name, function, want)
		}
	}
	// plugin is imported by main, so its init runs and is not listed under any spelling.
	for _, function := range got {
		if function.Package == pluginPackage && (function.Name == "init" || strings.HasPrefix(function.Name, "init#")) {
			t.Errorf("plugin's init is reached through main's import but is listed as %s at %s:%d", function.Name, function.File, function.Line)
		}
	}
}

func TestDeadAlwaysListsBlankFunctions(t *testing.T) {
	root := copyFixture(t, "initroots")
	got, err := deadcode.Dead(t.Context(), root, deadcode.Options{})
	if err != nil {
		t.Fatalf("Dead: %v", err)
	}
	// A func _ can never be called, so it is listed in the main package and in a package main uses (rule 2).
	blanks := []struct{ packagePath, file string }{
		{appPackage, mainFile},
		{registryPackage, registryFile},
	}
	for _, blank := range blanks {
		want := expectFunc(t, root, blank.packagePath, blank.file, "_")
		function, found := findByName(got, blank.packagePath, "_")
		if !found {
			t.Errorf("func _ in %s must always be listed, got:%s", blank.file, describeFuncs(got))
			continue
		}
		if function != want {
			t.Errorf("func _ in %s: got %+v, want %+v", blank.file, function, want)
		}
	}
}

func TestDeadInitOfAnImportedPackageMakesItsCalleesLive(t *testing.T) {
	root := copyFixture(t, "initroots")
	got, err := deadcode.Dead(t.Context(), root, deadcode.Options{})
	if err != nil {
		t.Fatalf("Dead: %v", err)
	}
	// main imports plugin only for its side effects, so plugin's init runs and reaches these (rule 2).
	live := []struct{ packagePath, name, why string }{
		{pluginPackage, "prefix", "plugin's init registers greeter, whose Name calls prefix"},
		{pluginPackage, "greeter.Name", "plugin's init stores greeter as a Namer and main calls Describe, which calls Name"},
		{registryPackage, "Register", "plugin's init calls it"},
		{registryPackage, "Describe", "main calls it"},
	}
	for _, function := range live {
		if _, found := findByName(got, function.packagePath, function.name); found {
			t.Errorf("%s.%s is reported dead but is live: %s", function.packagePath, function.name, function.why)
		}
	}
	// The import keeps alive only what init reaches, not the whole package.
	if _, found := findByName(got, pluginPackage, "NotRegistered"); !found {
		t.Errorf("plugin.NotRegistered is reached by nothing and must be listed, got:%s", describeFuncs(got))
	}
}

// deadInMarkers is the dead list of the markers fixture, the same with and without Tests (rule 5).
func deadInMarkers(t *testing.T, root string) []deadcode.Func {
	t.Helper()
	// Circle.isShape and Square.isShape are unreachable but are markers, so they are missing here.
	return sortLikeDead([]deadcode.Func{
		expectFunc(t, root, shapesPackage, shapesFile, "Busy.isBusy"),
		expectFunc(t, root, shapesPackage, shapesFile, "Speaker.IsLoud"),
		expectFunc(t, root, shapesPackage, shapesFile, "Box.resize"),
		expectFunc(t, root, shapesPackage, shapesFile, "Foreign.isForeign"),
		expectFunc(t, root, shapesPackage, shapesFile, "Plain.isPlain"),
		expectFunc(t, root, shapesPackage, shapesFile, "Unused"),
		expectFunc(t, root, shapesPackage, shapesFile, "Tested.isTested"),
	})
}

func TestDeadNeverListsMarkerMethods(t *testing.T) {
	root := copyFixture(t, "markers")
	got, err := deadcode.Dead(t.Context(), root, deadcode.Options{})
	if err != nil {
		t.Fatalf("Dead: %v", err)
	}
	assertSortedLikeDead(t, "Dead", got)
	assertFuncs(t, "Dead of markers", got, deadInMarkers(t, root))
	for _, marker := range []string{"Circle.isShape", "Square.isShape"} {
		if _, found := findByName(got, shapesPackage, marker); found {
			t.Errorf("%s is a marker method and must never be listed", marker)
		}
	}
}

func TestDeadListsMarkerLookAlikesThatBreakOneCondition(t *testing.T) {
	root := copyFixture(t, "markers")
	got, err := deadcode.Dead(t.Context(), root, deadcode.Options{})
	if err != nil {
		t.Fatalf("Dead: %v", err)
	}
	lookAlikes := []struct{ name, brokenCondition string }{
		{"Busy.isBusy", "the body has a statement"},
		{"Speaker.IsLoud", "the method is exported"},
		{"Box.resize", "the method has a parameter"},
		{"Foreign.isForeign", "the only interface naming it is declared in another package"},
		{"Plain.isPlain", "no interface asks for it at all"},
	}
	for _, lookAlike := range lookAlikes {
		want := expectFunc(t, root, shapesPackage, shapesFile, lookAlike.name)
		function, found := findByName(got, shapesPackage, lookAlike.name)
		if !found {
			t.Errorf("%s is unreachable and not a marker because %s, so it must be listed", lookAlike.name, lookAlike.brokenCondition)
			continue
		}
		if function != want {
			t.Errorf("%s: got %+v, want %+v", lookAlike.name, function, want)
		}
	}
}

func TestDeadWithTestsChecksMarkersOnThePlainPackageCopy(t *testing.T) {
	root := copyFixture(t, "markers")
	got, err := deadcode.Dead(t.Context(), root, deadcode.Options{Tests: true})
	if err != nil {
		t.Fatalf("Dead: %v", err)
	}
	// Only the test variant declares the interface Tested.isTested satisfies; packages.Visit reaches the plain copy first (rule 5).
	assertSortedLikeDead(t, "Dead", got)
	assertFuncs(t, "Dead of markers with tests", got, deadInMarkers(t, root))
}
