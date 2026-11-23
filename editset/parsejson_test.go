package editset_test

import (
	"slices"
	"strings"
	"testing"

	"github.com/AleksanderMako/mechanised-vibe-coding/editset"
)

func TestRule13_ParseJSONValidInput(t *testing.T) {
	cases := []struct {
		name  string
		input string
		want  []editset.Edit
	}{
		{
			name:  "all keys present",
			input: `[{"file": "a.go", "old": "x", "new": "y"}]`,
			want:  []editset.Edit{{File: "a.go", Old: "x", New: "y"}},
		},
		{
			name:  "new omitted means delete",
			input: `[{"file": "a.go", "old": "x"}]`,
			want:  []editset.Edit{{File: "a.go", Old: "x", New: ""}},
		},
		{
			name:  "new explicitly empty",
			input: `[{"file": "a.go", "old": "x", "new": ""}]`,
			want:  []editset.Edit{{File: "a.go", Old: "x", New: ""}},
		},
		{
			name:  "keys in any order and several elements keep their order",
			input: `[{"new": "2", "old": "1", "file": "/abs/b.txt"}, {"old": "line\n\ttab", "file": "c/d.go"}, {"file": "e", "old": "é", "new": "multi\nline"}]`,
			want: []editset.Edit{
				{File: "/abs/b.txt", Old: "1", New: "2"},
				{File: "c/d.go", Old: "line\n\ttab", New: ""},
				{File: "e", Old: "é", New: "multi\nline"},
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := editset.ParseJSON(strings.NewReader(tc.input))
			if err != nil {
				t.Fatalf("ParseJSON: %v", err)
			}
			if !slices.Equal(got, tc.want) {
				t.Errorf("want %+v, got %+v", tc.want, got)
			}
		})
	}
}

func TestRule13_ParseJSONEmptyArrayIsValid(t *testing.T) {
	got, err := editset.ParseJSON(strings.NewReader(`[]`))
	if err != nil {
		t.Fatalf("ParseJSON: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("want an empty slice, got %+v", got)
	}
}

func TestRule13_ParseJSONElementErrorsNameTheIndex(t *testing.T) {
	// The bad element is always at index 3 so the error must name "3".
	const goodElements = `{"file": "a", "old": "b"}, {"file": "a", "old": "c", "new": "d"}, {"file": "a", "old": "e"}, `
	cases := []struct {
		name       string
		badElement string
	}{
		{name: "unknown key", badElement: `{"file": "a", "old": "b", "extra": "x"}`},
		{name: "misspelled key", badElement: `{"file": "a", "Old ": "b"}`},
		{name: "missing file", badElement: `{"old": "b", "new": "c"}`},
		{name: "missing old", badElement: `{"file": "a", "new": "c"}`},
		{name: "file is a number", badElement: `{"file": 7, "old": "b"}`},
		{name: "old is a boolean", badElement: `{"file": "a", "old": true}`},
		{name: "new is a number", badElement: `{"file": "a", "old": "b", "new": 1}`},
		{name: "old is an array", badElement: `{"file": "a", "old": ["b"]}`},
		{name: "element is not an object", badElement: `"just a string"`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			input := "[" + goodElements + tc.badElement + "]"
			got, err := editset.ParseJSON(strings.NewReader(input))
			if err == nil {
				t.Fatalf("want error, got nil and %+v", got)
			}
			if !strings.Contains(err.Error(), "3") {
				t.Errorf("error does not name element index 3: %v", err)
			}
		})
	}
}

func TestRule13_ParseJSONTopLevelMustBeArray(t *testing.T) {
	cases := []struct {
		name  string
		input string
	}{
		{name: "object", input: `{"file": "a", "old": "b"}`},
		{name: "string", input: `"edits"`},
		{name: "number", input: `42`},
		{name: "null", input: `null`},
		{name: "empty input", input: ``},
		{name: "broken json", input: `[{"file": "a", "old": "b"`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := editset.ParseJSON(strings.NewReader(tc.input))
			if err == nil {
				t.Fatalf("want error, got nil and %+v", got)
			}
		})
	}
}
