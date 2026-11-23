package main_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"strings"
	"testing"
)

// parseGoFile parses dir/relative with comments and fails the test when it does not parse.
func parseGoFile(t *testing.T, dir, relative string) *ast.File {
	t.Helper()
	file, err := parser.ParseFile(token.NewFileSet(), filepath.Join(dir, relative), nil, parser.ParseComments)
	if err != nil {
		t.Fatalf("parse %s: %v", relative, err)
	}
	return file
}

// expectPrintedPath checks rule 8: the written file's absolute path is printed on a line of its own.
func expectPrintedPath(t *testing.T, result runResult, wantPath string) {
	t.Helper()
	expectExit(t, result, 0)
	lines := nonEmptyLines(result.stdout)
	if len(lines) != 1 || strings.TrimSpace(lines[0]) != wantPath {
		t.Fatalf("want the single line %s on stdout, got:\n%s", wantPath, result.describe())
	}
}

// squareFieldNames returns the field names of struct Square in declaration order.
func squareFieldNames(t *testing.T, file *ast.File) []string {
	t.Helper()
	for _, declaration := range file.Decls {
		general, ok := declaration.(*ast.GenDecl)
		if !ok {
			continue
		}
		for _, spec := range general.Specs {
			typeSpec, ok := spec.(*ast.TypeSpec)
			if !ok || typeSpec.Name.Name != "Square" {
				continue
			}
			var names []string
			for _, field := range typeSpec.Type.(*ast.StructType).Fields.List {
				for _, name := range field.Names {
					names = append(names, name.Name)
				}
			}
			return names
		}
	}
	t.Fatal("struct Square not found")
	return nil
}

// Rule 8: table-case appends one element to the table of the test function.
func TestASTTableCase(t *testing.T) {
	module := copyModule(t)
	result := runMVC(t, module, "ast", "table-case", "-root", module,
		"-file", "shapes/shapes_test.go", "-func", "TestArea", "-case", `{name: "three", side: 3, want: 9}`)
	expectPrintedPath(t, result, filepath.Join(module, "shapes", "shapes_test.go"))

	file := parseGoFile(t, module, "shapes/shapes_test.go")
	elementCount := -1
	ast.Inspect(file, func(node ast.Node) bool {
		literal, ok := node.(*ast.CompositeLit)
		if !ok {
			return true
		}
		if _, isSlice := literal.Type.(*ast.ArrayType); isSlice {
			elementCount = len(literal.Elts)
			return false
		}
		return true
	})
	if elementCount != 3 {
		t.Errorf("table has %d elements, want 3", elementCount)
	}
	if !strings.Contains(readFile(t, module, "shapes/shapes_test.go"), `"three"`) {
		t.Error("new case text is missing")
	}
}

// Rule 8: add-field inserts the field after the named one.
func TestASTAddField(t *testing.T) {
	module := copyModule(t)
	result := runMVC(t, module, "ast", "add-field", "-root", module,
		"-file", "shapes/shapes.go", "-type", "Square", "-field", "Color string `json:\"color\"`", "-after", "Side")
	expectPrintedPath(t, result, filepath.Join(module, "shapes", "shapes.go"))

	names := squareFieldNames(t, parseGoFile(t, module, "shapes/shapes.go"))
	if strings.Join(names, ",") != "Side,Color,Label" {
		t.Errorf("fields = %v, want [Side Color Label]", names)
	}
}

// Rule 8: remove-field drops the named field.
func TestASTRemoveField(t *testing.T) {
	module := copyModule(t)
	result := runMVC(t, module, "ast", "remove-field", "-root", module,
		"-file", "shapes/shapes.go", "-type", "Square", "-name", "Label")
	expectPrintedPath(t, result, filepath.Join(module, "shapes", "shapes.go"))

	names := squareFieldNames(t, parseGoFile(t, module, "shapes/shapes.go"))
	if strings.Join(names, ",") != "Side" {
		t.Errorf("fields = %v, want [Side]", names)
	}
}

// Rule 8: set-doc replaces the doc comment of a function.
func TestASTSetDoc(t *testing.T) {
	module := copyModule(t)
	result := runMVC(t, module, "ast", "set-doc", "-root", module,
		"-file", "shapes/shapes.go", "-symbol", "NewSquare", "-doc", "NewSquare makes a square of the given side.")
	expectPrintedPath(t, result, filepath.Join(module, "shapes", "shapes.go"))

	for _, declaration := range parseGoFile(t, module, "shapes/shapes.go").Decls {
		function, ok := declaration.(*ast.FuncDecl)
		if !ok || function.Name.Name != "NewSquare" {
			continue
		}
		if got := strings.TrimSpace(function.Doc.Text()); got != "NewSquare makes a square of the given side." {
			t.Errorf("doc = %q", got)
		}
		return
	}
	t.Fatal("func NewSquare not found")
}

// Rule 8: set-const replaces the value and the doc of a constant.
func TestASTSetConst(t *testing.T) {
	module := copyModule(t)
	result := runMVC(t, module, "ast", "set-const", "-root", module,
		"-file", "shapes/shapes.go", "-name", "Version", "-value", `"2.0"`, "-doc", "Version is bumped.")
	expectPrintedPath(t, result, filepath.Join(module, "shapes", "shapes.go"))

	for _, declaration := range parseGoFile(t, module, "shapes/shapes.go").Decls {
		general, ok := declaration.(*ast.GenDecl)
		if !ok || general.Tok != token.CONST {
			continue
		}
		valueSpec := general.Specs[0].(*ast.ValueSpec)
		if valueSpec.Names[0].Name != "Version" {
			continue
		}
		if literal, ok := valueSpec.Values[0].(*ast.BasicLit); !ok || literal.Value != `"2.0"` {
			t.Errorf("value = %#v, want \"2.0\"", valueSpec.Values[0])
		}
		if got := strings.TrimSpace(general.Doc.Text()); got != "Version is bumped." {
			t.Errorf("doc = %q", got)
		}
		return
	}
	t.Fatal("const Version not found")
}

// Rule 8: conformance adds `var _ Shape = (*Square)(nil)`.
func TestASTConformance(t *testing.T) {
	module := copyModule(t)
	result := runMVC(t, module, "ast", "conformance", "-root", module,
		"-file", "shapes/shapes.go", "-type", "Square", "-iface", "Shape")
	expectPrintedPath(t, result, filepath.Join(module, "shapes", "shapes.go"))

	for _, declaration := range parseGoFile(t, module, "shapes/shapes.go").Decls {
		general, ok := declaration.(*ast.GenDecl)
		if !ok || general.Tok != token.VAR {
			continue
		}
		valueSpec := general.Specs[0].(*ast.ValueSpec)
		if typeName, ok := valueSpec.Type.(*ast.Ident); ok && valueSpec.Names[0].Name == "_" && typeName.Name == "Shape" {
			return
		}
	}
	t.Fatal("no `var _ Shape = ...` declaration was added")
}

// Rule 3: an op whose target does not exist is an ordinary error and writes nothing.
func TestASTMissingTargetIsError(t *testing.T) {
	module := copyModule(t)
	before := readFile(t, module, "shapes/shapes.go")
	result := runMVC(t, module, "ast", "set-doc", "-root", module,
		"-file", "shapes/shapes.go", "-symbol", "NoSuchSymbol", "-doc", "text")
	expectOrdinaryError(t, result)
	if readFile(t, module, "shapes/shapes.go") != before {
		t.Error("file changed despite the error")
	}
}
