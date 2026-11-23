package symbols_test

import (
	"testing"

	"github.com/AleksanderMako/mechanised-vibe-coding/symbols"
)

func TestParseSymbolSplitsPackageAndName(t *testing.T) {
	cases := []struct {
		input string
		want  symbols.Symbol
	}{
		{"io.Reader", symbols.Symbol{Package: "io", Name: "Reader"}},
		{"strings.ToUpper", symbols.Symbol{Package: "strings", Name: "ToUpper"}},
		{"github.com/a/b/pkg.Type.Method", symbols.Symbol{Package: "github.com/a/b/pkg", Name: "Type.Method"}},
		{"example.com/fixture/shapes.Square", symbols.Symbol{Package: "example.com/fixture/shapes", Name: "Square"}},
		{"example.com/fixture/shapes.Square.Side", symbols.Symbol{Package: "example.com/fixture/shapes", Name: "Square.Side"}},
		{"net/http.Client.Do", symbols.Symbol{Package: "net/http", Name: "Client.Do"}},
	}
	for _, testCase := range cases {
		got, err := symbols.ParseSymbol(testCase.input)
		if err != nil {
			t.Errorf("ParseSymbol(%q): unexpected error %v", testCase.input, err)
			continue
		}
		if got != testCase.want {
			t.Errorf("ParseSymbol(%q): want %+v, got %+v", testCase.input, testCase.want, got)
		}
	}
}

func TestParseSymbolRejectsMalformedInput(t *testing.T) {
	inputs := []string{
		"",                        // empty
		"io",                      // no dot at all
		"github.com/a/b/pkg",      // no dot after the last slash
		"io.",                     // trailing dot
		"io.Reader.",              // trailing dot after a member
		"github.com/a/b/pkg.T.M.", // trailing dot after a member
		"io.A.B.C",                // three dots after the last slash
		"github.com/a/pkg.A.B.C",  // three dots after the last slash
	}
	for _, input := range inputs {
		if got, err := symbols.ParseSymbol(input); err == nil {
			t.Errorf("ParseSymbol(%q): want error, got %+v", input, got)
		}
	}
}

func TestStringIsTheInverseOfParseSymbol(t *testing.T) {
	inputs := []string{
		"io.Reader",
		"github.com/a/b/pkg.Type.Method",
		"example.com/fixture/shapes.Square.Side",
		"example.com/fixture/user.Holder",
	}
	for _, input := range inputs {
		parsed, err := symbols.ParseSymbol(input)
		if err != nil {
			t.Errorf("ParseSymbol(%q): unexpected error %v", input, err)
			continue
		}
		if rendered := parsed.String(); rendered != input {
			t.Errorf("ParseSymbol(%q).String(): got %q", input, rendered)
		}
	}
}

func TestStringRendersBuiltSymbols(t *testing.T) {
	cases := []struct {
		symbol symbols.Symbol
		want   string
	}{
		{symbols.Symbol{Package: "io", Name: "Reader"}, "io.Reader"},
		{symbols.Symbol{Package: "github.com/a/b/pkg", Name: "Type.Method"}, "github.com/a/b/pkg.Type.Method"},
	}
	for _, testCase := range cases {
		if got := testCase.symbol.String(); got != testCase.want {
			t.Errorf("%+v.String(): want %q, got %q", testCase.symbol, testCase.want, got)
		}
		parsed, err := symbols.ParseSymbol(testCase.symbol.String())
		if err != nil || parsed != testCase.symbol {
			t.Errorf("ParseSymbol(%q): want %+v, got %+v, %v", testCase.want, testCase.symbol, parsed, err)
		}
	}
}
