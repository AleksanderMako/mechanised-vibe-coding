package symbols_test

import (
	"testing"

	"github.com/AleksanderMako/machanised-vibe-coding/symbols"
)

// referencesOf runs References on a fresh copy of the main fixture and fails on error.
func referencesOf(t *testing.T, symbol symbols.Symbol, opts symbols.Options) (string, []symbols.Reference) {
	t.Helper()
	root := copyFixture(t, "module")
	refs, err := symbols.References(t.Context(), root, symbol, opts)
	if err != nil {
		t.Fatalf("References(%s): %v", symbol, err)
	}
	return root, refs
}

func TestReferencesOfTypeClassifyEveryPosition(t *testing.T) {
	root, refs := referencesOf(t, sym("shapes", "Square"), symbols.Options{})
	assertReferences(t, root, refs, []wantRef{
		{"shapes/shapes.go", "square-receiver", symbols.KindType},
		{"shapes/shapes.go", "square-alias", symbols.KindType},
		{"shapes/shapes.go", "square-result", symbols.KindType},
		{"shapes/shapes.go", "square-literal-in-shapes", symbols.KindType},
		{"shapes/shapes.go", "square-embed", symbols.KindEmbed},
		{"user/user.go", "square-var", symbols.KindType},
		{"user/user.go", "square-embed-struct", symbols.KindEmbed},
		{"user/user.go", "square-param", symbols.KindType},
		{"user/user.go", "square-func-param", symbols.KindType},
		{"user/user.go", "square-conversion", symbols.KindConversion},
		{"user/user.go", "square-slice", symbols.KindType},
		{"user/user.go", "square-composite", symbols.KindType},
	})
}

func TestReferencesOfFunctionIncludeTestFilesAndValues(t *testing.T) {
	root, refs := referencesOf(t, sym("shapes", "NewSquare"), symbols.Options{})
	assertReferences(t, root, refs, []wantRef{
		{"user/user.go", "newsquare-call", symbols.KindCall},
		{"user/user.go", "newsquare-passed", symbols.KindValue},
		{"user/user.go", "newsquare-assigned", symbols.KindValue},
		{"user/user_test.go", "newsquare-test-call", symbols.KindCall},
	})
}

func TestReferencesWithExcludeTestsDropTestFiles(t *testing.T) {
	root, refs := referencesOf(t, sym("shapes", "NewSquare"), symbols.Options{ExcludeTests: true})
	assertReferences(t, root, refs, []wantRef{
		{"user/user.go", "newsquare-call", symbols.KindCall},
		{"user/user.go", "newsquare-passed", symbols.KindValue},
		{"user/user.go", "newsquare-assigned", symbols.KindValue},
	})
	for _, ref := range refs {
		if ref.Test {
			t.Errorf("ExcludeTests: got a test reference %+v", ref)
		}
	}
}

func TestReferencesInFilesSharedByTestVariantsAppearOnce(t *testing.T) {
	// user.go belongs to both the user package and its test variant, so it is loaded twice.
	_, withTests := referencesOf(t, sym("shapes", "Square"), symbols.Options{})
	_, withoutTests := referencesOf(t, sym("shapes", "Square"), symbols.Options{ExcludeTests: true})
	assertSortedAndUnique(t, withTests)
	if len(withTests) != len(withoutTests) {
		t.Errorf("Square has no uses in test files, so loading tests must not add any: %d with tests, %d without", len(withTests), len(withoutTests))
	}
}

func TestReferencesOfValueReceiverMethod(t *testing.T) {
	root, refs := referencesOf(t, sym("shapes", "Square.Area"), symbols.Options{})
	assertReferences(t, root, refs, []wantRef{
		{"user/user.go", "area-call-on-param", symbols.KindCall},
		{"user/user.go", "area-call-on-result", symbols.KindCall},
		{"user/user.go", "area-call", symbols.KindCall},
		{"user/user.go", "area-method-value", symbols.KindValue},
		{"user/user.go", "area-promoted-call", symbols.KindCall},
	})
}

func TestReferencesOfPointerReceiverMethod(t *testing.T) {
	root, refs := referencesOf(t, sym("shapes", "Circle.Area"), symbols.Options{})
	assertReferences(t, root, refs, []wantRef{
		{"user/user.go", "circle-area-call", symbols.KindCall},
	})
}

func TestReferencesOfField(t *testing.T) {
	root, refs := referencesOf(t, sym("shapes", "Square.Side"), symbols.Options{})
	assertReferences(t, root, refs, []wantRef{
		{"shapes/shapes.go", "side-in-method", symbols.KindValue},
		{"shapes/shapes.go", "square-literal-in-shapes", symbols.KindValue},
		{"user/user.go", "square-composite", symbols.KindValue},
		{"user/user.go", "side-selected", symbols.KindValue},
	})
}

func TestReferencesOfGenericFunctionCountEveryInstantiation(t *testing.T) {
	root, refs := referencesOf(t, sym("shapes", "Biggest"), symbols.Options{})
	assertReferences(t, root, refs, []wantRef{
		{"user/user.go", "biggest-int", symbols.KindCall},
		{"user/user.go", "biggest-float", symbols.KindCall},
	})
}

func TestReferencesOfInterfaceEmbeddedInInterface(t *testing.T) {
	root, refs := referencesOf(t, sym("shapes", "Shape"), symbols.Options{})
	assertReferences(t, root, refs, []wantRef{
		{"user/user.go", "shape-embed-interface", symbols.KindEmbed},
		{"user/user.go", "shape-var", symbols.KindType},
	})
}

func TestReferencesOfNonStructTypeConversion(t *testing.T) {
	root, refs := referencesOf(t, sym("shapes", "Unit"), symbols.Options{})
	assertReferences(t, root, refs, []wantRef{
		{"user/user.go", "unit-conversion", symbols.KindConversion},
	})
}

func TestReferencesOfConstant(t *testing.T) {
	root, refs := referencesOf(t, sym("shapes", "MaxSide"), symbols.Options{})
	assertReferences(t, root, refs, []wantRef{
		{"user/user.go", "maxside-value", symbols.KindValue},
	})
}

func TestReferencesOfStandardLibrarySymbols(t *testing.T) {
	root, refs := referencesOf(t, symbols.Symbol{Package: "io", Name: "Reader"}, symbols.Options{})
	assertReferences(t, root, refs, []wantRef{
		{"shapes/shapes.go", "reader-embed", symbols.KindEmbed},
		{"user/user.go", "reader-param", symbols.KindType},
	})

	root, refs = referencesOf(t, symbols.Symbol{Package: "strings", Name: "ToUpper"}, symbols.Options{})
	assertReferences(t, root, refs, []wantRef{
		{"user/user.go", "toupper-call", symbols.KindCall},
	})
}

func TestReferencesRespectPatterns(t *testing.T) {
	root, refs := referencesOf(t, sym("shapes", "Square"), symbols.Options{Patterns: []string{"./shapes"}})
	assertReferences(t, root, refs, []wantRef{
		{"shapes/shapes.go", "square-receiver", symbols.KindType},
		{"shapes/shapes.go", "square-alias", symbols.KindType},
		{"shapes/shapes.go", "square-result", symbols.KindType},
		{"shapes/shapes.go", "square-literal-in-shapes", symbols.KindType},
		{"shapes/shapes.go", "square-embed", symbols.KindEmbed},
	})
}
