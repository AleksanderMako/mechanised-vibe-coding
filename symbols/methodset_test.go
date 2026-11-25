package symbols_test

import (
	"errors"
	"fmt"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/AleksanderMako/mechanised-vibe-coding/symbols"
)

// The stage 10 fixture symbols, all declared in package store of testdata/methodset.
var (
	handleAll      = sym("store", "Handle.*")
	handleExec     = sym("store", "Handle.Exec")
	handleQuery    = sym("store", "Handle.Query")
	handleQueryRow = sym("store", "Handle.QueryRow")
	querierAll     = sym("store", "Querier.*")
	querierQuery   = sym("store", "Querier.Query")
	baseAll        = sym("store", "Base.*")
	baseClose      = sym("store", "Base.Close")
	boxAll         = sym("store", "Box.*")
	boxGet         = sym("store", "Box.Get")
	storeOpen      = sym("store", "Open")
	ioReaderRead   = symbols.Symbol{Package: "io", Name: "Reader.Read"}
)

// wantSymRef is one expected reference with the stage 10 fields.
type wantSymRef struct {
	file      string // slash separated, relative to the fixture root
	marker    string
	symbol    symbols.Symbol
	kind      symbols.Kind
	generated bool
}

// The references of each Handle method in the methodset fixture, generated files and tests included.
func handleExecRefs() []wantSymRef {
	return []wantSymRef{
		{"store/gen.go", "storegen-exec", handleExec, symbols.KindCall, true},
		{"store/late.go", "late-exec", handleExec, symbols.KindCall, false},
		{"app/app.go", "app-exec", handleExec, symbols.KindCall, false},
		{"app/app.go", "app-exec-value", handleExec, symbols.KindValue, false},
		{"app/app_test.go", "test-exec", handleExec, symbols.KindCall, false},
		{"app/zz_gen_test.go", "gentest-exec", handleExec, symbols.KindCall, true},
	}
}

func handleQueryRefs() []wantSymRef {
	return []wantSymRef{
		{"store/gen.go", "storegen-query", handleQuery, symbols.KindCall, true},
		{"app/app.go", "app-query", handleQuery, symbols.KindCall, false},
		{"app/mock_gen.go", "appgen-query", handleQuery, symbols.KindCall, true},
	}
}

func handleQueryRowRefs() []wantSymRef {
	return []wantSymRef{
		{"app/app.go", "app-queryrow", handleQueryRow, symbols.KindCall, false},
	}
}

// handleAllRefs is Handle.*: Exec, Query and QueryRow, never the promoted Base.Close.
func handleAllRefs() []wantSymRef {
	return slices.Concat(handleExecRefs(), handleQueryRefs(), handleQueryRowRefs())
}

// keepRefs returns the wanted references for which keep holds.
func keepRefs(refs []wantSymRef, keep func(wantSymRef) bool) []wantSymRef {
	var kept []wantSymRef
	for _, ref := range refs {
		if keep(ref) {
			kept = append(kept, ref)
		}
	}
	return kept
}

func notGenerated(ref wantSymRef) bool { return !ref.generated }
func notTest(ref wantSymRef) bool      { return !strings.HasSuffix(ref.file, "_test.go") }

// symRefKey identifies a reference by file, line and symbol; a line may hold uses of two different symbols.
func symRefKey(file string, line int, symbol symbols.Symbol) string {
	return fmt.Sprintf("%s:%d %s", file, line, symbol)
}

// assertSymReferences checks got holds exactly the wanted references with every promised field right.
func assertSymReferences(t *testing.T, root string, got []symbols.Reference, want []wantSymRef) {
	t.Helper()
	assertSortedAndUnique(t, got)

	wantByKey := make(map[string]wantSymRef, len(want))
	for _, expected := range want {
		line := markerLine(t, root, expected.file, expected.marker)
		absolute := filepath.Join(root, filepath.FromSlash(expected.file))
		wantByKey[symRefKey(absolute, line, expected.symbol)] = expected
	}

	gotKeys := make(map[string]bool, len(got))
	for _, ref := range got {
		key := symRefKey(ref.File, ref.Line, ref.Symbol)
		gotKeys[key] = true
		expected, ok := wantByKey[key]
		if !ok {
			t.Errorf("unexpected reference %s (%s, generated %v): %q", key, ref.Kind, ref.Generated, ref.Context)
			continue
		}
		if strings.HasSuffix(ref.Symbol.Name, "*") {
			t.Errorf("reference at marker %s: Symbol must never be a method set symbol, got %s", expected.marker, ref.Symbol)
		}
		if ref.Kind != expected.kind {
			t.Errorf("reference at marker %s: want kind %q, got %q", expected.marker, expected.kind, ref.Kind)
		}
		if ref.Generated != expected.generated {
			t.Errorf("reference at marker %s: want Generated %v, got %v", expected.marker, expected.generated, ref.Generated)
		}
		wantPackage := fixtureModulePath + "/" + path.Dir(expected.file)
		if ref.Package != wantPackage {
			t.Errorf("reference at marker %s: want Package %q, got %q", expected.marker, wantPackage, ref.Package)
		}
		wantTest := strings.HasSuffix(expected.file, "_test.go")
		if ref.Test != wantTest {
			t.Errorf("reference at marker %s: want Test %v, got %v", expected.marker, wantTest, ref.Test)
		}
		if ref.Column <= 0 {
			t.Errorf("reference at marker %s: Column must be positive, got %d", expected.marker, ref.Column)
		}
		wantContext := strings.TrimSpace(fileLines(t, root, expected.file)[ref.Line-1])
		if ref.Context != wantContext {
			t.Errorf("reference at marker %s: want Context %q, got %q", expected.marker, wantContext, ref.Context)
		}
	}
	for key, expected := range wantByKey {
		if !gotKeys[key] {
			t.Errorf("missing reference at marker %s (%s)", expected.marker, key)
		}
	}
}

// lookup is References or ReferencesTo behind one signature, so rules that hold for both run as one table.
type lookup struct {
	name string
	call func(t *testing.T, root string, symbol symbols.Symbol, opts symbols.Options) ([]symbols.Reference, error)
}

var bothLookups = []lookup{
	{"References", func(t *testing.T, root string, symbol symbols.Symbol, opts symbols.Options) ([]symbols.Reference, error) {
		return symbols.References(t.Context(), root, symbol, opts)
	}},
	{"ReferencesTo", func(t *testing.T, root string, symbol symbols.Symbol, opts symbols.Options) ([]symbols.Reference, error) {
		return symbols.ReferencesTo(t.Context(), root, []symbols.Symbol{symbol}, opts)
	}},
}

// referencesToOf runs ReferencesTo on root and fails on error.
func referencesToOf(t *testing.T, root string, syms []symbols.Symbol, opts symbols.Options) []symbols.Reference {
	t.Helper()
	refs, err := symbols.ReferencesTo(t.Context(), root, syms, opts)
	if err != nil {
		t.Fatalf("ReferencesTo(%v): %v", syms, err)
	}
	return refs
}

// Rule 20: Generated is true exactly for files with the header before the package clause; a header after it does not count.
func TestMethodSetGeneratedFlag(t *testing.T) {
	root := copyFixture(t, "methodset")
	for _, lookup := range bothLookups {
		t.Run(lookup.name, func(t *testing.T) {
			refs, err := lookup.call(t, root, handleExec, symbols.Options{})
			if err != nil {
				t.Fatalf("%s(%s): %v", lookup.name, handleExec, err)
			}
			assertSymReferences(t, root, refs, handleExecRefs())
		})
	}
}

// Rule 21: ExcludeGenerated drops references in generated files but still finds uses of declarations made in them.
func TestMethodSetExcludeGenerated(t *testing.T) {
	root := copyFixture(t, "methodset")
	cases := []struct {
		name   string
		symbol symbols.Symbol
		want   []wantSymRef
	}{
		{"method used from a generated file", handleExec, keepRefs(handleExecRefs(), notGenerated)},
		{"method declared in a generated file", handleQueryRow, handleQueryRowRefs()},
		{"method set", handleAll, keepRefs(handleAllRefs(), notGenerated)},
	}
	for _, testCase := range cases {
		for _, lookup := range bothLookups {
			t.Run(testCase.name+"/"+lookup.name, func(t *testing.T) {
				refs, err := lookup.call(t, root, testCase.symbol, symbols.Options{ExcludeGenerated: true})
				if err != nil {
					t.Fatalf("%s(%s) with ExcludeGenerated: %v", lookup.name, testCase.symbol, err)
				}
				assertSymReferences(t, root, refs, testCase.want)
				for _, ref := range refs {
					if ref.Generated {
						t.Errorf("ExcludeGenerated: got a generated reference %+v", ref)
					}
				}
			})
		}
	}
}

// Rule 21: a generated file that does not compile is tolerated and logged exactly as without ExcludeGenerated.
func TestMethodSetBrokenGeneratedFileToleratedAsBefore(t *testing.T) {
	root := copyFixture(t, "methodset")
	writeFixtureFile(t, root, "stalegen/stalegen.go", "// Code generated by sqlc. DO NOT EDIT.\n\npackage stalegen\n\nvar Value int = missingGenerated\n")
	staleGen := fixtureModulePath + "/stalegen"

	for _, exclude := range []bool{false, true} {
		t.Run(fmt.Sprintf("ExcludeGenerated=%v", exclude), func(t *testing.T) {
			recorder := &logRecorder{}
			refs := referencesToOf(t, root, []symbols.Symbol{handleExec}, symbols.Options{ExcludeGenerated: exclude, Logf: recorder.logf})
			want := handleExecRefs()
			if exclude {
				want = keepRefs(want, notGenerated)
			}
			assertSymReferences(t, root, refs, want)
			lines := recorder.snapshot()
			assertOneLine(t, lines, staleGen, ignoringLinePrefix(staleGen), "missingGenerated")
			if len(lines) != 1 {
				t.Errorf("want exactly one log line, got %q", lines)
			}

			refs, err := symbols.ReferencesTo(t.Context(), root, []symbols.Symbol{handleExec}, symbols.Options{ExcludeGenerated: exclude, Strict: true})
			if err == nil {
				t.Error("Strict with a broken generated package: want error")
			} else if !strings.Contains(err.Error(), staleGen) {
				t.Errorf("Strict error must name %s, got %v", staleGen, err)
			}
			if refs != nil {
				t.Errorf("Strict error must come without a partial result, got %+v", refs)
			}
		})
	}
}

// Rule 22: a generated _test.go file is dropped by ExcludeTests, by ExcludeGenerated, and by both.
func TestMethodSetExcludeTestsAndGeneratedCombine(t *testing.T) {
	root := copyFixture(t, "methodset")
	cases := []struct {
		excludeTests, excludeGenerated bool
	}{
		{false, false}, {true, false}, {false, true}, {true, true},
	}
	for _, testCase := range cases {
		t.Run(fmt.Sprintf("ExcludeTests=%v,ExcludeGenerated=%v", testCase.excludeTests, testCase.excludeGenerated), func(t *testing.T) {
			want := handleExecRefs()
			if testCase.excludeTests {
				want = keepRefs(want, notTest)
			}
			if testCase.excludeGenerated {
				want = keepRefs(want, notGenerated)
			}
			opts := symbols.Options{ExcludeTests: testCase.excludeTests, ExcludeGenerated: testCase.excludeGenerated}
			for _, lookup := range bothLookups {
				refs, err := lookup.call(t, root, handleExec, opts)
				if err != nil {
					t.Fatalf("%s(%s): %v", lookup.name, handleExec, err)
				}
				assertSymReferences(t, root, refs, want)
			}
		})
	}
}

// Rule 23: Symbol names the declaring type: the embedded interface, the embedded struct, the generic origin, T for *T methods.
func TestReferenceSymbolIsTheDeclaringType(t *testing.T) {
	root := copyFixture(t, "methodset")
	refs := referencesToOf(t, root, []symbols.Symbol{querierAll, baseAll, boxAll, handleQuery}, symbols.Options{})
	assertSymReferences(t, root, refs, slices.Concat([]wantSymRef{
		{"app/app.go", "app-iface-query", querierQuery, symbols.KindCall, false},
		{"app/app.go", "app-iface-read", ioReaderRead, symbols.KindCall, false},
		{"app/app.go", "app-close", baseClose, symbols.KindCall, false},
		{"app/app.go", "app-box-get", boxGet, symbols.KindCall, false},
	}, handleQueryRefs()))
}

// Rule 24: References fills Symbol: equal to sym for a plain symbol, the used method for a method set symbol.
func TestReferencesFillsSymbol(t *testing.T) {
	root := copyFixture(t, "methodset")
	cases := []struct {
		symbol symbols.Symbol
		want   []wantSymRef
	}{
		{handleExec, handleExecRefs()},
		{storeOpen, []wantSymRef{
			{"app/app.go", "app-open", storeOpen, symbols.KindCall, false},
			{"app/app_test.go", "test-exec", storeOpen, symbols.KindCall, false},
		}},
		{handleAll, handleAllRefs()},
		{querierAll, []wantSymRef{
			{"app/app.go", "app-iface-query", querierQuery, symbols.KindCall, false},
			{"app/app.go", "app-iface-read", ioReaderRead, symbols.KindCall, false},
		}},
	}
	for _, testCase := range cases {
		t.Run(testCase.symbol.String(), func(t *testing.T) {
			refs, err := symbols.References(t.Context(), root, testCase.symbol, symbols.Options{})
			if err != nil {
				t.Fatalf("References(%s): %v", testCase.symbol, err)
			}
			assertSymReferences(t, root, refs, testCase.want)
		})
	}
}

// Rules 25 and 28: Type.* whose Type is missing, not a type name, or absent wraps ErrNotFound, with no result.
func TestMethodSetUnresolvableTypeIsNotFound(t *testing.T) {
	root := copyFixture(t, "methodset")
	cases := []struct {
		name   string
		symbol symbols.Symbol
	}{
		{"rule 25 missing type", sym("store", "Missing.*")},
		{"rule 25 function", sym("store", "Open.*")},
		{"rule 25 variable", sym("store", "Default.*")},
		{"rule 25 constant", sym("store", "Limit.*")},
		{"rule 28 asterisk alone", sym("store", "*")},
		{"rule 28 asterisk as type", sym("store", "*.Exec")},
	}
	for _, testCase := range cases {
		for _, lookup := range bothLookups {
			t.Run(testCase.name+"/"+lookup.name, func(t *testing.T) {
				refs, err := lookup.call(t, root, testCase.symbol, symbols.Options{})
				if !errors.Is(err, symbols.ErrNotFound) {
					t.Errorf("%s(%s): want ErrNotFound, got %v", lookup.name, testCase.symbol, err)
				}
				if refs != nil {
					t.Errorf("%s(%s): want no result with the error, got %+v", lookup.name, testCase.symbol, refs)
				}
			})
		}
	}
}

// mergeReferences is the union of several References results: sorted by Package, File, Line, Column, one per position.
func mergeReferences(results ...[]symbols.Reference) []symbols.Reference {
	merged := slices.Concat(results...)
	slices.SortStableFunc(merged, func(a, b symbols.Reference) int {
		switch {
		case referenceLess(a, b):
			return -1
		case referenceLess(b, a):
			return 1
		}
		return 0
	})
	return slices.CompactFunc(merged, func(a, b symbols.Reference) bool {
		return a.File == b.File && a.Line == b.Line && a.Column == b.Column
	})
}

// Rule 26: Type.* is the union of each method's references, for a concrete type and an interface with an embedded one.
func TestMethodSetIsUnionOfMethods(t *testing.T) {
	root := copyFixture(t, "methodset")
	cases := []struct {
		set     symbols.Symbol
		methods []symbols.Symbol
	}{
		{handleAll, []symbols.Symbol{handleExec, handleQuery, handleQueryRow}},
		{querierAll, []symbols.Symbol{querierQuery, sym("store", "Querier.Read")}},
	}
	for _, testCase := range cases {
		t.Run(testCase.set.String(), func(t *testing.T) {
			var perMethod [][]symbols.Reference
			for _, method := range testCase.methods {
				refs, err := symbols.References(t.Context(), root, method, symbols.Options{})
				if err != nil {
					t.Fatalf("References(%s): %v", method, err)
				}
				perMethod = append(perMethod, refs)
			}
			want := mergeReferences(perMethod...)
			got := referencesToOf(t, root, []symbols.Symbol{testCase.set}, symbols.Options{})
			if len(got) != len(want) {
				t.Fatalf("%s: want %d references as the union of its methods, got %d:\n got %+v\nwant %+v", testCase.set, len(want), len(got), got, want)
			}
			for index := range got {
				if got[index].File != want[index].File || got[index].Line != want[index].Line || got[index].Column != want[index].Column || got[index].Kind != want[index].Kind {
					t.Errorf("%s reference %d: got %+v, want %+v", testCase.set, index, got[index], want[index])
				}
			}
		})
	}
	// The promoted Base.Close is no method of Handle, so its call is not in Handle.*.
	assertSymReferences(t, root, referencesToOf(t, root, []symbols.Symbol{handleAll}, symbols.Options{}), handleAllRefs())
}

// Rule 27: a type with no methods, struct or interface, gives an empty list and no error.
func TestMethodSetOfTypeWithoutMethodsIsEmpty(t *testing.T) {
	root := copyFixture(t, "methodset")
	for _, symbol := range []symbols.Symbol{sym("store", "Plain.*"), sym("store", "Empty.*")} {
		for _, lookup := range bothLookups {
			t.Run(symbol.Name+"/"+lookup.name, func(t *testing.T) {
				refs, err := lookup.call(t, root, symbol, symbols.Options{})
				if err != nil {
					t.Fatalf("%s(%s): want no error, got %v", lookup.name, symbol, err)
				}
				if len(refs) != 0 {
					t.Errorf("%s(%s): want no references, got %+v", lookup.name, symbol, refs)
				}
			})
		}
	}
}

// Rule 29: an empty syms is an ordinary error and nothing is loaded, so no broken package is logged.
func TestReferencesToWithoutSymbolsIsOrdinaryError(t *testing.T) {
	root := copyFixture(t, "tolerance")
	for name, syms := range map[string][]symbols.Symbol{"nil": nil, "empty": {}} {
		t.Run(name, func(t *testing.T) {
			recorder := &logRecorder{}
			refs, err := symbols.ReferencesTo(t.Context(), root, syms, symbols.Options{Logf: recorder.logf})
			if err == nil {
				t.Fatal("ReferencesTo with no symbols: want error")
			}
			if errors.Is(err, symbols.ErrNotFound) {
				t.Errorf("ReferencesTo with no symbols: want an ordinary error, got ErrNotFound: %v", err)
			}
			if refs != nil {
				t.Errorf("want no result with the error, got %+v", refs)
			}
			if lines := recorder.snapshot(); len(lines) != 0 {
				t.Errorf("nothing may be loaded, so nothing logged; got %q", lines)
			}
		})
	}
}

// Rule 30: one unresolved symbol fails the whole call without a partial result, whatever its position.
func TestReferencesToUnresolvedSymbolFailsTheCall(t *testing.T) {
	root := copyFixture(t, "methodset")
	missing := sym("store", "NoSuch")
	for name, syms := range map[string][]symbols.Symbol{
		"missing last":       {handleExec, missing},
		"missing first":      {missing, handleExec},
		"missing method set": {handleExec, sym("store", "Open.*")},
	} {
		t.Run(name, func(t *testing.T) {
			refs, err := symbols.ReferencesTo(t.Context(), root, syms, symbols.Options{})
			if !errors.Is(err, symbols.ErrNotFound) {
				t.Errorf("ReferencesTo(%v): want ErrNotFound, got %v", syms, err)
			}
			if refs != nil {
				t.Errorf("ReferencesTo(%v): want no partial result, got %+v", syms, refs)
			}
		})
	}
}

// Rule 30: with several unresolved symbols the first in syms order is reported.
// A symbol in a broken declaring package is an ordinary error (stage 9 rule 2), a missing one wraps ErrNotFound.
func TestReferencesToReportsFirstUnresolvedSymbol(t *testing.T) {
	root := copyFixture(t, "tolerance")
	brokenDeclaring := symbols.Symbol{Package: toleranceStale, Name: "Value"}
	missing := symbols.Symbol{Package: toleranceLib, Name: "NoSuch"}

	refs, err := symbols.ReferencesTo(t.Context(), root, []symbols.Symbol{brokenDeclaring, missing}, symbols.Options{})
	if err == nil || errors.Is(err, symbols.ErrNotFound) {
		t.Errorf("broken declaring package first: want its ordinary error, got %v", err)
	}
	if refs != nil {
		t.Errorf("want no partial result, got %+v", refs)
	}

	refs, err = symbols.ReferencesTo(t.Context(), root, []symbols.Symbol{missing, brokenDeclaring}, symbols.Options{})
	if !errors.Is(err, symbols.ErrNotFound) {
		t.Errorf("missing symbol first: want ErrNotFound, got %v", err)
	}
	if refs != nil {
		t.Errorf("want no partial result, got %+v", refs)
	}
}

// Rule 31: each reference carries its own Symbol, and a position covered twice is listed once.
func TestReferencesToUnionAndDeduplication(t *testing.T) {
	root := copyFixture(t, "methodset")
	cases := []struct {
		name string
		syms []symbols.Symbol
		want []wantSymRef
	}{
		{"same method directly and through Type.*", []symbols.Symbol{handleExec, handleAll}, handleAllRefs()},
		{"same symbol twice", []symbols.Symbol{handleExec, handleExec}, handleExecRefs()},
		{"two symbols on one line", []symbols.Symbol{storeOpen, handleExec}, append(handleExecRefs(),
			wantSymRef{"app/app.go", "app-open", storeOpen, symbols.KindCall, false},
			wantSymRef{"app/app_test.go", "test-exec", storeOpen, symbols.KindCall, false},
		)},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			assertSymReferences(t, root, referencesToOf(t, root, testCase.syms, symbols.Options{}), testCase.want)
		})
	}
}

// Rule 32: the order of syms does not affect the output.
func TestReferencesToIgnoresSymbolOrder(t *testing.T) {
	root := copyFixture(t, "methodset")
	forward := referencesToOf(t, root, []symbols.Symbol{querierAll, handleAll, storeOpen, baseClose}, symbols.Options{})
	backward := referencesToOf(t, root, []symbols.Symbol{baseClose, storeOpen, handleAll, querierAll}, symbols.Options{})
	assertSortedAndUnique(t, forward)
	if !slices.Equal(forward, backward) {
		t.Errorf("output depends on the order of syms:\nforward  %+v\nbackward %+v", forward, backward)
	}
}

// Rule 33: stage 9 tolerance applies per symbol, with each broken package logged once per call.
func TestReferencesToToleratesBrokenPackages(t *testing.T) {
	root := copyFixture(t, "tolerance")
	hello := symbols.Symbol{Package: toleranceLib, Name: "Hello"}
	config := symbols.Symbol{Package: toleranceLib, Name: "Config"}

	recorder := &logRecorder{}
	refs := referencesToOf(t, root, []symbols.Symbol{hello, config}, symbols.Options{Logf: recorder.logf})
	assertSymReferences(t, root, refs, []wantSymRef{
		{"app/main.go", "app-config", config, symbols.KindType, false},
		{"app/main.go", "app-hello", hello, symbols.KindCall, false},
		{"brokenuser/brokenuser.go", "broken-config", config, symbols.KindType, false},
		{"brokenuser/brokenuser.go", "broken-hello", hello, symbols.KindCall, false},
	})
	lines := recorder.snapshot()
	assertOneLine(t, lines, toleranceStale, ignoringLinePrefix(toleranceStale), "missingThing")
	assertOneLine(t, lines, toleranceBrokenUser, bestEffortLinePrefix(toleranceBrokenUser), "undefinedName")
	assertLinesWellFormed(t, lines, toleranceLib, toleranceApp)
	if len(lines) != 2 {
		t.Errorf("want exactly two log lines, one per broken package, got %q", lines)
	}

	refs, err := symbols.ReferencesTo(t.Context(), root, []symbols.Symbol{hello, config}, symbols.Options{Strict: true})
	if err == nil {
		t.Error("Strict with broken packages: want error")
	} else if !strings.Contains(err.Error(), toleranceStale) && !strings.Contains(err.Error(), toleranceBrokenUser) {
		t.Errorf("Strict error must name a broken package, got %v", err)
	}
	if refs != nil {
		t.Errorf("Strict error must come without a partial result, got %+v", refs)
	}
}
