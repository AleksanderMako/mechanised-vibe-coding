package astedit_test

import (
	"go/ast"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/AleksanderMako/machanised-vibe-coding/astedit"
)

func TestRules14to15_SetDoc(t *testing.T) {
	cases := []struct {
		name     string
		symbol   string
		doc      string
		wantDoc  []string
		gone     []string
		kept     []string
		groupDoc []string // for a grouped spec, the doc of the whole group must stay as it is
	}{
		{
			name:    "function without a doc",
			symbol:  "Helper",
			doc:     "Helper returns one.",
			wantDoc: []string{"// Helper returns one."},
		},
		{
			name:    "type with several lines and an empty line",
			symbol:  "Point",
			doc:     "Point is a 2D location.\n\nIt has two fields.",
			wantDoc: []string{"// Point is a 2D location.", "//", "// It has two fields."},
			gone:    []string{"Point is a location."},
			kept:    []string{"// X is horizontal.", "// x line comment"},
		},
		{
			name:    "interface type",
			symbol:  "Shape",
			doc:     "Shape is anything with an area.",
			wantDoc: []string{"// Shape is anything with an area."},
			gone:    []string{"Shape has an area."},
		},
		{
			name:    "method with value receiver keeps body comments",
			symbol:  "Square.Area",
			doc:     "Area is side squared.",
			wantDoc: []string{"// Area is side squared."},
			gone:    []string{"Area returns the area."},
			kept:    []string{"// multiply the side by itself"},
		},
		{
			name:    "method with pointer receiver",
			symbol:  "Square.Close",
			doc:     "Close does nothing.",
			wantDoc: []string{"// Close does nothing."},
			gone:    []string{"Close closes the square."},
		},
		{
			name:    "single constant sets the declaration doc",
			symbol:  "Version",
			doc:     "Version names the release.",
			wantDoc: []string{"// Version names the release."},
			gone:    []string{"Version is the fixture version."},
		},
		{
			name:     "grouped constant with a doc sets the spec doc",
			symbol:   "Red",
			doc:      "Red comes first.",
			wantDoc:  []string{"// Red comes first."},
			gone:     []string{"Red is the first color."},
			groupDoc: []string{"// Colors used by shapes."},
		},
		{
			name:     "grouped constant without a doc",
			symbol:   "Green",
			doc:      "Green is second.",
			wantDoc:  []string{"// Green is second."},
			groupDoc: []string{"// Colors used by shapes."},
		},
		{
			name:     "grouped constant keeps its line comment",
			symbol:   "Blue",
			doc:      "Blue is third.",
			wantDoc:  []string{"// Blue is third."},
			kept:     []string{"// the last color"},
			groupDoc: []string{"// Colors used by shapes."},
		},
		{
			name:    "variable",
			symbol:  "Count",
			doc:     "Count counts.",
			wantDoc: []string{"// Count counts."},
			gone:    []string{"Count is a variable, not a constant."},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root := copyFixture(t, "shapes")
			path := filepath.Join(root, "shapes.go")
			written := applyOK(t, root, astedit.SetDoc("shapes.go", tc.symbol, tc.doc))
			if len(written) != 1 || written[0] != path {
				t.Errorf("want written [%s], got %v", path, written)
			}
			after := parseGo(t, path)
			if got := commentLines(after.docOf(t, tc.symbol)); !slices.Equal(got, tc.wantDoc) {
				t.Errorf("doc: want %q, got %q", tc.wantDoc, got)
			}
			assertTexts(t, after.source, tc.gone, tc.kept)
			if tc.groupDoc != nil {
				group, _ := after.genDeclOf(tc.symbol)
				if got := commentLines(group.Doc); !slices.Equal(got, tc.groupDoc) {
					t.Errorf("group doc: want %q, got %q", tc.groupDoc, got)
				}
			}
		})
	}
}

func TestRule16_SetDocEmptyRemovesTheDoc(t *testing.T) {
	cases := []struct {
		name     string
		symbol   string
		gone     []string
		kept     []string
		groupDoc []string
	}{
		{name: "type doc", symbol: "Point", gone: []string{"Point is a location."}, kept: []string{"// X is horizontal."}},
		{name: "method doc", symbol: "Square.Area", gone: []string{"Area returns the area."}, kept: []string{"// multiply the side by itself"}},
		{name: "grouped constant doc", symbol: "Red", gone: []string{"Red is the first color."}, groupDoc: []string{"// Colors used by shapes."}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root := copyFixture(t, "shapes")
			path := filepath.Join(root, "shapes.go")
			applyOK(t, root, astedit.SetDoc("shapes.go", tc.symbol, ""))
			after := parseGo(t, path)
			if got := after.docOf(t, tc.symbol); got != nil {
				t.Errorf("want no doc, got %q", commentLines(got))
			}
			assertTexts(t, after.source, tc.gone, tc.kept)
			if tc.groupDoc != nil {
				group, _ := after.genDeclOf(tc.symbol)
				if got := commentLines(group.Doc); !slices.Equal(got, tc.groupDoc) {
					t.Errorf("group doc: want %q, got %q", tc.groupDoc, got)
				}
			}
		})
	}

	t.Run("symbol without a doc is not an error", func(t *testing.T) {
		root := copyFixture(t, "shapes")
		path := filepath.Join(root, "shapes.go")
		applyOK(t, root, astedit.SetDoc("shapes.go", "Helper", ""))
		if got := parseGo(t, path).docOf(t, "Helper"); got != nil {
			t.Errorf("want no doc, got %q", commentLines(got))
		}
	})
}

func TestRule14_SetDocErrors(t *testing.T) {
	cases := []struct {
		name   string
		symbol string
	}{
		{name: "missing symbol", symbol: "Missing"},
		{name: "missing method", symbol: "Square.Missing"},
		{name: "method on a missing type", symbol: "Missing.Area"},
		{name: "a method is not a top level func", symbol: "Area"},
		{name: "a field is not a method", symbol: "Point.X"},
		{name: "type declared inside a function", symbol: "Inner"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root := copyFixture(t, "shapes")
			expectFailure(t, root, astedit.SetDoc("shapes.go", tc.symbol, "Some doc."), astedit.ErrNotFound)
		})
	}
}

func TestRules17to19_SetConst(t *testing.T) {
	cases := []struct {
		name      string
		constName string
		valueSrc  string
		doc       string
		wantValue string
		wantDoc   []string
		kept      []string
	}{
		{
			name:      "single constant, empty doc keeps the doc",
			constName: "Version",
			valueSrc:  `"2.0"`,
			wantValue: `"2.0"`,
			wantDoc:   []string{"// Version is the fixture version."},
		},
		{
			name:      "single constant with a new doc",
			constName: "Version",
			valueSrc:  `"3.0"`,
			doc:       "Version is three.\n\nReally.",
			wantValue: `"3.0"`,
			wantDoc:   []string{"// Version is three.", "//", "// Really."},
		},
		{
			name:      "grouped constant with a value",
			constName: "Red",
			valueSrc:  "iota + 1",
			wantValue: "iota + 1",
			wantDoc:   []string{"// Red is the first color."},
		},
		{
			name:      "grouped constant with an implicit value gets one",
			constName: "Green",
			valueSrc:  "5",
			wantValue: "5",
			wantDoc:   nil,
		},
		{
			name:      "grouped constant with a doc keeps its line comment",
			constName: "Blue",
			valueSrc:  "Red + 2",
			doc:       "Blue is derived.",
			wantValue: "Red + 2",
			wantDoc:   []string{"// Blue is derived."},
			kept:      []string{"// the last color", "// Colors used by shapes."},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root := copyFixture(t, "shapes")
			path := filepath.Join(root, "shapes.go")
			applyOK(t, root, astedit.SetConst("shapes.go", tc.constName, tc.valueSrc, tc.doc))
			after := parseGo(t, path)
			_, spec := after.genDeclOf(tc.constName)
			if spec == nil {
				t.Fatalf("constant %s disappeared", tc.constName)
			}
			values := spec.(*ast.ValueSpec).Values
			if len(values) != 1 {
				t.Fatalf("want one value, got %d:\n%s", len(values), after.source)
			}
			if got := after.text(values[0]); got != tc.wantValue {
				t.Errorf("value: want %s, got %s", tc.wantValue, got)
			}
			if got := commentLines(after.docOf(t, tc.constName)); !slices.Equal(got, tc.wantDoc) {
				t.Errorf("doc: want %q, got %q", tc.wantDoc, got)
			}
			assertTexts(t, after.source, nil, tc.kept)
		})
	}
}

func TestRules17to18_SetConstErrors(t *testing.T) {
	cases := []struct {
		name      string
		constName string
		valueSrc  string
		want      error
	}{
		{name: "variable is not a constant", constName: "Count", valueSrc: "4", want: astedit.ErrNotFound},
		{name: "missing constant", constName: "Missing", valueSrc: "4", want: astedit.ErrNotFound},
		{name: "first of a multi name spec", constName: "Width", valueSrc: "4", want: astedit.ErrAmbiguous},
		{name: "second of a multi name spec", constName: "Height", valueSrc: "4", want: astedit.ErrAmbiguous},
		{name: "incomplete expression", constName: "Version", valueSrc: "1 +", want: astedit.ErrBadSource},
		{name: "statement instead of expression", constName: "Version", valueSrc: "x := 1", want: astedit.ErrBadSource},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root := copyFixture(t, "shapes")
			expectFailure(t, root, astedit.SetConst("shapes.go", tc.constName, tc.valueSrc, ""), tc.want)
		})
	}
}

// assertTexts checks that every gone text is absent and every kept text present.
func assertTexts(t *testing.T, source string, gone, kept []string) {
	t.Helper()
	for _, text := range gone {
		if strings.Contains(source, text) {
			t.Errorf("output still has %q:\n%s", text, source)
		}
	}
	for _, text := range kept {
		if !strings.Contains(source, text) {
			t.Errorf("output lost %q:\n%s", text, source)
		}
	}
}
