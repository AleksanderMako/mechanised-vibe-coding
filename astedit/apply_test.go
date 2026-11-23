package astedit_test

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/AleksanderMako/machanised-vibe-coding/astedit"
)

func TestOpFileEchoesInputAndDescribeIsOneLine(t *testing.T) {
	for _, file := range []string{"shapes.go", "/some/absolute/dir/shapes.go", "sub/../shapes.go"} {
		ops := map[string]astedit.Op{
			"AddTableCase":      astedit.AddTableCase(file, "TestAssigned", `{name: "x"}`),
			"AddStructField":    astedit.AddStructField(file, "Point", "Z int", ""),
			"RemoveStructField": astedit.RemoveStructField(file, "Point", "X"),
			"SetDoc":            astedit.SetDoc(file, "Point", "Point is a point."),
			"SetConst":          astedit.SetConst(file, "Version", `"2"`, ""),
			"AddConformance":    astedit.AddConformance(file, "Point", "io.Closer"),
		}
		for name, op := range ops {
			t.Run(name+" "+file, func(t *testing.T) {
				if got := op.File(); got != file {
					t.Errorf("File(): want %q, got %q", file, got)
				}
				description := op.Describe()
				if strings.TrimSpace(description) == "" {
					t.Errorf("Describe() is empty")
				}
				if strings.Contains(description, "\n") {
					t.Errorf("Describe() is not one line: %q", description)
				}
			})
		}
	}
}

func TestRule1_CommentsOutsideTheEditSurvive(t *testing.T) {
	root := copyFixture(t, "shapes")
	path := filepath.Join(root, "shapes.go")
	applyOK(t, root, astedit.AddStructField("shapes.go", "Square", "Color string", ""))
	source := readFile(t, path)
	for _, comment := range []string{
		"// Package shapes is a fixture for the astedit tests.",
		"// Version is the fixture version.",
		"// Colors used by shapes.",
		"// Red is the first color.",
		"// the last color",
		"// X is horizontal.",
		"// x line comment",
		"// Circle is round.",
		"// Triangle has three sides.",
		"// Area returns the area.",
		"// multiply the side by itself",
		"// Close closes the square.",
	} {
		if !strings.Contains(source, comment) {
			t.Errorf("comment %q was lost:\n%s", comment, source)
		}
	}
}

func TestRule2_UnparsableFileIsAnErrorNamingTheFile(t *testing.T) {
	root := copyFixture(t, "shapes")
	broken := filepath.Join(root, "broken.go")
	writeFile(t, broken, "package shapes\n\ntype Point struct {\n")
	pinned := pinFiles(t, broken)
	written, err := astedit.Apply(context.Background(), root, astedit.AddStructField("broken.go", "Point", "X int", ""))
	if err == nil {
		t.Fatal("want error, got nil")
	}
	if !strings.Contains(err.Error(), "broken.go") {
		t.Errorf("error does not name the file: %v", err)
	}
	if len(written) != 0 {
		t.Errorf("want nothing written, got %v", written)
	}
	assertNotWritten(t, pinned)
}

func TestApplyMissingFileIsAnError(t *testing.T) {
	root := copyFixture(t, "shapes")
	written, err := astedit.Apply(context.Background(), root, astedit.SetDoc("missing.go", "Point", "doc"))
	if err == nil {
		t.Fatal("want error, got nil")
	}
	if len(written) != 0 {
		t.Errorf("want nothing written, got %v", written)
	}
	if _, statErr := os.Stat(filepath.Join(root, "missing.go")); !os.IsNotExist(statErr) {
		t.Errorf("missing.go must not be created")
	}
}

func TestRule4_WrittenFileEndsWithOneNewline(t *testing.T) {
	root := copyFixture(t, "shapes")
	path := filepath.Join(root, "trailing.go")
	// Extra trailing blank lines and a last declaration that is edited.
	writeFile(t, path, "package shapes\n\ntype Trailing struct {\n\tA int\n}\n\n\n\n")
	applyOK(t, root, astedit.AddStructField("trailing.go", "Trailing", "B int", ""))
	source := readFile(t, path)
	if !strings.HasSuffix(source, "}\n") {
		t.Errorf("want the file to end with exactly one newline, got %q", source)
	}

	t.Run("conformance appended after the last declaration", func(t *testing.T) {
		applyOK(t, root, astedit.AddConformance("trailing.go", "Trailing", "any"))
		if got := readFile(t, path); !strings.HasSuffix(got, "(*Trailing)(nil)\n") {
			t.Errorf("want the file to end with the new line and one newline, got %q", got)
		}
	})
}

func TestRule1_UnformattedFileIsWrittenFormatted(t *testing.T) {
	root := copyFixture(t, "shapes")
	path := filepath.Join(root, "messy.go")
	writeFile(t, path, "package shapes\ntype   Messy struct{\nA    int\n}\nfunc  messy( ) {  }\n")
	// applyOK checks the written file equals its gofmt output.
	applyOK(t, root, astedit.AddStructField("messy.go", "Messy", "B int", ""))
	after := parseGo(t, path)
	if got := after.fieldNames(after.structType("Messy")); !slices.Equal(got, []string{"A", "B"}) {
		t.Errorf("fields: want [A B], got %v", got)
	}
}

func TestRule23_FailingOpLeavesEveryFileUntouched(t *testing.T) {
	cases := []struct {
		name    string
		ops     func() []astedit.Op
		failing int // index of the op whose error is expected
		want    error
	}{
		{
			name: "failure in the last op on the first file",
			ops: func() []astedit.Op {
				return []astedit.Op{
					astedit.AddStructField("other.go", "Other", "B int", ""),
					astedit.AddStructField("shapes.go", "Point", "Z int", ""),
					astedit.RemoveStructField("other.go", "Other", "Missing"),
				}
			},
			failing: 2,
			want:    astedit.ErrNotFound,
		},
		{
			name: "failure in the second file after the first file's ops succeeded",
			ops: func() []astedit.Op {
				return []astedit.Op{
					astedit.SetDoc("other.go", "Other", "Other is changed."),
					astedit.AddConformance("shapes.go", "Point", "io.Reader"),
					astedit.SetConst("shapes.go", "Width", "1", ""),
				}
			},
			failing: 2,
			want:    astedit.ErrAmbiguous,
		},
		{
			name: "failure in the first op",
			ops: func() []astedit.Op {
				return []astedit.Op{
					astedit.AddStructField("shapes.go", "Point", "X int", ""),
					astedit.AddStructField("other.go", "Other", "B int", ""),
				}
			},
			failing: 0,
			want:    astedit.ErrExists,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root := copyFixture(t, "shapes")
			pinned := pinFiles(t, filepath.Join(root, "shapes.go"), filepath.Join(root, "other.go"))
			ops := tc.ops()
			written, err := astedit.Apply(context.Background(), root, ops...)
			assertOpError(t, err, tc.want, ops[tc.failing])
			if len(written) != 0 {
				t.Errorf("want nothing written, got %v", written)
			}
			assertNotWritten(t, pinned)
		})
	}
}

func TestRule23_OpsOnOneFileApplyInOrder(t *testing.T) {
	root := copyFixture(t, "shapes")
	path := filepath.Join(root, "shapes.go")
	applyOK(t, root,
		astedit.AddStructField("shapes.go", "Point", "Z int", ""),
		astedit.AddStructField("shapes.go", "Point", "W int", "Z"),
		astedit.RemoveStructField("shapes.go", "Point", "X"),
		astedit.AddStructField(path, "Point", "X float64", "Y"),
	)
	after := parseGo(t, path)
	if got, want := after.fieldNames(after.structType("Point")), []string{"Y", "X", "Z", "W"}; !slices.Equal(got, want) {
		t.Errorf("fields: want %v, got %v", want, got)
	}
}

func TestRule24_FileWithOnlyNoOpsIsNotWritten(t *testing.T) {
	root := copyFixture(t, "shapes")
	shapes := filepath.Join(root, "shapes.go")
	other := filepath.Join(root, "other.go")
	pinned := pinFiles(t, shapes)
	written := applyOK(t, root,
		astedit.AddConformance("shapes.go", "Square", "io.Closer"),
		astedit.AddStructField("other.go", "Other", "B int", ""),
	)
	if want := []string{other}; !slices.Equal(written, want) {
		t.Errorf("written: want %v, got %v", want, written)
	}
	assertNotWritten(t, pinned)
}

func TestRule25_WrittenFilesInFirstOpOrder(t *testing.T) {
	root := copyFixture(t, "shapes")
	shapes := filepath.Join(root, "shapes.go")
	other := filepath.Join(root, "other.go")
	untouched := filepath.Join(root, "untouched.go")
	// Not targeted by any op, and deliberately unformatted: it must stay byte for byte.
	writeFile(t, untouched, "package shapes\nfunc  untouched( ) {  }\n")
	pinned := pinFiles(t, untouched)

	written := applyOK(t, root,
		astedit.AddStructField("other.go", "Other", "B int", ""),
		astedit.SetDoc(shapes, "Helper", "Helper helps."),
		// The same file named by its absolute path joins the first group.
		astedit.AddStructField(other, "Other", "C int", ""),
	)
	if want := []string{other, shapes}; !slices.Equal(written, want) {
		t.Errorf("written: want %v, got %v", want, written)
	}
	after := parseGo(t, other)
	if got, want := after.fieldNames(after.structType("Other")), []string{"A", "B", "C"}; !slices.Equal(got, want) {
		t.Errorf("Other fields: want %v, got %v", want, got)
	}
	assertNotWritten(t, pinned)
}

func TestApplyAcceptsPathsRelativeToRoot(t *testing.T) {
	root := copyFixture(t, "tables")
	path := filepath.Join(root, "tables_test.go")
	written := applyOK(t, root, astedit.AddTableCase("./tables_test.go", "TestMap", `"second": {want: 2}`))
	if want := []string{path}; !slices.Equal(written, want) {
		t.Errorf("written: want %v, got %v", want, written)
	}
}
