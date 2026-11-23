package refactor_test

import (
	"context"
	"testing"

	"github.com/AleksanderMako/mechanised-vibe-coding/refactor"
)

func TestRule6_RenameFunctionRewritesDeclarationAndEveryUse(t *testing.T) {
	root := copyFixture(t, "rename")
	before := readTree(t, root)

	written, err := refactor.Rename(context.Background(), root, sym("shapes", "Area"), "Surface", refactor.Options{})
	if err != nil {
		t.Fatalf("Rename: %v", err)
	}

	changed := []string{"app/app.go", "shapes/external_test.go", "shapes/shapes.go", "shapes/shapes_test.go"}
	assertWritten(t, root, before, written, changed, nil)
	assertModuleBuilds(t, root)
	for _, file := range changed {
		if count := identifierCount(t, root, file, "Area"); count != 0 {
			t.Errorf("%s still has %d identifier(s) named Area", file, count)
		}
		if count := identifierCount(t, root, file, "Surface"); count == 0 {
			t.Errorf("%s has no identifier named Surface", file)
		}
	}
	assertContains(t, root, "shapes/shapes.go",
		"func Surface(width, height int) int",
		"Surface(r.Width, r.Height)",
	)
	assertContains(t, root, "shapes/shapes_test.go", "if Surface(2, 3) != 6")
	assertContains(t, root, "shapes/external_test.go", "shapes.Surface(1, 1)")
	assertContains(t, root, "app/app.go", "shapes.Surface(bigger.Width, bigger.Height)")
}

func TestRule4_RenameRewritesOnlyTheFirstWordOfTheDocComment(t *testing.T) {
	root := copyFixture(t, "rename")

	if _, err := refactor.Rename(context.Background(), root, sym("shapes", "Area"), "Surface", refactor.Options{}); err != nil {
		t.Fatalf("Rename: %v", err)
	}

	assertModuleBuilds(t, root)
	assertContains(t, root, "shapes/shapes.go",
		"// Surface returns the area of a rectangle.\n// Area is mentioned again on this second line.\nfunc Surface(",
	)
}

func TestRule9_RenameLeavesCommentsAndStringsAlone(t *testing.T) {
	root := copyFixture(t, "rename")

	if _, err := refactor.Rename(context.Background(), root, sym("shapes", "Area"), "Surface", refactor.Options{}); err != nil {
		t.Fatalf("Rename: %v", err)
	}

	assertModuleBuilds(t, root)
	assertContains(t, root, "shapes/shapes.go",
		"// Describe mentions Area in a comment and in a string.",
		"// Area is computed below.",
		`"Area of rect: %d"`,
	)
}

func TestRule6_RenameWithNoBuildStillRewritesEverything(t *testing.T) {
	root := copyFixture(t, "rename")
	before := readTree(t, root)

	written, err := refactor.Rename(context.Background(), root, sym("shapes", "Area"), "Surface", refactor.Options{NoBuild: true})
	if err != nil {
		t.Fatalf("Rename: %v", err)
	}

	assertWritten(t, root, before, written, []string{"app/app.go", "shapes/external_test.go", "shapes/shapes.go", "shapes/shapes_test.go"}, nil)
	assertModuleBuilds(t, root)
	assertContains(t, root, "app/app.go", "shapes.Surface(")
}

func TestRule6_RenameStructFieldInKeysAndSelectorsOnly(t *testing.T) {
	root := copyFixture(t, "rename")
	before := readTree(t, root)

	written, err := refactor.Rename(context.Background(), root, sym("shapes", "Rect.Width"), "Wide", refactor.Options{})
	if err != nil {
		t.Fatalf("Rename: %v", err)
	}

	assertWritten(t, root, before, written, []string{"app/app.go", "shapes/shapes.go", "shapes/shapes_test.go"}, nil)
	assertModuleBuilds(t, root)
	assertContains(t, root, "shapes/shapes.go",
		"type Rect struct { Wide int Height int }",
		"Rect{Wide: r.Wide * factor, Height: r.Height * factor}",
		"2 * (r.Wide + r.Height)",
		"Area(r.Wide, r.Height)",
		// Box.Width is a different field and keeps its name.
		"type Box struct { Width int }",
		"return b.Width",
	)
	assertContains(t, root, "shapes/shapes_test.go", "Rect{Wide: 1, Height: 2}", "scaled.Wide != 3")
	assertContains(t, root, "app/app.go",
		"shapes.Rect{Wide: 2, Height: 3}",
		"shapes.Area(bigger.Wide, bigger.Height)",
		"shapes.Box{Width: 7}",
		"box.Width + box.Size()",
	)
}

func TestRule6_RenameMethodOnDeclarationAndSelectors(t *testing.T) {
	root := copyFixture(t, "rename")
	before := readTree(t, root)

	written, err := refactor.Rename(context.Background(), root, sym("shapes", "Rect.Scale"), "Grow", refactor.Options{})
	if err != nil {
		t.Fatalf("Rename: %v", err)
	}

	assertWritten(t, root, before, written, []string{"app/app.go", "shapes/shapes.go", "shapes/shapes_test.go"}, nil)
	assertModuleBuilds(t, root)
	assertContains(t, root, "shapes/shapes.go",
		"// Grow multiplies both sides by factor.\nfunc (r Rect) Grow(factor int) Rect",
	)
	assertContains(t, root, "shapes/shapes_test.go", "Rect{Width: 1, Height: 2}.Grow(3)")
	assertContains(t, root, "app/app.go", "rect.Grow(2)")
	for _, file := range []string{"app/app.go", "shapes/shapes.go", "shapes/shapes_test.go"} {
		if count := identifierCount(t, root, file, "Scale"); count != 0 {
			t.Errorf("%s still has %d identifier(s) named Scale", file, count)
		}
	}
}

func TestRule6_RenameMethodOfInterfaceNeverUsedAsTypeSucceeds(t *testing.T) {
	root := copyFixture(t, "rename")
	before := readTree(t, root)

	written, err := refactor.Rename(context.Background(), root, sym("shapes", "Box.Size"), "Measure", refactor.Options{})
	if err != nil {
		t.Fatalf("Rename: %v", err)
	}

	assertWritten(t, root, before, written, []string{"app/app.go", "shapes/shapes.go"}, nil)
	assertModuleBuilds(t, root)
	assertContains(t, root, "shapes/shapes.go",
		"func (b Box) Measure() int",
		// Only the type's method is renamed; the interface keeps its method name.
		"type Sizer interface { Size() int }",
	)
	assertContains(t, root, "app/app.go", "box.Width + box.Measure()")
}

func TestRule6_RenameMethodBreakingInterfaceUsedAsTypeFails(t *testing.T) {
	root := copyFixture(t, "rename")
	before := readTree(t, root)

	_, err := refactor.Rename(context.Background(), root, sym("shapes", "Rect.Perimeter"), "Edge", refactor.Options{})

	assertErrorIs(t, err, refactor.ErrBreaksInterface)
	assertTreeUnchanged(t, root, before)
}

func TestRule5_RenameRejectsInvalidIdentifiers(t *testing.T) {
	for _, badName := range []string{"", "1area", "two words", "area-size", "func"} {
		t.Run(badName, func(t *testing.T) {
			root := copyFixture(t, "rename")
			before := readTree(t, root)

			_, err := refactor.Rename(context.Background(), root, sym("shapes", "Area"), badName, refactor.Options{})

			assertErrorIs(t, err, refactor.ErrBadName)
			assertTreeUnchanged(t, root, before)
		})
	}
}

func TestRule5_RenameToSameNameIsNoOp(t *testing.T) {
	root := copyFixture(t, "rename")
	before := readTree(t, root)

	written, err := refactor.Rename(context.Background(), root, sym("shapes", "Area"), "Area", refactor.Options{})
	if err != nil {
		t.Fatalf("Rename: %v", err)
	}

	if len(written) != 0 {
		t.Errorf("want no files written, got %v", written)
	}
	assertTreeUnchanged(t, root, before)
}

func TestRule7And8_RenameConflicts(t *testing.T) {
	cases := []struct {
		name    string
		symbol  string
		newName string
	}{
		{"package level function", "Area", "Describe"},
		{"package level type", "Area", "Rect"},
		{"import in another file scope", "helper", "strings"},
		{"method set", "Rect.Scale", "Perimeter"},
		{"method against field", "Rect.Scale", "Height"},
		{"field list", "Rect.Width", "Height"},
		{"field against method", "Rect.Width", "Perimeter"},
		{"shadowed by local at reference site", "helper", "assist"},
		{"exported to unexported with reference in another package", "Area", "area"},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			root := copyFixture(t, "rename")
			before := readTree(t, root)

			_, err := refactor.Rename(context.Background(), root, sym("shapes", testCase.symbol), testCase.newName, refactor.Options{})

			assertErrorIs(t, err, refactor.ErrConflict)
			assertTreeUnchanged(t, root, before)
		})
	}
}

func TestRule7_LocalWithNewNameAwayFromReferencesIsNoConflict(t *testing.T) {
	root := copyFixture(t, "rename")
	before := readTree(t, root)

	written, err := refactor.Rename(context.Background(), root, sym("shapes", "Area"), "Spare", refactor.Options{})
	if err != nil {
		t.Fatalf("Rename: %v", err)
	}

	assertWritten(t, root, before, written, []string{"app/app.go", "shapes/external_test.go", "shapes/shapes.go", "shapes/shapes_test.go"}, nil)
	assertModuleBuilds(t, root)
	assertContains(t, root, "shapes/shapes.go", "func Spare(width, height int) int", "Spare := 5 return Spare")
}

func TestRule8_ExportedToUnexportedWithoutOutsideReferenceSucceeds(t *testing.T) {
	root := copyFixture(t, "rename")
	before := readTree(t, root)

	written, err := refactor.Rename(context.Background(), root, sym("shapes", "Internal"), "internal", refactor.Options{})
	if err != nil {
		t.Fatalf("Rename: %v", err)
	}

	assertWritten(t, root, before, written, []string{"shapes/shapes.go", "shapes/shapes_test.go"}, nil)
	assertModuleBuilds(t, root)
	assertContains(t, root, "shapes/shapes.go", "// internal is exported but only referenced inside this package.\nfunc internal() int")
	assertContains(t, root, "shapes/shapes_test.go", "if internal() != 1")
}
