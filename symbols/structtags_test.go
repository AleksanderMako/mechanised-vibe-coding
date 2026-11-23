package symbols_test

import (
	"errors"
	"testing"

	"github.com/AleksanderMako/mechanised-vibe-coding/symbols"
)

// structTagsOf runs StructTags on a fresh copy of the main fixture and fails on error.
func structTagsOf(t *testing.T, typ symbols.Symbol) []symbols.FieldTag {
	t.Helper()
	root := copyFixture(t, "module")
	fields, err := symbols.StructTags(t.Context(), root, typ, symbols.Options{})
	if err != nil {
		t.Fatalf("StructTags(%s): %v", typ, err)
	}
	return fields
}

// assertFields compares field by field so a failure names the field that differs.
func assertFields(t *testing.T, got, want []symbols.FieldTag) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("want %d fields, got %d: %+v", len(want), len(got), got)
	}
	for index := range want {
		if got[index] != want[index] {
			t.Errorf("field %d:\nwant %+v\ngot  %+v", index, want[index], got[index])
		}
	}
}

func TestStructTagsCoverEveryTagForm(t *testing.T) {
	got := structTagsOf(t, sym("shapes", "Tagged"))
	assertFields(t, got, []symbols.FieldTag{
		{Field: "Name", Type: "string", Exported: true, Tag: `json:"name"`, JSON: "name"},
		{Field: "Secret", Type: "string", Exported: true, Tag: `json:"-"`, JSON: "-"},
		{Field: "Note", Type: "string", Exported: true, Tag: `json:",omitempty"`, JSON: "", OmitEmpty: true},
		{Field: "Count", Type: "int", Exported: true, Tag: `json:"count,omitempty"`, JSON: "count", OmitEmpty: true},
		{Field: "Plain", Type: "bool", Exported: true},
		{Field: "Column", Type: "string", Exported: true, Tag: `db:"column_name"`},
		{Field: "Square", Type: "Square", Embedded: true, Exported: true},
		{Field: "Circle", Type: "*Circle", Embedded: true, Exported: true},
		{Field: "Reader", Type: "io.Reader", Embedded: true, Exported: true},
		{Field: "Scores", Type: "map[string][]int", Exported: true, Tag: `json:"scores" db:"scores"`, JSON: "scores"},
		{Field: "hidden", Type: "int", Tag: `db:"hidden"`},
	})
}

func TestStructTagsOfTypeInAnotherPackage(t *testing.T) {
	got := structTagsOf(t, sym("user", "Holder"))
	assertFields(t, got, []symbols.FieldTag{
		{Field: "Square", Type: "shapes.Square", Embedded: true, Exported: true},
	})
}

func TestStructTagsKeepDeclarationOrder(t *testing.T) {
	got := structTagsOf(t, sym("shapes", "Point"))
	assertFields(t, got, []symbols.FieldTag{
		{Field: "X", Type: "int", Exported: true},
		{Field: "Y", Type: "int", Exported: true},
	})
}

func TestStructTagsRejectNonStructs(t *testing.T) {
	root := copyFixture(t, "module")
	for _, notStruct := range []symbols.Symbol{
		sym("shapes", "Unit"),
		sym("shapes", "Shape"),
		sym("shapes", "NewSquare"),
		sym("shapes", "MaxSide"),
	} {
		fields, err := symbols.StructTags(t.Context(), root, notStruct, symbols.Options{})
		if !errors.Is(err, symbols.ErrNotStruct) {
			t.Errorf("StructTags(%s): want ErrNotStruct, got %v", notStruct, err)
		}
		if fields != nil {
			t.Errorf("StructTags(%s): want no results with an error, got %+v", notStruct, fields)
		}
	}
}
