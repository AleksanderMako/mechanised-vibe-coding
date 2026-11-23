package main

import (
	"context"
	"flag"
	"fmt"

	"github.com/AleksanderMako/machanised-vibe-coding/symbols"
)

const symbolArgHelp = `SYMBOL is "import/path.Name" or "import/path.Type.Member".` + brokenPackageHelp

var symbolsGroup = group{
	name:    "symbols",
	summary: "type aware references, implementations and struct tags (package symbols)",
	actions: []action{
		{
			name:     "refs",
			synopsis: "-root DIR [-pkg P]... [-no-tests] [-strict] [-json] SYMBOL",
			help: `Lists every use of SYMBOL except its declaration, one per line as
FILE:LINE:COL<TAB>KIND<TAB>CONTEXT. With -json the []symbols.Reference.
` + symbolArgHelp,
			setup: setupSymbolsRefs,
		},
		{
			name:     "impls",
			synopsis: "-root DIR [-pkg P]... [-no-tests] [-strict] [-json] IFACE",
			help: `Lists the named types that satisfy interface IFACE, one per line as
import/path.Name. With -json the []symbols.Symbol. ` + symbolArgHelp,
			setup: setupSymbolsImpls,
		},
		{
			name:     "tags",
			synopsis: "-root DIR [-pkg P]... [-strict] [-json] TYPE",
			help: `Lists the fields of struct TYPE in declaration order, one per line as
FIELD<TAB>TYPE<TAB>TAG with the raw tag (empty when none). With -json the
[]symbols.FieldTag. ` + symbolArgHelp,
			setup: setupSymbolsTags,
		},
	},
}

// brokenPackageHelp explains -strict and the warnings in the help of every action that has it.
const brokenPackageHelp = `
A package that does not compile is tolerated where possible, with one
"mvc: warning:" line on stderr; -strict makes any such package an error.`

// addPackagesFlag registers the repeatable -pkg flag shared by the loading actions.
func addPackagesFlag(flags *flag.FlagSet) *listFlag {
	var packages listFlag
	flags.Var(&packages, "pkg", "package pattern `P` relative to -root (repeatable); none means ./...")
	return &packages
}

// parseOneSymbol checks there is exactly one positional argument and parses it.
func parseOneSymbol(args []string) (symbols.Symbol, error) {
	if len(args) != 1 {
		return symbols.Symbol{}, usageErrorf("want exactly one symbol argument, got %d", len(args))
	}
	return symbols.ParseSymbol(args[0])
}

func setupSymbolsRefs(flags *flag.FlagSet) func(context.Context, []string) error {
	root := addRootFlag(flags)
	packages := addPackagesFlag(flags)
	noTests := flags.Bool("no-tests", false, "leave _test.go files out")
	strict := addStrictFlag(flags)
	asJSON := addJSONFlag(flags)
	return func(ctx context.Context, args []string) error {
		symbol, err := parseOneSymbol(args)
		if err != nil {
			return err
		}
		absolute, err := absRoot(*root)
		if err != nil {
			return err
		}
		options := symbols.Options{Patterns: *packages, ExcludeTests: *noTests, Strict: *strict, Logf: warn}
		references, err := symbols.References(ctx, absolute, symbol, options)
		if err != nil {
			return err
		}
		if *asJSON {
			return printJSON(references)
		}
		for _, ref := range references {
			fmt.Printf("%s:%d:%d\t%s\t%s\n", ref.File, ref.Line, ref.Column, ref.Kind, ref.Context)
		}
		return nil
	}
}

func setupSymbolsImpls(flags *flag.FlagSet) func(context.Context, []string) error {
	root := addRootFlag(flags)
	packages := addPackagesFlag(flags)
	noTests := flags.Bool("no-tests", false, "leave _test.go files out")
	strict := addStrictFlag(flags)
	asJSON := addJSONFlag(flags)
	return func(ctx context.Context, args []string) error {
		iface, err := parseOneSymbol(args)
		if err != nil {
			return err
		}
		absolute, err := absRoot(*root)
		if err != nil {
			return err
		}
		options := symbols.Options{Patterns: *packages, ExcludeTests: *noTests, Strict: *strict, Logf: warn}
		implementations, err := symbols.Implementations(ctx, absolute, iface, options)
		if err != nil {
			return err
		}
		if *asJSON {
			return printJSON(implementations)
		}
		for _, implementation := range implementations {
			fmt.Println(implementation.String())
		}
		return nil
	}
}

func setupSymbolsTags(flags *flag.FlagSet) func(context.Context, []string) error {
	root := addRootFlag(flags)
	packages := addPackagesFlag(flags)
	strict := addStrictFlag(flags)
	asJSON := addJSONFlag(flags)
	return func(ctx context.Context, args []string) error {
		typ, err := parseOneSymbol(args)
		if err != nil {
			return err
		}
		absolute, err := absRoot(*root)
		if err != nil {
			return err
		}
		fields, err := symbols.StructTags(ctx, absolute, typ, symbols.Options{Patterns: *packages, Strict: *strict, Logf: warn})
		if err != nil {
			return err
		}
		if *asJSON {
			return printJSON(fields)
		}
		for _, field := range fields {
			fmt.Printf("%s\t%s\t%s\n", field.Field, field.Type, field.Tag)
		}
		return nil
	}
}
