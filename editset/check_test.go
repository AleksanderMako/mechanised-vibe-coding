package editset_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/AleksanderMako/machanised-vibe-coding/editset"
)

// wantProblem is the part of a Problem a test can predict; Count is only compared where the contract defines it.
type wantProblem struct {
	index  int
	file   string
	reason string
	count  int
}

// assertProblems checks err is a *CheckError whose Problems match want exactly, in order.
func assertProblems(t *testing.T, err error, want []wantProblem) {
	t.Helper()
	var checkErr *editset.CheckError
	if !errors.As(err, &checkErr) {
		t.Fatalf("want *editset.CheckError, got %T: %v", err, err)
	}
	if checkErr.Error() == "" {
		t.Errorf("CheckError.Error() is empty")
	}
	if len(checkErr.Problems) != len(want) {
		t.Fatalf("want %d problems, got %d: %+v", len(want), len(checkErr.Problems), checkErr.Problems)
	}
	for i, got := range checkErr.Problems {
		expected := want[i]
		if got.Index != expected.index || got.File != expected.file || got.Reason != expected.reason {
			t.Errorf("problem %d: want index=%d file=%s reason=%q, got index=%d file=%s reason=%q",
				i, expected.index, expected.file, expected.reason, got.Index, got.File, got.Reason)
		}
		// Count is meaningful only for these two reasons (contract, type Problem).
		if expected.reason == editset.ReasonNotFound || expected.reason == editset.ReasonNotUnique {
			if got.Count != expected.count {
				t.Errorf("problem %d: want count %d, got %d", i, expected.count, got.Count)
			}
		}
	}
}

func TestRule1_RootMissingOrInvalidIsOrdinaryError(t *testing.T) {
	root := copyFixture(t, "module")
	notADirectory := filepath.Join(root, "notes.txt")
	edits := []editset.Edit{{File: "notes.txt", Old: "beta", New: "BETA"}}
	cases := []struct {
		name string
		root string
	}{
		{name: "empty root", root: ""},
		{name: "root does not exist", root: filepath.Join(root, "no-such-dir")},
		{name: "root is a file", root: notADirectory},
	}
	for _, tc := range cases {
		t.Run(tc.name+"/Check", func(t *testing.T) {
			err := editset.Check(edits, editset.Options{Root: tc.root})
			if err == nil {
				t.Fatal("want error, got nil")
			}
			var checkErr *editset.CheckError
			if errors.As(err, &checkErr) {
				t.Fatalf("want an ordinary error, got *CheckError: %v", err)
			}
		})
		t.Run(tc.name+"/Apply", func(t *testing.T) {
			_, err := editset.Apply(context.Background(), edits, editset.Options{Root: tc.root, NoBuild: true})
			if err == nil {
				t.Fatal("want error, got nil")
			}
			var checkErr *editset.CheckError
			if errors.As(err, &checkErr) {
				t.Fatalf("want an ordinary error, got *CheckError: %v", err)
			}
		})
	}
	if got := readFile(t, notADirectory); got != "alpha\nbeta\nalpha\ngamma\nprice (USD) $5.00 [x]*\n" {
		t.Errorf("notes.txt changed although root was invalid: %q", got)
	}
}

func TestRule2_PathOutsideRoot(t *testing.T) {
	root := copyFixture(t, "module")
	outsideDir := t.TempDir()
	outsideFile := filepath.Join(outsideDir, "outside.txt")
	if err := os.WriteFile(outsideFile, []byte("beta\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	// A sibling of root, reached with a relative path that climbs out.
	siblingFile := filepath.Join(filepath.Dir(root), "sibling.txt")
	if err := os.WriteFile(siblingFile, []byte("beta\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name     string
		file     string
		wantFile string
	}{
		{name: "absolute path elsewhere", file: outsideFile, wantFile: outsideFile},
		{name: "relative path climbing out", file: "../sibling.txt", wantFile: siblingFile},
		{name: "relative path climbing out through a subdirectory", file: "calc/../../sibling.txt", wantFile: siblingFile},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			edits := []editset.Edit{{File: tc.file, Old: "beta", New: "BETA"}}
			err := editset.Check(edits, editset.Options{Root: root})
			assertProblems(t, err, []wantProblem{{index: 0, file: tc.wantFile, reason: editset.ReasonOutsideRoot}})
		})
	}
}

func TestRule2_RelativeAndAbsolutePathsInsideRootAreAccepted(t *testing.T) {
	root := copyFixture(t, "module")
	edits := []editset.Edit{
		{File: "notes.txt", Old: "beta", New: "BETA"},
		{File: filepath.Join(root, "calc", "calc.go"), Old: "a + b", New: "b + a"},
		{File: "greet/../root.go", Old: `"root"`, New: `"top"`},
	}
	if err := editset.Check(edits, editset.Options{Root: root}); err != nil {
		t.Fatalf("want nil, got %v", err)
	}
}

func TestRule3_MissingFile(t *testing.T) {
	root := copyFixture(t, "module")
	edits := []editset.Edit{
		{File: "does-not-exist.txt", Old: "beta", New: "BETA"},
		{File: filepath.Join(root, "calc", "missing.go"), Old: "x", New: "y"},
		// A directory cannot be read as a file.
		{File: "calc", Old: "x", New: "y"},
	}
	err := editset.Check(edits, editset.Options{Root: root})
	assertProblems(t, err, []wantProblem{
		{index: 0, file: filepath.Join(root, "does-not-exist.txt"), reason: editset.ReasonMissingFile},
		{index: 1, file: filepath.Join(root, "calc", "missing.go"), reason: editset.ReasonMissingFile},
		{index: 2, file: filepath.Join(root, "calc"), reason: editset.ReasonMissingFile},
	})
}

func TestRule4_EmptyOld(t *testing.T) {
	root := copyFixture(t, "module")
	edits := []editset.Edit{{File: "notes.txt", Old: "", New: "anything"}}
	err := editset.Check(edits, editset.Options{Root: root})
	assertProblems(t, err, []wantProblem{{index: 0, file: filepath.Join(root, "notes.txt"), reason: editset.ReasonEmptyOld}})
}

func TestRule4_NotFoundAndNotUniqueCounts(t *testing.T) {
	root := copyFixture(t, "module")
	notes := filepath.Join(root, "notes.txt")
	calc := filepath.Join(root, "calc", "calc.go")
	cases := []struct {
		name string
		edit editset.Edit
		want wantProblem
	}{
		{name: "text absent", edit: editset.Edit{File: "notes.txt", Old: "delta", New: "x"}, want: wantProblem{index: 0, file: notes, reason: editset.ReasonNotFound, count: 0}},
		{name: "text twice", edit: editset.Edit{File: "notes.txt", Old: "alpha", New: "x"}, want: wantProblem{index: 0, file: notes, reason: editset.ReasonNotUnique, count: 2}},
		{name: "go keyword twice", edit: editset.Edit{File: "calc/calc.go", Old: "return a", New: "x"}, want: wantProblem{index: 0, file: calc, reason: editset.ReasonNotUnique, count: 2}},
		{name: "type name four times", edit: editset.Edit{File: "calc/calc.go", Old: "int", New: "x"}, want: wantProblem{index: 0, file: calc, reason: editset.ReasonNotUnique, count: 4}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := editset.Check([]editset.Edit{tc.edit}, editset.Options{Root: root})
			assertProblems(t, err, []wantProblem{tc.want})
		})
	}
}

func TestRule4_EditsSeeEarlierEditsToTheSameFile(t *testing.T) {
	root := copyFixture(t, "module")
	notes := filepath.Join(root, "notes.txt")
	t.Run("an earlier edit makes a duplicate unique", func(t *testing.T) {
		edits := []editset.Edit{
			{File: "notes.txt", Old: "alpha\nbeta", New: "delta\nbeta"},
			{File: "notes.txt", Old: "alpha", New: "omega"},
		}
		if err := editset.Check(edits, editset.Options{Root: root}); err != nil {
			t.Fatalf("want nil, got %v", err)
		}
	})
	t.Run("an earlier edit adds a third occurrence", func(t *testing.T) {
		edits := []editset.Edit{
			{File: "notes.txt", Old: "beta", New: "alpha"},
			{File: "notes.txt", Old: "alpha", New: "x"},
		}
		err := editset.Check(edits, editset.Options{Root: root})
		assertProblems(t, err, []wantProblem{{index: 1, file: notes, reason: editset.ReasonNotUnique, count: 3}})
	})
	t.Run("an earlier edit consumes the text", func(t *testing.T) {
		edits := []editset.Edit{
			{File: "notes.txt", Old: "gamma", New: "epsilon"},
			{File: "notes.txt", Old: "gamma", New: "x"},
		}
		err := editset.Check(edits, editset.Options{Root: root})
		assertProblems(t, err, []wantProblem{{index: 1, file: notes, reason: editset.ReasonNotFound, count: 0}})
	})
	t.Run("an earlier edit creates the text", func(t *testing.T) {
		edits := []editset.Edit{
			{File: "notes.txt", Old: "gamma", New: "zeta"},
			{File: "notes.txt", Old: "zeta", New: "eta"},
		}
		if err := editset.Check(edits, editset.Options{Root: root}); err != nil {
			t.Fatalf("want nil, got %v", err)
		}
	})
	t.Run("the same path written two ways is one file", func(t *testing.T) {
		edits := []editset.Edit{
			{File: "notes.txt", Old: "gamma", New: "epsilon"},
			{File: notes, Old: "gamma", New: "x"},
		}
		err := editset.Check(edits, editset.Options{Root: root})
		assertProblems(t, err, []wantProblem{{index: 1, file: notes, reason: editset.ReasonNotFound, count: 0}})
	})
	t.Run("edits to another file do not interfere", func(t *testing.T) {
		edits := []editset.Edit{
			{File: "calc/calc.go", Old: "a + b", New: "gamma"},
			{File: "notes.txt", Old: "gamma", New: "x"},
		}
		if err := editset.Check(edits, editset.Options{Root: root}); err != nil {
			t.Fatalf("want nil, got %v", err)
		}
	})
}

func TestRule4_CheckDoesNotWrite(t *testing.T) {
	root := copyFixture(t, "module")
	snapshot := snapshotFiles(t, filepath.Join(root, "notes.txt"), filepath.Join(root, "calc", "calc.go"))
	edits := []editset.Edit{
		{File: "notes.txt", Old: "beta", New: "BETA"},
		{File: "calc/calc.go", Old: "a + b", New: "a + b + 1"},
	}
	if err := editset.Check(edits, editset.Options{Root: root}); err != nil {
		t.Fatalf("want nil, got %v", err)
	}
	assertUnchanged(t, snapshot)
}

func TestRule5_EveryProblemIsReportedInIndexOrder(t *testing.T) {
	root := copyFixture(t, "module")
	outside := filepath.Join(t.TempDir(), "outside.txt")
	if err := os.WriteFile(outside, []byte("beta\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	notes := filepath.Join(root, "notes.txt")
	calc := filepath.Join(root, "calc", "calc.go")
	edits := []editset.Edit{
		{File: "notes.txt", Old: "beta", New: "BETA"},         // 0 fine
		{File: "missing.txt", Old: "x", New: "y"},             // 1 missing
		{File: "notes.txt", Old: "", New: "y"},                // 2 empty old
		{File: "calc/calc.go", Old: "a * b", New: "y"},        // 3 not found
		{File: "notes.txt", Old: "alpha", New: "y"},           // 4 not unique
		{File: outside, Old: "beta", New: "y"},                // 5 outside root
		{File: "calc/calc.go", Old: "a + b", New: "b + a"},    // 6 fine
		{File: "notes.txt", Old: "BETA", New: "beta again"},   // 7 fine, sees edit 0
		{File: "calc/calc.go", Old: "return", New: "return"},  // 8 not unique
		{File: "notes.txt", Old: "price (usd)", New: "price"}, // 9 not found, case differs
	}
	err := editset.Check(edits, editset.Options{Root: root})
	want := []wantProblem{
		{index: 1, file: filepath.Join(root, "missing.txt"), reason: editset.ReasonMissingFile},
		{index: 2, file: notes, reason: editset.ReasonEmptyOld},
		{index: 3, file: calc, reason: editset.ReasonNotFound, count: 0},
		{index: 4, file: notes, reason: editset.ReasonNotUnique, count: 2},
		{index: 5, file: outside, reason: editset.ReasonOutsideRoot},
		{index: 8, file: calc, reason: editset.ReasonNotUnique, count: 4},
		{index: 9, file: notes, reason: editset.ReasonNotFound, count: 0},
	}
	assertProblems(t, err, want)

	t.Run("Apply reports the same problems and writes nothing", func(t *testing.T) {
		snapshot := snapshotFiles(t, notes, calc, outside)
		_, err := editset.Apply(context.Background(), edits, editset.Options{Root: root})
		assertProblems(t, err, want)
		assertUnchanged(t, snapshot)
	})
}

func TestRule6_ExactByteMatching(t *testing.T) {
	root := copyFixture(t, "module")
	notes := filepath.Join(root, "notes.txt")
	cases := []struct {
		name string
		old  string
	}{
		{name: "different case", old: "Alpha\nbeta"},
		{name: "trailing space", old: "beta \n"},
		{name: "collapsed newline", old: "beta gamma"},
		{name: "regular expression", old: "g.mma"},
		{name: "regular expression anchor", old: "^beta$"},
		{name: "tab instead of nothing", old: "\tbeta"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			edits := []editset.Edit{{File: "notes.txt", Old: tc.old, New: "x"}}
			err := editset.Check(edits, editset.Options{Root: root})
			assertProblems(t, err, []wantProblem{{index: 0, file: notes, reason: editset.ReasonNotFound, count: 0}})
		})
	}
	t.Run("regular expression characters match literally", func(t *testing.T) {
		edits := []editset.Edit{{File: "notes.txt", Old: "price (USD) $5.00 [x]*", New: "price (EUR) €4.00"}}
		result, err := editset.Apply(context.Background(), edits, editset.Options{Root: root})
		if err != nil {
			t.Fatalf("Apply: %v", err)
		}
		if got, want := readFile(t, notes), "alpha\nbeta\nalpha\ngamma\nprice (EUR) €4.00\n"; got != want {
			t.Errorf("want %q, got %q", want, got)
		}
		if len(result.Touched) != 1 || result.Touched[0] != notes {
			t.Errorf("want Touched [%s], got %v", notes, result.Touched)
		}
	})
}
