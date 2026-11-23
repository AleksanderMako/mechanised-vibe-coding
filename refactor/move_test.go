package refactor_test

import (
	"context"
	"strings"
	"testing"

	"github.com/AleksanderMako/mechanised-vibe-coding/refactor"
	"github.com/AleksanderMako/mechanised-vibe-coding/symbols"
)

func TestRule11And15_MoveIntoNewPackage(t *testing.T) {
	root := copyFixture(t, "move")
	before := readTree(t, root)

	moved := []symbols.Symbol{sym("store", "Format"), sym("store", "Label")}
	written, err := refactor.Move(context.Background(), root, moved, importPath("text"), "format.go", refactor.Options{})
	if err != nil {
		t.Fatalf("Move: %v", err)
	}

	changed := []string{"consumer/consumer.go", "report/report.go", "store/store.go", "store/store_test.go", "text/format.go"}
	assertWritten(t, root, before, written, changed, nil)
	assertModuleBuilds(t, root)

	assertPackageClause(t, root, "text/format.go", "text")
	assertContains(t, root, "text/format.go",
		"// Format renders a label for display.\nfunc Format(label Label) string",
		"// Label is a display label.\ntype Label string",
	)
	// References inside the source package become qualified.
	assertContains(t, root, "store/store.go", `"example.com/fixture/text"`, "return text.Format(text.Label(title))")
	assertContains(t, root, "store/store_test.go", `text.Format("x")`)
	// References in importers switch to the destination package.
	assertContains(t, root, "consumer/consumer.go",
		`"example.com/fixture/text"`,
		`text.Format(text.Label("a"))`,
		`store.Banner("b")`,
	)
	assertContains(t, root, "report/report.go", "return text.Format(text.Label(name))")
}

func TestRule12_MoveRemovesDeclarationsDocCommentsAndUnusedImports(t *testing.T) {
	root := copyFixture(t, "move")

	moved := []symbols.Symbol{sym("store", "Format"), sym("store", "Label")}
	if _, err := refactor.Move(context.Background(), root, moved, importPath("text"), "format.go", refactor.Options{}); err != nil {
		t.Fatalf("Move: %v", err)
	}

	assertModuleBuilds(t, root)
	assertNotContains(t, root, "store/store.go",
		"func Format(",
		"type Label string",
		"// Format renders a label for display.",
		"// Label is a display label.",
		// Only the moved Format used fmt.
		`"fmt"`,
	)
	// Shout and others still use strings, so the import stays.
	assertContains(t, root, "store/store.go", `"strings"`)
}

func TestRule11_MoveIntoNestedNewPackageUsesLastPathElement(t *testing.T) {
	root := copyFixture(t, "move")
	before := readTree(t, root)

	written, err := refactor.Move(context.Background(), root, []symbols.Symbol{sym("store", "Join")}, importPath("lib/textfmt"), "join.go", refactor.Options{})
	if err != nil {
		t.Fatalf("Move: %v", err)
	}

	assertWritten(t, root, before, written, []string{"audit/audit.go", "consumer/consumer.go", "lib/textfmt/join.go", "store/store.go"}, nil)
	assertModuleBuilds(t, root)
	assertPackageClause(t, root, "lib/textfmt/join.go", "textfmt")
	assertContains(t, root, "lib/textfmt/join.go", "// Join joins words with spaces.\nfunc Join(words []string) string")
	assertContains(t, root, "consumer/consumer.go", `"example.com/fixture/lib/textfmt"`, `textfmt.Join([]string{"f", "g"})`)
	assertContains(t, root, "audit/audit.go", "return textfmt.Join(words)")
}

func TestRule11And14_MoveAppendsToExistingFileAndKeepsNameUsedOnlyInDestination(t *testing.T) {
	root := copyFixture(t, "move")
	before := readTree(t, root)

	moved := []symbols.Symbol{sym("store", "Shout"), sym("store", "suffix")}
	written, err := refactor.Move(context.Background(), root, moved, importPath("util"), "util.go", refactor.Options{})
	if err != nil {
		t.Fatalf("Move: %v", err)
	}

	assertWritten(t, root, before, written, []string{"consumer/consumer.go", "store/store.go", "util/util.go"}, nil)
	assertModuleBuilds(t, root)
	assertPackageClause(t, root, "util/util.go", "util")
	assertContains(t, root, "util/util.go",
		"func Tidy(text string) string",
		"func Join(words []string) string",
		"// Shout upper cases text and appends suffix.\nfunc Shout(text string) string",
		`const suffix = "!"`,
		"strings.ToUpper(text) + suffix",
	)
	if count := identifierCount(t, root, "util/util.go", "Suffix"); count != 0 {
		t.Errorf("suffix is only used inside the destination and must keep its name, found %d Suffix identifier(s)", count)
	}
	assertNotContains(t, root, "store/store.go", "func Shout(", "const suffix")
	assertContains(t, root, "consumer/consumer.go", `util.Shout("c")`)
}

func TestRule11_MoveCreatesFileInExistingPackageWithItsExistingName(t *testing.T) {
	root := copyFixture(t, "move")
	before := readTree(t, root)

	moved := []symbols.Symbol{sym("store", "Tidied"), sym("store", "tidy")}
	written, err := refactor.Move(context.Background(), root, moved, importPath("tools"), "added.go", refactor.Options{})
	if err != nil {
		t.Fatalf("Move: %v", err)
	}

	assertWritten(t, root, before, written, []string{"consumer/consumer.go", "store/store.go", "tools/added.go"}, nil)
	assertModuleBuilds(t, root)
	assertPackageClause(t, root, "tools/added.go", "toolkit")
	assertContains(t, root, "tools/added.go", "func Tidied(text string) string", "func tidy(text string) string")
	assertContains(t, root, "consumer/consumer.go", `"example.com/fixture/tools"`, `toolkit.Tidied("E")`)
}

func TestRule12_MoveDeletesSourceFileLeftWithoutDeclarations(t *testing.T) {
	root := copyFixture(t, "move")
	before := readTree(t, root)

	written, err := refactor.Move(context.Background(), root, []symbols.Symbol{sym("store", "Pad")}, importPath("util"), "util.go", refactor.Options{})
	if err != nil {
		t.Fatalf("Move: %v", err)
	}

	assertWritten(t, root, before, written, []string{"consumer/consumer.go", "util/util.go"}, []string{"store/extra.go"})
	assertModuleBuilds(t, root)
	assertContains(t, root, "util/util.go", "// Pad adds a space on each side.\nfunc Pad(text string) string")
	assertContains(t, root, "consumer/consumer.go", `util.Pad("h")`)
}

func TestRule15_MoveDropsQualifierInsideDestination(t *testing.T) {
	root := copyFixture(t, "move")
	before := readTree(t, root)

	moved := []symbols.Symbol{sym("store", "Format"), sym("store", "Label")}
	written, err := refactor.Move(context.Background(), root, moved, importPath("report"), "report.go", refactor.Options{})
	if err != nil {
		t.Fatalf("Move: %v", err)
	}

	assertWritten(t, root, before, written, []string{"consumer/consumer.go", "report/report.go", "store/store.go", "store/store_test.go"}, nil)
	assertModuleBuilds(t, root)
	assertContains(t, root, "report/report.go", "return Format(Label(name))", "func Format(label Label) string")
	assertNotContains(t, root, "report/report.go", `"example.com/fixture/store"`, "store.")
	assertContains(t, root, "store/store.go", "return report.Format(report.Label(title))")
	assertContains(t, root, "consumer/consumer.go", `report.Format(report.Label("a"))`)
}

func TestRule14_MoveExportsUnexportedSymbolReferencedOutsideDestination(t *testing.T) {
	root := copyFixture(t, "move")
	before := readTree(t, root)

	written, err := refactor.Move(context.Background(), root, []symbols.Symbol{sym("store", "clean")}, importPath("util"), "util.go", refactor.Options{})
	if err != nil {
		t.Fatalf("Move: %v", err)
	}

	assertWritten(t, root, before, written, []string{"store/store.go", "store/store_test.go", "util/util.go"}, nil)
	assertModuleBuilds(t, root)
	// Rule 4 rewrites the doc comment's first word together with the export.
	assertContains(t, root, "util/util.go", "// Clean trims surrounding spaces.\nfunc Clean(text string) string")
	if count := identifierCount(t, root, "util/util.go", "clean"); count != 0 {
		t.Errorf("util/util.go still has %d identifier(s) named clean", count)
	}
	assertContains(t, root, "store/store.go", "return util.Clean(text)")
	assertContains(t, root, "store/store_test.go", `util.Clean(" z ")`)
}

func TestRule13_MoveRejectsDependencyOnUnexportedSymbolLeftBehind(t *testing.T) {
	root := copyFixture(t, "move")
	before := readTree(t, root)

	_, err := refactor.Move(context.Background(), root, []symbols.Symbol{sym("store", "Shout")}, importPath("util"), "util.go", refactor.Options{})

	assertErrorIs(t, err, refactor.ErrLeftBehind)
	if !strings.Contains(err.Error(), "suffix") {
		t.Errorf("want the error to name suffix, got %v", err)
	}
	assertTreeUnchanged(t, root, before)
}

func TestRule14And16_MoveConflicts(t *testing.T) {
	cases := []struct {
		name  string
		moved string
	}{
		// util already declares Join.
		{"same name already in destination", "Join"},
		// tidy is still used by store.Tidied, so it is exported to Tidy, which util already declares.
		{"exported name clashes in destination", "tidy"},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			root := copyFixture(t, "move")
			before := readTree(t, root)

			_, err := refactor.Move(context.Background(), root, []symbols.Symbol{sym("store", testCase.moved)}, importPath("util"), "util.go", refactor.Options{})

			assertErrorIs(t, err, refactor.ErrConflict)
			assertTreeUnchanged(t, root, before)
		})
	}
}

func TestRule10And11_MoveRejectsInvalidRequests(t *testing.T) {
	cases := []struct {
		name         string
		moved        []symbols.Symbol
		toImportPath string
	}{
		{"empty symbol list", nil, importPath("util")},
		{"symbols from two packages", []symbols.Symbol{sym("store", "Format"), sym("util", "Tidy")}, importPath("report")},
		{"destination outside the module", []symbols.Symbol{sym("store", "Pad")}, "example.com/elsewhere/text"},
		{"destination in the standard library", []symbols.Symbol{sym("store", "Pad")}, "strings"},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			root := copyFixture(t, "move")
			before := readTree(t, root)

			_, err := refactor.Move(context.Background(), root, testCase.moved, testCase.toImportPath, "moved.go", refactor.Options{})

			if err == nil {
				t.Fatal("want an error, got nil")
			}
			assertNotBuildError(t, err)
			assertTreeUnchanged(t, root, before)
		})
	}
}
