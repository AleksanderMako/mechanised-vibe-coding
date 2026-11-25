package main_test

import (
	"encoding/json"
	"fmt"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/AleksanderMako/mechanised-vibe-coding/symbols"
)

// Symbols of the stage 10 fixture testdata/methodset.
const (
	msHandleAll      = "example.com/fixture/store.Handle.*"
	msHandleExec     = "example.com/fixture/store.Handle.Exec"
	msHandleQuery    = "example.com/fixture/store.Handle.Query"
	msHandleQueryRow = "example.com/fixture/store.Handle.QueryRow"
	msOpen           = "example.com/fixture/store.Open"
	msPlainAll       = "example.com/fixture/store.Plain.*"
	msMissing        = "example.com/fixture/store.NoSuch"
)

// fourColumnLine is rule 37's FILE:LINE:COL<TAB>KIND<TAB>SYMBOL<TAB>CONTEXT.
var fourColumnLine = regexp.MustCompile(`^(/[^\t]+):(\d+):(\d+)\t(call|conversion|type|embed|value)\t([^\t]+)\t([^\t]+)$`)

// copyMethodSet copies the stage 10 fixture module.
func copyMethodSet(t *testing.T) string {
	t.Helper()
	return copyFixture(t, "methodset")
}

// refsJSON runs symbols refs -json with extra flags and symbols and decodes the result.
func refsJSON(t *testing.T, module string, args ...string) []symbols.Reference {
	t.Helper()
	full := append([]string{"symbols", "refs", "-root", module, "-json"}, args...)
	result := runMVC(t, module, full...)
	expectExit(t, result, 0)
	var refs []symbols.Reference
	decodeJSON(t, result, &refs)
	return refs
}

// referenceSymbols returns the distinct Symbol.String() values of refs, sorted.
func referenceSymbols(refs []symbols.Reference) []string {
	var names []string
	for _, ref := range refs {
		if !slices.Contains(names, ref.Symbol.String()) {
			names = append(names, ref.Symbol.String())
		}
	}
	slices.Sort(names)
	return names
}

// expectUsageError checks a usage error: exit 2, nothing on stdout, something on stderr.
func expectUsageError(t *testing.T, result runResult) {
	t.Helper()
	expectExit(t, result, 2)
	if strings.TrimSpace(result.stdout) != "" {
		t.Errorf("usage error printed to stdout:\n%s", result.describe())
	}
	if strings.TrimSpace(result.stderr) == "" {
		t.Errorf("usage error printed nothing to stderr:\n%s", result.describe())
	}
}

// Rule 35: refs without any SYMBOL is a usage error with usage on stderr.
func TestRefsWithoutSymbolIsUsageError(t *testing.T) {
	module := copyMethodSet(t)
	for _, args := range [][]string{
		{"symbols", "refs", "-root", module},
		{"symbols", "refs", "-root", module, "-json", "-no-generated"},
	} {
		t.Run(strings.Join(args[2:], " "), func(t *testing.T) {
			result := runMVC(t, module, args...)
			expectUsageError(t, result)
			if !strings.Contains(result.stderr, "-root") {
				t.Errorf("stderr must hold the usage of refs, which lists -root:\n%s", result.describe())
			}
		})
	}
}

// Rule 35: a SYMBOL ParseSymbol rejects is a usage error naming that argument, also among valid ones.
func TestRefsUnparsableSymbolIsUsageErrorNamingIt(t *testing.T) {
	module := copyMethodSet(t)
	cases := []struct {
		name string
		args []string
		bad  string
	}{
		{"alone", []string{"nodotsymbol"}, "nodotsymbol"},
		{"after a valid one", []string{msHandleExec, "example.com/fixture/store..Exec"}, "example.com/fixture/store..Exec"},
		{"before a valid one", []string{"example.com/fixture/store.A.B.C", msHandleExec}, "example.com/fixture/store.A.B.C"},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			result := runMVC(t, module, append([]string{"symbols", "refs", "-root", module}, testCase.args...)...)
			expectUsageError(t, result)
			if !strings.Contains(result.stderr, testCase.bad) {
				t.Errorf("stderr must name the bad argument %q:\n%s", testCase.bad, result.describe())
			}
		})
	}
}

// Rule 35: several symbols in one call give the union of their references.
func TestRefsSeveralSymbols(t *testing.T) {
	module := copyMethodSet(t)
	both := refsJSON(t, module, msOpen, msHandleExec)
	if got, want := referenceSymbols(both), []string{msHandleExec, msOpen}; !slices.Equal(got, want) {
		t.Errorf("symbols of the references = %v, want %v", got, want)
	}
	open := refsJSON(t, module, msOpen)
	exec := refsJSON(t, module, msHandleExec)
	if len(both) != len(open)+len(exec) {
		t.Errorf("want %d references (%d of Open plus %d of Exec), got %d: %+v", len(open)+len(exec), len(open), len(exec), len(both), both)
	}
}

// Rule 35: Type.* lists the references of every method of the type, each with the method as Symbol.
func TestRefsMethodSetSymbol(t *testing.T) {
	module := copyMethodSet(t)
	refs := refsJSON(t, module, msHandleAll)
	want := []string{msHandleExec, msHandleQuery, msHandleQueryRow}
	if got := referenceSymbols(refs); !slices.Equal(got, want) {
		t.Errorf("symbols of Handle.* references = %v, want %v", got, want)
	}
	methods := refsJSON(t, module, msHandleExec, msHandleQuery, msHandleQueryRow)
	if !slices.Equal(refs, methods) {
		t.Errorf("Handle.* must equal its methods named one by one:\n got %+v\nwant %+v", refs, methods)
	}
}

// Rule 36: -no-generated leaves out references in generated files and keeps uses of generated declarations.
func TestRefsNoGenerated(t *testing.T) {
	module := copyMethodSet(t)
	all := refsJSON(t, module, msHandleAll)
	if !slices.ContainsFunc(all, func(ref symbols.Reference) bool { return ref.Generated }) {
		t.Fatalf("the fixture has references in generated files, none reported Generated: %+v", all)
	}
	filtered := refsJSON(t, module, "-no-generated", msHandleAll)
	var want []symbols.Reference
	for _, ref := range all {
		if !ref.Generated {
			want = append(want, ref)
		}
	}
	if !slices.Equal(filtered, want) {
		t.Errorf("-no-generated must drop exactly the generated references:\n got %+v\nwant %+v", filtered, want)
	}
	if !slices.ContainsFunc(filtered, func(ref symbols.Reference) bool { return ref.Symbol.String() == msHandleQueryRow }) {
		t.Errorf("QueryRow is declared in a generated file but used in a hand written one; that use must stay: %+v", filtered)
	}
}

// Rule 36: -no-tests keeps its meaning and combines with -no-generated.
func TestRefsNoTestsAndNoGenerated(t *testing.T) {
	module := copyMethodSet(t)
	cases := []struct {
		flags                  []string
		allowTest, allowGenned bool
	}{
		{[]string{"-no-tests"}, false, true},
		{[]string{"-no-tests", "-no-generated"}, false, false},
	}
	for _, testCase := range cases {
		t.Run(strings.Join(testCase.flags, " "), func(t *testing.T) {
			refs := refsJSON(t, module, append(testCase.flags, msHandleExec)...)
			if len(refs) == 0 {
				t.Fatal("Exec has hand written non test references, got none")
			}
			for _, ref := range refs {
				if ref.Test && !testCase.allowTest {
					t.Errorf("test reference with %v: %+v", testCase.flags, ref)
				}
				if ref.Generated && !testCase.allowGenned {
					t.Errorf("generated reference with %v: %+v", testCase.flags, ref)
				}
			}
		})
	}
}

// Rule 36: -strict keeps its meaning with several symbols; without it broken packages are warnings.
func TestRefsStrictWithSeveralSymbols(t *testing.T) {
	module := copyFixture(t, "tolerance")
	symbolArgs := []string{"example.com/fixture/lib.Hello", "example.com/fixture/lib.Config"}

	tolerant := runMVC(t, module, append([]string{"symbols", "refs", "-root", module}, symbolArgs...)...)
	expectExit(t, tolerant, 0)
	if !strings.Contains(tolerant.stderr, "mvc: warning: ") {
		t.Errorf("broken packages must be reported as warnings on stderr:\n%s", tolerant.describe())
	}

	expectOrdinaryError(t, runMVC(t, module, append([]string{"symbols", "refs", "-root", module, "-strict"}, symbolArgs...)...))
}

// Rule 37: each text line is FILE:LINE:COL, KIND, SYMBOL and CONTEXT, matching the -json result line by line.
func TestRefsTextHasSymbolColumn(t *testing.T) {
	module := copyMethodSet(t)
	result := runMVC(t, module, "symbols", "refs", "-root", module, msHandleAll, msOpen)
	expectExit(t, result, 0)
	lines := nonEmptyLines(result.stdout)
	refs := refsJSON(t, module, msHandleAll, msOpen)
	if len(lines) != len(refs) {
		t.Fatalf("want one text line per reference (%d), got %d:\n%s", len(refs), len(lines), result.describe())
	}
	for index, line := range lines {
		match := fourColumnLine.FindStringSubmatch(line)
		if match == nil {
			t.Errorf("line %q does not match %s", line, fourColumnLine)
			continue
		}
		ref := refs[index]
		wantPosition := fmt.Sprintf("%s:%d:%d", ref.File, ref.Line, ref.Column)
		if gotPosition := match[1] + ":" + match[2] + ":" + match[3]; gotPosition != wantPosition {
			t.Errorf("line %d: position %s, want %s", index, gotPosition, wantPosition)
		}
		if match[4] != string(ref.Kind) {
			t.Errorf("line %d: kind %s, want %s", index, match[4], ref.Kind)
		}
		if match[5] != ref.Symbol.String() {
			t.Errorf("line %d: symbol column %q, want %q", index, match[5], ref.Symbol.String())
		}
		if match[6] != ref.Context {
			t.Errorf("line %d: context %q, want %q", index, match[6], ref.Context)
		}
	}
}

// Rule 38: -json prints []symbols.Reference with Generated and Symbol present on every element.
func TestRefsJSONCarriesGeneratedAndSymbol(t *testing.T) {
	module := copyMethodSet(t)
	result := runMVC(t, module, "symbols", "refs", "-root", module, "-json", msHandleAll)
	expectExit(t, result, 0)
	var raw []map[string]json.RawMessage
	decodeJSON(t, result, &raw)
	if len(raw) == 0 {
		t.Fatalf("want references, got none:\n%s", result.describe())
	}
	for index, element := range raw {
		for _, key := range []string{"Generated", "Symbol"} {
			if _, ok := element[key]; !ok {
				t.Errorf("element %d has no %q field: %s", index, key, result.stdout)
			}
		}
	}
	var refs []symbols.Reference
	decodeJSON(t, result, &refs)
	for _, ref := range refs {
		if ref.Symbol.Package == "" || ref.Symbol.Name == "" || strings.Contains(ref.Symbol.Name, "*") {
			t.Errorf("Symbol must name the used method, got %+v", ref.Symbol)
		}
	}
}

// Rule 39: an unresolved symbol is an ordinary error with exactly one `mvc: ` line on stderr.
func TestRefsUnresolvedSymbolIsOrdinaryError(t *testing.T) {
	module := copyMethodSet(t)
	cases := []struct {
		name string
		args []string
	}{
		{"missing name", []string{msMissing}},
		{"missing among valid", []string{msHandleExec, msMissing}},
		{"method set of a function", []string{"example.com/fixture/store.Open.*"}},
		{"json", []string{"-json", msMissing}},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			result := runMVC(t, module, append([]string{"symbols", "refs", "-root", module}, testCase.args...)...)
			expectOrdinaryError(t, result)
			if lines := nonEmptyLines(result.stderr); len(lines) != 1 {
				t.Errorf("want exactly one stderr line, got %d:\n%s", len(lines), result.describe())
			}
		})
	}
}

// Rule 39: an empty result is success with no output.
func TestRefsEmptyResultPrintsNothing(t *testing.T) {
	module := copyMethodSet(t)
	result := runMVC(t, module, "symbols", "refs", "-root", module, msPlainAll)
	expectExit(t, result, 0)
	if result.stdout != "" {
		t.Errorf("empty result must print nothing:\n%s", result.describe())
	}
}

// Rule 40: the refs help names the Type.* form and -no-generated.
func TestRefsHelpNamesMethodSetAndNoGenerated(t *testing.T) {
	result := runMVC(t, realTempDir(t), "symbols", "refs", "-h")
	expectExit(t, result, 0)
	for _, want := range []string{".*", "-no-generated"} {
		if !strings.Contains(result.stdout, want) {
			t.Errorf("refs help does not mention %q:\n%s", want, result.describe())
		}
	}
}
