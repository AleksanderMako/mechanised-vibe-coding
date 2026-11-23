package mergeinfo_test

import (
	"slices"
	"testing"

	"github.com/AleksanderMako/machanised-vibe-coding/mergeinfo"
)

// parseSorted runs ParseHunks, fails on error, and orders hunks by Start so tests do not depend on report order.
func parseSorted(t *testing.T, content string) []markerFields {
	t.Helper()
	hunks, err := mergeinfo.ParseHunks(content)
	if err != nil {
		t.Fatalf("ParseHunks: %v", err)
	}
	slices.SortFunc(hunks, func(a, b mergeinfo.Hunk) int { return a.Start - b.Start })
	fields := make([]markerFields, 0, len(hunks))
	for _, hunk := range hunks {
		fields = append(fields, markersOf(hunk))
	}
	return fields
}

// assertMarkers compares parsed hunks to want, field by field.
func assertMarkers(t *testing.T, got, want []markerFields) {
	t.Helper()
	if !slices.Equal(got, want) {
		t.Errorf("hunks differ\nwant %+v\ngot  %+v", want, got)
	}
}

func TestRule4_6_7_PlainMarkers(t *testing.T) {
	t.Parallel()
	content := "package app\n" +
		"<<<<<<< HEAD\n" +
		"ours one\n" +
		"ours two\n" +
		"=======\n" +
		"theirs one\n" +
		">>>>>>> theirs\n" +
		"after\n"
	assertMarkers(t, parseSorted(t, content), []markerFields{{
		Start:  2,
		End:    7,
		Ours:   "ours one\nours two\n",
		Theirs: "theirs one\n",
	}})
}

func TestRule4_6_Diff3MarkersFillBase(t *testing.T) {
	t.Parallel()
	content := "<<<<<<< HEAD\n" +
		"ours\n" +
		"||||||| 544b98b\n" +
		"base one\n" +
		"base two\n" +
		"=======\n" +
		"theirs\n" +
		">>>>>>> theirs\n"
	assertMarkers(t, parseSorted(t, content), []markerFields{{
		Start:  1,
		End:    8,
		Ours:   "ours\n",
		Base:   "base one\nbase two\n",
		Theirs: "theirs\n",
	}})
}

func TestRule4_CRLFInputIsAcceptedAndSidesKeepCRLF(t *testing.T) {
	t.Parallel()
	content := "before\r\n" +
		"<<<<<<< HEAD\r\n" +
		"ours\r\n" +
		"=======\r\n" +
		"theirs\r\n" +
		">>>>>>> theirs\r\n" +
		"after\r\n"
	assertMarkers(t, parseSorted(t, content), []markerFields{{
		Start:  2,
		End:    6,
		Ours:   "ours\r\n",
		Theirs: "theirs\r\n",
	}})
}

func TestRule4_CRLFDiff3KeepsCRLFInBase(t *testing.T) {
	t.Parallel()
	content := "<<<<<<< HEAD\r\n" +
		"ours\r\n" +
		"||||||| base\r\n" +
		"base\r\n" +
		"=======\r\n" +
		"theirs\r\n" +
		">>>>>>> theirs\r\n"
	assertMarkers(t, parseSorted(t, content), []markerFields{{
		Start:  1,
		End:    7,
		Ours:   "ours\r\n",
		Base:   "base\r\n",
		Theirs: "theirs\r\n",
	}})
}

func TestRule6_EmptyOurSideIsDeleted(t *testing.T) {
	t.Parallel()
	content := "package app\n" +
		"\n" +
		"<<<<<<< HEAD\n" +
		"=======\n" +
		"func Greet() {}\n" +
		"\n" +
		">>>>>>> theirs\n" +
		"// tail\n"
	assertMarkers(t, parseSorted(t, content), []markerFields{{
		Start:       3,
		End:         7,
		Ours:        "",
		Theirs:      "func Greet() {}\n\n",
		OursDeleted: true,
	}})
}

func TestRule6_EmptyTheirSideIsDeleted(t *testing.T) {
	t.Parallel()
	content := "<<<<<<< HEAD\n" +
		"kept by us\n" +
		"=======\n" +
		">>>>>>> theirs\n"
	assertMarkers(t, parseSorted(t, content), []markerFields{{
		Start:         1,
		End:           4,
		Ours:          "kept by us\n",
		Theirs:        "",
		TheirsDeleted: true,
	}})
}

func TestRule6_EmptyBaseInDiff3IsEmptyString(t *testing.T) {
	t.Parallel()
	content := "<<<<<<< HEAD\n" +
		"ours\n" +
		"||||||| base\n" +
		"=======\n" +
		"theirs\n" +
		">>>>>>> theirs\n"
	assertMarkers(t, parseSorted(t, content), []markerFields{{
		Start:  1,
		End:    6,
		Ours:   "ours\n",
		Theirs: "theirs\n",
	}})
}

func TestRule4_7_TwoHunksInOneFile(t *testing.T) {
	t.Parallel()
	content := "line one\n" +
		"<<<<<<< HEAD\n" +
		"first ours\n" +
		"=======\n" +
		"first theirs\n" +
		">>>>>>> theirs\n" +
		"middle\n" +
		"middle again\n" +
		"<<<<<<< HEAD\n" +
		"second ours\n" +
		"=======\n" +
		"second theirs a\n" +
		"second theirs b\n" +
		">>>>>>> theirs\n" +
		"last\n"
	assertMarkers(t, parseSorted(t, content), []markerFields{
		{Start: 2, End: 6, Ours: "first ours\n", Theirs: "first theirs\n"},
		{Start: 9, End: 14, Ours: "second ours\n", Theirs: "second theirs a\nsecond theirs b\n"},
	})
}

func TestRule5_TextOutsideHunksIsIgnored(t *testing.T) {
	t.Parallel()
	// Marker look-alikes that do not start the line, and ordinary text, are not markers.
	content := "x := \"<<<<<<< HEAD\"\n" +
		"  =======\n" +
		"y := \">>>>>>> theirs\"\n" +
		"// |||||||\n"
	assertMarkers(t, parseSorted(t, content), []markerFields{})
}

func TestRule5_NoMarkersGivesNoHunks(t *testing.T) {
	t.Parallel()
	assertMarkers(t, parseSorted(t, "package app\n\nfunc main() {}\n"), []markerFields{})
	assertMarkers(t, parseSorted(t, ""), []markerFields{})
}

func TestRule4_LastMarkerWithoutTrailingNewline(t *testing.T) {
	t.Parallel()
	content := "<<<<<<< HEAD\n" +
		"ours\n" +
		"=======\n" +
		"theirs\n" +
		">>>>>>> theirs"
	assertMarkers(t, parseSorted(t, content), []markerFields{{
		Start:  1,
		End:    5,
		Ours:   "ours\n",
		Theirs: "theirs\n",
	}})
}

func TestRule5_MalformedMarkersAreErrorsNamingTheLine(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name    string
		content string
		badLine int
	}{
		{
			name: "unmatched start marker",
			content: "one\n" +
				"two\n" +
				"three\n" +
				"<<<<<<< HEAD\n" +
				"ours\n" +
				"=======\n" +
				"theirs\n" +
				"eight\n" +
				"nine\n",
			badLine: 4,
		},
		{
			name: "separator before start marker",
			content: "one\n" +
				"two\n" +
				"=======\n" +
				"four\n" +
				"<<<<<<< HEAD\n" +
				"ours\n" +
				"=======\n" +
				"theirs\n" +
				">>>>>>> theirs\n",
			badLine: 3,
		},
		{
			name: "end marker without separator",
			content: "one\n" +
				"<<<<<<< HEAD\n" +
				"ours\n" +
				"more ours\n" +
				">>>>>>> theirs\n" +
				"six\n" +
				"seven\n",
			badLine: 5,
		},
		{
			name: "end marker without start marker",
			content: "one\n" +
				"two\n" +
				"three\n" +
				"four\n" +
				"five\n" +
				">>>>>>> theirs\n" +
				"seven\n",
			badLine: 6,
		},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			_, err := mergeinfo.ParseHunks(testCase.content)
			if err == nil {
				t.Fatal("want an error, got nil")
			}
			if !containsLineNumber(err.Error(), testCase.badLine) {
				t.Errorf("error should name line %d: %v", testCase.badLine, err)
			}
		})
	}
}
