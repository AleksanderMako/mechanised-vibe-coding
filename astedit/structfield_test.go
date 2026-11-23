package astedit_test

import (
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/AleksanderMako/mechanised-vibe-coding/astedit"
)

func TestRules10to12_AddStructField(t *testing.T) {
	cases := []struct {
		name       string
		typeName   string
		fieldSrc   string
		after      string
		wantFields []string
		wantTag    map[string]string
		wantText   []string
	}{
		{
			name:       "append with a tag",
			typeName:   "Point",
			fieldSrc:   "Z int `json:\"z\"`",
			wantFields: []string{"X", "Y", "Z"},
			wantTag:    map[string]string{"Z": "`json:\"z\"`", "Y": "`json:\"y\"`"},
		},
		{
			name:       "insert after a named field",
			typeName:   "Point",
			fieldSrc:   "W string",
			after:      "X",
			wantFields: []string{"X", "W", "Y"},
		},
		{
			name:       "field with a trailing line comment",
			typeName:   "Point",
			fieldSrc:   "Label string // a label",
			wantFields: []string{"X", "Y", "Label"},
			wantText:   []string{"// a label"},
		},
		{
			name:       "embedded field",
			typeName:   "Point",
			fieldSrc:   "io.Reader",
			wantFields: []string{"X", "Y", "io.Reader"},
		},
		{
			name:       "into an empty struct",
			typeName:   "Empty",
			fieldSrc:   "Count int",
			wantFields: []string{"Count"},
		},
		{
			name:       "into a struct inside a grouped type declaration",
			typeName:   "Circle",
			fieldSrc:   "Center Point",
			wantFields: []string{"R", "Center"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root := copyFixture(t, "shapes")
			path := filepath.Join(root, "shapes.go")
			written := applyOK(t, root, astedit.AddStructField("shapes.go", tc.typeName, tc.fieldSrc, tc.after))
			if len(written) != 1 || written[0] != path {
				t.Errorf("want written [%s], got %v", path, written)
			}
			after := parseGo(t, path)
			structure := after.structType(tc.typeName)
			if structure == nil {
				t.Fatalf("struct %s disappeared", tc.typeName)
			}
			if got := after.fieldNames(structure); !slices.Equal(got, tc.wantFields) {
				t.Errorf("fields: want %v, got %v", tc.wantFields, got)
			}
			for name, wantTag := range tc.wantTag {
				fieldNode := field(structure, name)
				if fieldNode == nil || fieldNode.Tag == nil || fieldNode.Tag.Value != wantTag {
					t.Errorf("field %s: want tag %s", name, wantTag)
				}
			}
			for _, text := range tc.wantText {
				if !strings.Contains(after.source, text) {
					t.Errorf("output lacks %q:\n%s", text, after.source)
				}
			}
			// Comments on the existing Point fields must survive any edit.
			if tc.typeName == "Point" {
				if !strings.Contains(after.source, "// X is horizontal.") {
					t.Errorf("doc comment of X was lost:\n%s", after.source)
				}
				if !regexp.MustCompile(`X\s+int\s+// x line comment`).MatchString(after.source) {
					t.Errorf("line comment of X was lost or moved:\n%s", after.source)
				}
			}
		})
	}
}

func TestRules10to12_AddStructFieldErrors(t *testing.T) {
	cases := []struct {
		name     string
		typeName string
		fieldSrc string
		after    string
		want     error
	}{
		{name: "type missing", typeName: "Missing", fieldSrc: "A int", want: astedit.ErrNotFound},
		{name: "type is not a struct", typeName: "Name", fieldSrc: "A int", want: astedit.ErrNotFound},
		{name: "type is an interface", typeName: "Shape", fieldSrc: "A int", want: astedit.ErrNotFound},
		{name: "struct declared inside a function", typeName: "Inner", fieldSrc: "B int", want: astedit.ErrNotFound},
		{name: "after names a missing field", typeName: "Point", fieldSrc: "A int", after: "Missing", want: astedit.ErrNotFound},
		{name: "field already present", typeName: "Point", fieldSrc: "X int", want: astedit.ErrExists},
		{name: "field already present with another type", typeName: "Point", fieldSrc: "Y string", want: astedit.ErrExists},
		{name: "source does not parse", typeName: "Point", fieldSrc: "A int int", want: astedit.ErrBadSource},
		{name: "source is not a field", typeName: "Point", fieldSrc: "func (", want: astedit.ErrBadSource},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root := copyFixture(t, "shapes")
			expectFailure(t, root, astedit.AddStructField("shapes.go", tc.typeName, tc.fieldSrc, tc.after), tc.want)
		})
	}
}

func TestRule13_RemoveStructField(t *testing.T) {
	cases := []struct {
		name       string
		typeName   string
		fieldName  string
		wantFields []string
		gone       []string
		kept       []string
	}{
		{
			name:       "field with doc and line comment",
			typeName:   "Point",
			fieldName:  "X",
			wantFields: []string{"Y"},
			gone:       []string{"X is horizontal", "x line comment"},
			kept:       []string{"// Point is a location.", "`json:\"y\"`"},
		},
		{
			name:       "last field with a tag",
			typeName:   "Point",
			fieldName:  "Y",
			wantFields: []string{"X"},
			gone:       []string{"`json:\"y\"`"},
			kept:       []string{"// X is horizontal.", "// x line comment"},
		},
		{
			name:       "only field leaves struct{}",
			typeName:   "Single",
			fieldName:  "Only",
			wantFields: nil,
			gone:       []string{"Only is documented", "the only field"},
			kept:       []string{"// Single has one field."},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root := copyFixture(t, "shapes")
			path := filepath.Join(root, "shapes.go")
			applyOK(t, root, astedit.RemoveStructField(path, tc.typeName, tc.fieldName))
			after := parseGo(t, path)
			structure := after.structType(tc.typeName)
			if structure == nil {
				t.Fatalf("struct %s disappeared", tc.typeName)
			}
			if got := after.fieldNames(structure); !slices.Equal(got, tc.wantFields) {
				t.Errorf("fields: want %v, got %v", tc.wantFields, got)
			}
			for _, text := range tc.gone {
				if strings.Contains(after.source, text) {
					t.Errorf("output still has %q:\n%s", text, after.source)
				}
			}
			for _, text := range tc.kept {
				if !strings.Contains(after.source, text) {
					t.Errorf("output lost %q:\n%s", text, after.source)
				}
			}
			if len(tc.wantFields) == 0 && !strings.Contains(after.source, "type "+tc.typeName+" struct{}") {
				t.Errorf("empty struct must be written as struct{}:\n%s", after.source)
			}
		})
	}
}

func TestRule13_RemoveStructFieldErrors(t *testing.T) {
	cases := []struct {
		name      string
		typeName  string
		fieldName string
	}{
		{name: "field missing", typeName: "Point", fieldName: "Z"},
		{name: "type missing", typeName: "Missing", fieldName: "X"},
		{name: "type is not a struct", typeName: "Name", fieldName: "X"},
		{name: "struct declared inside a function", typeName: "Inner", fieldName: "A"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root := copyFixture(t, "shapes")
			expectFailure(t, root, astedit.RemoveStructField("shapes.go", tc.typeName, tc.fieldName), astedit.ErrNotFound)
		})
	}
}
