package symbols_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/AleksanderMako/machanised-vibe-coding/symbols"
)

// brokenPackage is the fixture package with a deliberate type error.
const brokenPackage = "example.com/broken/bad"

// callAll runs References, Implementations and StructTags with valid looking symbols and reports each error.
func callAll(t *testing.T, root string, reference, iface, structType symbols.Symbol, opts symbols.Options) map[string]error {
	t.Helper()
	refs, refsErr := symbols.References(t.Context(), root, reference, opts)
	if refsErr != nil && refs != nil {
		t.Errorf("References returned results together with error %v: %+v", refsErr, refs)
	}
	impls, implsErr := symbols.Implementations(t.Context(), root, iface, opts)
	if implsErr != nil && impls != nil {
		t.Errorf("Implementations returned results together with error %v: %+v", implsErr, impls)
	}
	fields, fieldsErr := symbols.StructTags(t.Context(), root, structType, opts)
	if fieldsErr != nil && fields != nil {
		t.Errorf("StructTags returned results together with error %v: %+v", fieldsErr, fields)
	}
	return map[string]error{"References": refsErr, "Implementations": implsErr, "StructTags": fieldsErr}
}

func TestEmptyRootIsAnOrdinaryError(t *testing.T) {
	errs := callAll(t, "", sym("shapes", "Square"), sym("shapes", "Shape"), sym("shapes", "Tagged"), symbols.Options{})
	for function, err := range errs {
		if err == nil {
			t.Errorf("%s with empty root: want error", function)
		} else if errors.Is(err, symbols.ErrNotFound) {
			t.Errorf("%s with empty root: want an ordinary error, got ErrNotFound: %v", function, err)
		}
	}
}

func TestRootWithoutGoModIsAnOrdinaryError(t *testing.T) {
	root := t.TempDir()
	errs := callAll(t, root, sym("shapes", "Square"), sym("shapes", "Shape"), sym("shapes", "Tagged"), symbols.Options{})
	for function, err := range errs {
		if err == nil {
			t.Errorf("%s with root lacking go.mod: want error", function)
		} else if errors.Is(err, symbols.ErrNotFound) {
			t.Errorf("%s with root lacking go.mod: want an ordinary error, got ErrNotFound: %v", function, err)
		}
	}
}

func TestTypeErrorFailsTheWholeCallAndNamesThePackage(t *testing.T) {
	root := copyFixture(t, "broken")
	good := func(name string) symbols.Symbol {
		return symbols.Symbol{Package: "example.com/broken/good", Name: name}
	}
	// Stage 9 made failing on any broken package the Strict behaviour.
	errs := callAll(t, root, good("Fine"), good("Sizer"), good("Box"), symbols.Options{Strict: true})
	for function, err := range errs {
		if err == nil {
			t.Errorf("%s on a module with a type error: want error", function)
			continue
		}
		if !strings.Contains(err.Error(), brokenPackage) {
			t.Errorf("%s: error must name package %s, got %v", function, brokenPackage, err)
		}
	}
}

func TestPatternsLimitLoadingToTheHealthyPackage(t *testing.T) {
	root := copyFixture(t, "broken")
	fine := symbols.Symbol{Package: "example.com/broken/good", Name: "Fine"}
	refs, err := symbols.References(t.Context(), root, fine, symbols.Options{Patterns: []string{"./good"}})
	if err != nil {
		t.Fatalf("References with Patterns ./good: unexpected error %v", err)
	}
	if len(refs) != 0 {
		t.Errorf("Fine is never used, want no references, got %+v", refs)
	}
}

func TestUnknownNameInLoadedPackageWrapsErrNotFound(t *testing.T) {
	root := copyFixture(t, "module")
	missingNames := []symbols.Symbol{
		sym("shapes", "DoesNotExist"),
		sym("shapes", "Square.Missing"),
		{Package: "io", Name: "NoSuchReader"},
	}
	for _, missing := range missingNames {
		errs := callAll(t, root, missing, missing, missing, symbols.Options{})
		for function, err := range errs {
			if !errors.Is(err, symbols.ErrNotFound) {
				t.Errorf("%s(%s): want ErrNotFound, got %v", function, missing, err)
			}
		}
	}
}

func TestUnreachablePackageWrapsErrNotFound(t *testing.T) {
	root := copyFixture(t, "module")
	nowhere := symbols.Symbol{Package: "example.com/elsewhere", Name: "Thing"}
	errs := callAll(t, root, nowhere, nowhere, nowhere, symbols.Options{})
	for function, err := range errs {
		if !errors.Is(err, symbols.ErrNotFound) {
			t.Errorf("%s(%s): want ErrNotFound, got %v", function, nowhere, err)
		}
	}

	// user exists on disk but shapes does not import it, so loading only ./shapes cannot reach it.
	holder := sym("user", "Holder")
	errs = callAll(t, root, holder, holder, holder, symbols.Options{Patterns: []string{"./shapes"}})
	for function, err := range errs {
		if !errors.Is(err, symbols.ErrNotFound) {
			t.Errorf("%s(%s) with Patterns ./shapes: want ErrNotFound, got %v", function, holder, err)
		}
	}
}

func TestDependencySymbolResolvesWithNarrowPatterns(t *testing.T) {
	root := copyFixture(t, "module")
	reader := symbols.Symbol{Package: "io", Name: "Reader"}
	refs, err := symbols.References(t.Context(), root, reader, symbols.Options{Patterns: []string{"./shapes"}})
	if err != nil {
		t.Fatalf("References(io.Reader) with Patterns ./shapes: %v", err)
	}
	assertReferences(t, root, refs, []wantRef{
		{"shapes/shapes.go", "reader-embed", symbols.KindEmbed},
	})
}
