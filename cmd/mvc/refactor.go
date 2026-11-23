package main

import (
	"context"
	"flag"

	"github.com/AleksanderMako/machanised-vibe-coding/refactor"
	"github.com/AleksanderMako/machanised-vibe-coding/symbols"
)

const refactorHelpSuffix = `
Prints the written files one per line; with -json a JSON array of strings.
Every error, such as a name conflict or a failed build, goes to stderr with
exit 2. Symbols are "import/path.Name" or "import/path.Type.Member".` + brokenPackageHelp

var refactorGroup = group{
	name:    "refactor",
	summary: "rename, move and demethodize with every reference rewritten (package refactor)",
	actions: []action{
		{
			name:     "rename",
			synopsis: "-root DIR [-pkg P]... [-no-build] [-strict] [-json] SYMBOL NEWNAME",
			help:     "Renames SYMBOL to NEWNAME everywhere it is declared and used." + refactorHelpSuffix,
			setup:    setupRefactorRename,
		},
		{
			name:     "move",
			synopsis: "-root DIR [-pkg P]... [-no-build] [-strict] [-json] -to IMPORTPATH -to-file FILE SYMBOL...",
			help:     "Moves top level SYMBOLs of one package into FILE of package IMPORTPATH." + refactorHelpSuffix,
			setup:    setupRefactorMove,
		},
		{
			name:     "demethodize",
			synopsis: "-root DIR [-pkg P]... [-no-build] [-strict] [-json] TYPE.METHOD [NEWNAME]",
			help: `Turns method TYPE.METHOD (written import/path.Type.Method) into a package
level function named NEWNAME, the method name when omitted.` + refactorHelpSuffix,
			setup: setupRefactorDemethodize,
		},
	},
}

// addRefactorFlags registers the flags every refactor action shares.
func addRefactorFlags(flags *flag.FlagSet) (root *string, packages *listFlag, noBuild, strict, asJSON *bool) {
	root = addRootFlag(flags)
	packages = addPackagesFlag(flags)
	noBuild = flags.Bool("no-build", false, "skip the build after writing")
	strict = addStrictFlag(flags)
	asJSON = addJSONFlag(flags)
	return root, packages, noBuild, strict, asJSON
}

// printWritten prints the files a refactor wrote as lines or as a JSON array.
func printWritten(written []string, asJSON bool) error {
	if asJSON {
		return printJSON(written)
	}
	printLines(written)
	return nil
}

func setupRefactorRename(flags *flag.FlagSet) func(context.Context, []string) error {
	root, packages, noBuild, strict, asJSON := addRefactorFlags(flags)
	return func(ctx context.Context, args []string) error {
		if len(args) != 2 {
			return usageErrorf("want SYMBOL and NEWNAME, got %d arguments", len(args))
		}
		symbol, err := symbols.ParseSymbol(args[0])
		if err != nil {
			return err
		}
		absolute, err := absRoot(*root)
		if err != nil {
			return err
		}
		options := refactor.Options{Patterns: *packages, NoBuild: *noBuild, Strict: *strict, Logf: warn}
		written, err := refactor.Rename(ctx, absolute, symbol, args[1], options)
		if err != nil {
			return err
		}
		return printWritten(written, *asJSON)
	}
}

func setupRefactorMove(flags *flag.FlagSet) func(context.Context, []string) error {
	root, packages, noBuild, strict, asJSON := addRefactorFlags(flags)
	toImportPath := flags.String("to", "", "destination package `IMPORTPATH`")
	toFile := flags.String("to-file", "", "destination `FILE` name inside that package")
	return func(ctx context.Context, args []string) error {
		if err := requireFlags(flags, "to", "to-file"); err != nil {
			return err
		}
		if len(args) == 0 {
			return usageErrorf("want at least one SYMBOL argument")
		}
		var moving []symbols.Symbol
		for _, arg := range args {
			symbol, err := symbols.ParseSymbol(arg)
			if err != nil {
				return err
			}
			moving = append(moving, symbol)
		}
		absolute, err := absRoot(*root)
		if err != nil {
			return err
		}
		options := refactor.Options{Patterns: *packages, NoBuild: *noBuild, Strict: *strict, Logf: warn}
		written, err := refactor.Move(ctx, absolute, moving, *toImportPath, *toFile, options)
		if err != nil {
			return err
		}
		return printWritten(written, *asJSON)
	}
}

func setupRefactorDemethodize(flags *flag.FlagSet) func(context.Context, []string) error {
	root, packages, noBuild, strict, asJSON := addRefactorFlags(flags)
	return func(ctx context.Context, args []string) error {
		if len(args) != 1 && len(args) != 2 {
			return usageErrorf("want TYPE.METHOD and an optional NEWNAME, got %d arguments", len(args))
		}
		method, err := symbols.ParseSymbol(args[0])
		if err != nil {
			return err
		}
		// An empty name tells Demethodize to keep the method name.
		newName := ""
		if len(args) == 2 {
			newName = args[1]
		}
		absolute, err := absRoot(*root)
		if err != nil {
			return err
		}
		options := refactor.Options{Patterns: *packages, NoBuild: *noBuild, Strict: *strict, Logf: warn}
		written, err := refactor.Demethodize(ctx, absolute, method, newName, options)
		if err != nil {
			return err
		}
		return printWritten(written, *asJSON)
	}
}
