package main_test

import (
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/AleksanderMako/mechanised-vibe-coding/symbols"
)

// referenceLine is rule 5's FILE:LINE:COL\tKIND\tCONTEXT.
var referenceLine = regexp.MustCompile(`^.+:\d+:\d+\t(call|conversion|type|embed|value)\t.+$`)

// newSquareSymbol is called from alpha, beta, gamma and the shapes test.
const newSquareSymbol = "example.com/fixture/shapes.NewSquare"

// referencePackages returns the distinct packages of refs, sorted.
func referencePackages(refs []symbols.Reference) []string {
	var packages []string
	for _, ref := range refs {
		if !slices.Contains(packages, ref.Package) {
			packages = append(packages, ref.Package)
		}
	}
	slices.Sort(packages)
	return packages
}

// Rule 5: every text line of refs is FILE:LINE:COL, kind and context, tab separated.
func TestSymbolsRefsText(t *testing.T) {
	module := copyModule(t)
	result := runMVC(t, module, "symbols", "refs", "-root", module, newSquareSymbol)
	expectExit(t, result, 0)
	lines := nonEmptyLines(result.stdout)
	if len(lines) < 4 {
		t.Fatalf("want at least 4 references, got:\n%s", result.describe())
	}
	for _, line := range lines {
		if !referenceLine.MatchString(line) {
			t.Errorf("line %q does not match %s", line, referenceLine)
		}
	}
	if !strings.Contains(result.stdout, "alpha.go:") || !strings.Contains(result.stdout, "\tcall\t") {
		t.Errorf("the call in alpha.go is not listed:\n%s", result.describe())
	}
}

// Rule 4: refs -json decodes into []symbols.Reference.
func TestSymbolsRefsJSON(t *testing.T) {
	module := copyModule(t)
	result := runMVC(t, module, "symbols", "refs", "-root", module, "-json", newSquareSymbol)
	expectExit(t, result, 0)
	var refs []symbols.Reference
	decodeJSON(t, result, &refs)
	for _, wantPackage := range []string{"example.com/fixture/alpha", "example.com/fixture/beta", "example.com/fixture/gamma"} {
		if !slices.Contains(referencePackages(refs), wantPackage) {
			t.Errorf("no reference in %s: %+v", wantPackage, refs)
		}
	}
}

// Rule 2: two -pkg flags restrict the references to exactly those packages.
func TestSymbolsRefsRepeatedPkg(t *testing.T) {
	module := copyModule(t)
	result := runMVC(t, module, "symbols", "refs", "-root", module, "-pkg", "./alpha", "-pkg", "./beta", "-json", newSquareSymbol)
	expectExit(t, result, 0)
	var refs []symbols.Reference
	decodeJSON(t, result, &refs)
	got := strings.Join(referencePackages(refs), ",")
	if got != "example.com/fixture/alpha,example.com/fixture/beta" {
		t.Errorf("reference packages = %s, want alpha and beta only", got)
	}
}

// Rule 1: without -root the current directory is the module root.
func TestSymbolsRefsDefaultRoot(t *testing.T) {
	module := copyModule(t)
	result := runMVC(t, module, "symbols", "refs", "-json", newSquareSymbol)
	expectExit(t, result, 0)
	var refs []symbols.Reference
	decodeJSON(t, result, &refs)
	if len(refs) == 0 {
		t.Fatal("no references found from the current directory")
	}
	for _, ref := range refs {
		if !strings.HasPrefix(ref.File, module+"/") {
			t.Errorf("reference file %s is not under %s", ref.File, module)
		}
	}
}

// Rule 3: a symbol that does not resolve is an ordinary error.
func TestSymbolsRefsUnknownSymbolIsError(t *testing.T) {
	module := copyModule(t)
	expectOrdinaryError(t, runMVC(t, module, "symbols", "refs", "-root", module, "example.com/fixture/shapes.NoSuch"))
}

// Rule 4: impls -json decodes into []symbols.Symbol and lists Square.
func TestSymbolsImplsJSON(t *testing.T) {
	module := copyModule(t)
	result := runMVC(t, module, "symbols", "impls", "-root", module, "-json", "example.com/fixture/shapes.Shape")
	expectExit(t, result, 0)
	var implementations []symbols.Symbol
	decodeJSON(t, result, &implementations)
	want := symbols.Symbol{Package: "example.com/fixture/shapes", Name: "Square"}
	if !slices.Contains(implementations, want) {
		t.Errorf("implementations = %+v, want %+v among them", implementations, want)
	}
}

// Rule 4: tags -json decodes into []symbols.FieldTag in declaration order.
func TestSymbolsTagsJSON(t *testing.T) {
	module := copyModule(t)
	result := runMVC(t, module, "symbols", "tags", "-root", module, "-json", "example.com/fixture/shapes.Square")
	expectExit(t, result, 0)
	var fields []symbols.FieldTag
	decodeJSON(t, result, &fields)
	if len(fields) != 2 || fields[0].Field != "Side" || fields[0].JSON != "side" || fields[1].Field != "Label" || !fields[1].OmitEmpty {
		t.Errorf("fields = %+v, want Side (json side) then Label (omitempty)", fields)
	}
}

// Rule 5: tags text prints one line per field.
func TestSymbolsTagsText(t *testing.T) {
	module := copyModule(t)
	result := runMVC(t, module, "symbols", "tags", "-root", module, "example.com/fixture/shapes.Square")
	expectExit(t, result, 0)
	lines := nonEmptyLines(result.stdout)
	if len(lines) != 2 || !strings.Contains(lines[0], "Side") || !strings.Contains(lines[1], "Label") {
		t.Errorf("want one line for Side then one for Label:\n%s", result.describe())
	}
}
