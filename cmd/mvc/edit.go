package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/AleksanderMako/machanised-vibe-coding/editset"
)

var editGroup = group{
	name:    "edit",
	summary: "transactional text edits across files (package editset)",
	actions: []action{
		{
			name:     "check",
			synopsis: "-root DIR [-json] EDITS.json",
			help: `Checks every edit of EDITS.json ("-" reads stdin) without writing anything.
Success prints nothing; with -json it prints {"Problems": []}.
Failing edits print one line each as INDEX<TAB>FILE<TAB>REASON<TAB>COUNT and
exit 1; with -json the *editset.CheckError is printed instead.`,
			setup: setupEditCheck,
		},
		{
			name:     "apply",
			synopsis: "-root DIR [-no-format] [-no-build] [-json] EDITS.json",
			help: `Applies every edit of EDITS.json ("-" reads stdin), formats and builds.
Success prints the written files one per line; with -json the editset.Result.
Failing edits print INDEX<TAB>FILE<TAB>REASON<TAB>COUNT per edit and exit 1;
a failed build prints the build output verbatim and exits 1. With -json the
*editset.CheckError or *editset.BuildError is printed instead, still exit 1.`,
			setup: setupEditApply,
		},
	},
}

func setupEditCheck(flags *flag.FlagSet) func(context.Context, []string) error {
	root := addRootFlag(flags)
	asJSON := addJSONFlag(flags)
	return func(ctx context.Context, args []string) error {
		edits, err := readEditsArg(args)
		if err != nil {
			return err
		}
		absolute, err := absRoot(*root)
		if err != nil {
			return err
		}
		checkErr := editset.Check(edits, editset.Options{Root: absolute})
		if checkErr == nil {
			if *asJSON {
				// Check has no result value; an empty CheckError keeps the JSON shape stable.
				return printJSON(&editset.CheckError{Problems: []editset.Problem{}})
			}
			return nil
		}
		return printEditFailure(checkErr, *asJSON)
	}
}

func setupEditApply(flags *flag.FlagSet) func(context.Context, []string) error {
	root := addRootFlag(flags)
	noFormat := flags.Bool("no-format", false, "skip goimports on touched .go files")
	noBuild := flags.Bool("no-build", false, "skip building the packages of touched .go files")
	asJSON := addJSONFlag(flags)
	return func(ctx context.Context, args []string) error {
		edits, err := readEditsArg(args)
		if err != nil {
			return err
		}
		absolute, err := absRoot(*root)
		if err != nil {
			return err
		}
		options := editset.Options{Root: absolute, NoFormat: *noFormat, NoBuild: *noBuild}
		result, applyErr := editset.Apply(ctx, edits, options)
		if applyErr != nil {
			return printEditFailure(applyErr, *asJSON)
		}
		if *asJSON {
			return printJSON(result)
		}
		printLines(result.Touched)
		return nil
	}
}

// readEditsArg reads the one EDITS.json argument, where "-" means stdin (rule 7).
func readEditsArg(args []string) ([]editset.Edit, error) {
	if len(args) != 1 {
		return nil, usageErrorf("want exactly one EDITS.json argument, got %d", len(args))
	}
	var input io.Reader = os.Stdin
	if args[0] != "-" {
		file, err := os.Open(args[0])
		if err != nil {
			return nil, err
		}
		defer file.Close()
		input = file
	}
	return editset.ParseJSON(input)
}

// printEditFailure prints a CheckError or BuildError as a negative result; other errors pass through.
func printEditFailure(err error, asJSON bool) error {
	var checkErr *editset.CheckError
	var buildErr *editset.BuildError
	switch {
	case errors.As(err, &checkErr):
		if asJSON {
			if jsonErr := printJSON(checkErr); jsonErr != nil {
				return jsonErr
			}
			return errNegative
		}
		for _, problem := range checkErr.Problems {
			fmt.Printf("%d\t%s\t%s\t%d\n", problem.Index, problem.File, problem.Reason, problem.Count)
		}
		return errNegative
	case errors.As(err, &buildErr):
		if asJSON {
			if jsonErr := printJSON(buildErr); jsonErr != nil {
				return jsonErr
			}
			return errNegative
		}
		printVerbatim(buildErr.Output)
		return errNegative
	default:
		return err
	}
}
