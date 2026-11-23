package astedit_test

import (
	"context"
	"go/ast"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/AleksanderMako/machanised-vibe-coding/astedit"
)

// declAfter returns the top level declaration that directly follows the one declaring name.
func (p parsedFile) declAfter(t *testing.T, name string) ast.Decl {
	t.Helper()
	group, _ := p.genDeclOf(name)
	if group == nil {
		t.Fatalf("%s not found", name)
	}
	for i, decl := range p.file.Decls {
		if decl == group && i+1 < len(p.file.Decls) {
			return p.file.Decls[i+1]
		}
	}
	t.Fatalf("nothing follows the declaration of %s", name)
	return nil
}

// assertConformanceLine checks rule 21 for one inserted line.
func assertConformanceLine(t *testing.T, after parsedFile, typeName, ifaceSrc string) {
	t.Helper()
	next, ok := after.declAfter(t, typeName).(*ast.GenDecl)
	if !ok || next.Tok != token.VAR || next.Lparen.IsValid() || len(next.Specs) != 1 {
		t.Fatalf("want a single ungrouped var directly after %s:\n%s", typeName, after.source)
	}
	spec := next.Specs[0].(*ast.ValueSpec)
	if len(spec.Names) != 1 || spec.Names[0].Name != "_" {
		t.Errorf("want the blank identifier, got %v", spec.Names)
	}
	if spec.Type == nil || after.text(spec.Type) != ifaceSrc {
		t.Errorf("want interface %s:\n%s", ifaceSrc, after.source)
	}
	wantValue := "(*" + typeName + ")(nil)"
	if len(spec.Values) != 1 || after.text(spec.Values[0]) != wantValue {
		t.Errorf("want value %s:\n%s", wantValue, after.source)
	}
	line := "var _ " + ifaceSrc + " = " + wantValue
	lines := strings.Split(after.source, "\n")
	for i, candidate := range lines {
		if candidate != line {
			continue
		}
		if i == 0 || lines[i-1] != "" || i+1 >= len(lines) || lines[i+1] != "" {
			t.Errorf("want a blank line before and after %q:\n%s", line, after.source)
		}
		return
	}
	t.Errorf("line %q not found:\n%s", line, after.source)
}

func TestRules20to21_AddConformance(t *testing.T) {
	cases := []struct {
		name     string
		typeName string
		ifaceSrc string
	}{
		{name: "struct type", typeName: "Point", ifaceSrc: "io.Reader"},
		{name: "struct followed by a doc commented type", typeName: "Empty", ifaceSrc: "fmt.Stringer"},
		{name: "non struct type", typeName: "Name", ifaceSrc: "fmt.Stringer"},
		{name: "interface type", typeName: "Shape", ifaceSrc: "any"},
		{name: "type inside a grouped declaration goes after the group", typeName: "Circle", ifaceSrc: "Shape"},
		{name: "same interface already used for another type", typeName: "Point", ifaceSrc: "io.Closer"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root := copyFixture(t, "shapes")
			path := filepath.Join(root, "shapes.go")
			written := applyOK(t, root, astedit.AddConformance("shapes.go", tc.typeName, tc.ifaceSrc))
			if len(written) != 1 || written[0] != path {
				t.Errorf("want written [%s], got %v", path, written)
			}
			after := parseGo(t, path)
			assertConformanceLine(t, after, tc.typeName, tc.ifaceSrc)
			if strings.Count(after.source, "var _ io.Closer = (*Square)(nil)") != 1 {
				t.Errorf("the existing conformance line must stay once:\n%s", after.source)
			}
		})
	}
}

func TestRule22_AddConformanceExistingLineIsNoOp(t *testing.T) {
	t.Run("line already present further down", func(t *testing.T) {
		root := copyFixture(t, "shapes")
		path := filepath.Join(root, "shapes.go")
		pinned := pinFiles(t, path)
		written, err := astedit.Apply(context.Background(), root, astedit.AddConformance("shapes.go", "Square", "io.Closer"))
		if err != nil {
			t.Fatalf("Apply: %v", err)
		}
		if len(written) != 0 {
			t.Errorf("want nothing written, got %v", written)
		}
		assertNotWritten(t, pinned)
	})

	existingForms := []struct {
		name   string
		source string
	}{
		{name: "unformatted whitespace", source: "package existing\n\nimport \"io\"\n\ntype Point struct{ X int }\n\nfunc Helper() {}\n\nvar  _   io.Closer=( * Point )( nil )\n"},
		{name: "inside a var group", source: "package existing\n\nimport \"io\"\n\ntype Point struct{ X int }\n\nvar (\n\tcount = 1\n\t_     io.Closer = (*Point)(nil)\n)\n"},
	}
	for _, form := range existingForms {
		t.Run(form.name, func(t *testing.T) {
			root := copyFixture(t, "shapes")
			path := filepath.Join(root, "existing", "existing.go")
			if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
				t.Fatal(err)
			}
			writeFile(t, path, form.source)
			pinned := pinFiles(t, path)
			written, err := astedit.Apply(context.Background(), root, astedit.AddConformance("existing/existing.go", "Point", "io.Closer"))
			if err != nil {
				t.Fatalf("Apply: %v", err)
			}
			if len(written) != 0 {
				t.Errorf("want nothing written, got %v", written)
			}
			assertNotWritten(t, pinned)
		})
	}

	t.Run("same op twice in one Apply adds the line once", func(t *testing.T) {
		root := copyFixture(t, "shapes")
		path := filepath.Join(root, "shapes.go")
		written := applyOK(t, root,
			astedit.AddConformance("shapes.go", "Point", "io.Reader"),
			astedit.AddConformance("shapes.go", "Point", "io.Reader"),
		)
		if len(written) != 1 || written[0] != path {
			t.Errorf("want written [%s], got %v", path, written)
		}
		if got := strings.Count(readFile(t, path), "var _ io.Reader = (*Point)(nil)"); got != 1 {
			t.Errorf("want the line once, found %d times", got)
		}
	})

	t.Run("different interface for the same type is not a no-op", func(t *testing.T) {
		root := copyFixture(t, "shapes")
		path := filepath.Join(root, "shapes.go")
		applyOK(t, root, astedit.AddConformance("shapes.go", "Square", "Shape"))
		assertConformanceLine(t, parseGo(t, path), "Square", "Shape")
	})
}

func TestRule20_AddConformanceErrors(t *testing.T) {
	cases := []struct {
		name     string
		typeName string
	}{
		{name: "missing type", typeName: "Missing"},
		{name: "type declared inside a function", typeName: "Inner"},
		{name: "a function is not a type", typeName: "Helper"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root := copyFixture(t, "shapes")
			expectFailure(t, root, astedit.AddConformance("shapes.go", tc.typeName, "io.Closer"), astedit.ErrNotFound)
		})
	}
}
