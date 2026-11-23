package main

import (
	"context"
	"flag"
	"fmt"

	"github.com/AleksanderMako/machanised-vibe-coding/deadcode"
)

var deadGroup = group{
	name:    "dead",
	summary: "functions no main package or test reaches (package deadcode)",
	actions: []action{
		{
			name:     "list",
			synopsis: "-root DIR [-pkg P]... [-tests] [-json]",
			help: `Lists the unreachable functions, one per line as PACKAGE<TAB>NAME<TAB>FILE:LINE.
Any dead function exits 1; with -json the []deadcode.Func, still exit 1.`,
			setup: setupDeadList,
		},
		{
			name:     "orphaned",
			synopsis: "-root DIR [-pkg P]... [-tests] [-json] BEFORE [AFTER]",
			help: `Compares dead functions at git revisions BEFORE and AFTER; without AFTER the
working tree is used. Prints a "newly dead:" line followed by the newly dead
functions, then a "revived:" line followed by the revived ones, each as
PACKAGE<TAB>NAME<TAB>FILE:LINE. Any newly dead function exits 1; with -json
the deadcode.Diff, still exit 1.`,
			setup: setupDeadOrphaned,
		},
	},
}

// addDeadFlags registers the flags both dead actions share.
func addDeadFlags(flags *flag.FlagSet) (root *string, packages *listFlag, tests, asJSON *bool) {
	root = addRootFlag(flags)
	packages = addPackagesFlag(flags)
	tests = flags.Bool("tests", false, "treat test functions as roots too")
	asJSON = addJSONFlag(flags)
	return root, packages, tests, asJSON
}

func printFuncs(funcs []deadcode.Func) {
	for _, fn := range funcs {
		fmt.Printf("%s\t%s\t%s:%d\n", fn.Package, fn.Name, fn.File, fn.Line)
	}
}

func setupDeadList(flags *flag.FlagSet) func(context.Context, []string) error {
	root, packages, tests, asJSON := addDeadFlags(flags)
	return func(ctx context.Context, args []string) error {
		if len(args) != 0 {
			return usageErrorf("unexpected arguments: %v", args)
		}
		absolute, err := absRoot(*root)
		if err != nil {
			return err
		}
		dead, err := deadcode.Dead(ctx, absolute, deadcode.Options{Patterns: *packages, Tests: *tests})
		if err != nil {
			return err
		}
		if *asJSON {
			if err := printJSON(dead); err != nil {
				return err
			}
		} else {
			printFuncs(dead)
		}
		if len(dead) > 0 {
			return errNegative
		}
		return nil
	}
}

func setupDeadOrphaned(flags *flag.FlagSet) func(context.Context, []string) error {
	root, packages, tests, asJSON := addDeadFlags(flags)
	return func(ctx context.Context, args []string) error {
		if len(args) != 1 && len(args) != 2 {
			return usageErrorf("want BEFORE and an optional AFTER revision, got %d arguments", len(args))
		}
		// An empty AFTER makes Orphaned analyse the working tree (rule 10).
		after := ""
		if len(args) == 2 {
			after = args[1]
		}
		absolute, err := absRoot(*root)
		if err != nil {
			return err
		}
		options := deadcode.Options{Patterns: *packages, Tests: *tests}
		diff, err := deadcode.Orphaned(ctx, absolute, args[0], after, options)
		if err != nil {
			return err
		}
		if *asJSON {
			if err := printJSON(diff); err != nil {
				return err
			}
		} else {
			fmt.Println("newly dead:")
			printFuncs(diff.NewlyDead)
			fmt.Println("revived:")
			printFuncs(diff.Revived)
		}
		if len(diff.NewlyDead) > 0 {
			return errNegative
		}
		return nil
	}
}
