package astedit_test

import (
	"go/ast"
	"path/filepath"
	"strings"
	"testing"

	"github.com/AleksanderMako/mechanised-vibe-coding/astedit"
)

// tableIn returns the single typed slice or map literal in the body of the top level test function.
func (p parsedFile) tableIn(t *testing.T, testFunc string) *ast.CompositeLit {
	t.Helper()
	function := p.function("", testFunc)
	if function == nil {
		t.Fatalf("function %s not found", testFunc)
	}
	var tables []*ast.CompositeLit
	ast.Inspect(function.Body, func(node ast.Node) bool {
		literal, ok := node.(*ast.CompositeLit)
		if !ok || literal.Type == nil {
			return true
		}
		switch literal.Type.(type) {
		case *ast.ArrayType, *ast.MapType:
			tables = append(tables, literal)
		}
		return true
	})
	if len(tables) != 1 {
		t.Fatalf("want one table in %s, found %d", testFunc, len(tables))
	}
	return tables[0]
}

func TestRules7to9_AddTableCaseAppendsOnItsOwnLine(t *testing.T) {
	cases := []struct {
		name     string
		testFunc string
		caseSrc  string
	}{
		{name: "slice of anonymous struct assigned with :=", testFunc: "TestAssigned", caseSrc: `{name: "three", want: 3}`},
		{name: "slice of named type assigned with =", testFunc: "TestNamedType", caseSrc: `{a: 2, b: 2, want: 4}`},
		{name: "table ranged over directly", testFunc: "TestRanged", caseSrc: `{in: "b"}`},
		{name: "map table takes a key value pair", testFunc: "TestMap", caseSrc: `"second": {want: 2}`},
		{name: "table inside a nested block", testFunc: "TestNestedBlock", caseSrc: `{in: 2}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root := copyFixture(t, "tables")
			path := filepath.Join(root, "tables_test.go")
			before := parseGo(t, path).tableIn(t, tc.testFunc)
			elementsBefore := len(before.Elts)

			written := applyOK(t, root, astedit.AddTableCase("tables_test.go", tc.testFunc, tc.caseSrc))
			if len(written) != 1 || written[0] != path {
				t.Errorf("want written [%s], got %v", path, written)
			}

			after := parseGo(t, path)
			table := after.tableIn(t, tc.testFunc)
			if len(table.Elts) != elementsBefore+1 {
				t.Fatalf("want %d elements, got %d", elementsBefore+1, len(table.Elts))
			}
			last := table.Elts[len(table.Elts)-1]
			if got := after.text(last); got != tc.caseSrc {
				t.Errorf("last element: want %s, got %s", tc.caseSrc, got)
			}
			previous := table.Elts[len(table.Elts)-2]
			if after.line(last.Pos()) <= after.line(previous.End()) {
				t.Errorf("new element shares a line with the previous element:\n%s", after.source)
			}
			if after.line(table.Rbrace) <= after.line(last.End()) {
				t.Errorf("closing brace shares a line with the new element:\n%s", after.source)
			}
		})
	}
}

func TestRule9_AddTableCaseKeepsComments(t *testing.T) {
	root := copyFixture(t, "tables")
	path := filepath.Join(root, "tables_test.go")
	caseSrc := "// three is new\n{name: \"three\", want: 3}"
	applyOK(t, root, astedit.AddTableCase(path, "TestAssigned", caseSrc))
	after := parseGo(t, path)
	table := after.tableIn(t, "TestAssigned")
	if got := after.text(table.Elts[len(table.Elts)-1]); got != `{name: "three", want: 3}` {
		t.Errorf("last element: got %s", got)
	}
	if !strings.Contains(after.source, "// three is new") {
		t.Errorf("comment of the new case was lost:\n%s", after.source)
	}
	if !strings.Contains(after.source, "// first case") {
		t.Errorf("existing comment in the table was lost:\n%s", after.source)
	}
}

func TestRules6to8_AddTableCaseErrors(t *testing.T) {
	cases := []struct {
		name     string
		testFunc string
		caseSrc  string
		want     error
	}{
		{name: "function missing", testFunc: "TestMissing", caseSrc: `{a: 1}`, want: astedit.ErrNotFound},
		{name: "method is not a target", testFunc: "TestMethod", caseSrc: `{a: 2}`, want: astedit.ErrNotFound},
		{name: "function without a table", testFunc: "TestNoTable", caseSrc: `{a: 1}`, want: astedit.ErrNotFound},
		{name: "two tables", testFunc: "TestTwoTables", caseSrc: `{a: 2}`, want: astedit.ErrAmbiguous},
		{name: "unbalanced braces", testFunc: "TestAssigned", caseSrc: `{name: "x"`, want: astedit.ErrBadSource},
		{name: "not an expression", testFunc: "TestAssigned", caseSrc: `)(`, want: astedit.ErrBadSource},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root := copyFixture(t, "tables")
			expectFailure(t, root, astedit.AddTableCase("tables_test.go", tc.testFunc, tc.caseSrc), tc.want)
		})
	}
}
