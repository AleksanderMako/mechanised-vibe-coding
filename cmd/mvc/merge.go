package main

import (
	"context"
	"flag"
	"fmt"

	"github.com/AleksanderMako/machanised-vibe-coding/mergeinfo"
)

var mergeGroup = group{
	name:    "merge",
	summary: "explain merge, rebase and cherry-pick conflicts (package mergeinfo)",
	actions: []action{
		{
			name:     "explain",
			synopsis: "-root DIR [-json]",
			help: `Explains every conflict hunk. Per hunk one line
FILE:START-END<TAB>ours=DECL|deleted<TAB>theirs=DECL|deleted
then each of their commits that touched it, newest first, indented as
  HASH<TAB>PR<TAB>SUBJECT
with HASH cut to 12 characters and PR as #123, or - when the subject names
none. Any hunk exits 1; with -json the []mergeinfo.Hunk, still exit 1.
No conflict in progress is an error (exit 2).`,
			setup: setupMergeExplain,
		},
	},
}

func setupMergeExplain(flags *flag.FlagSet) func(context.Context, []string) error {
	root := addRootFlag(flags)
	asJSON := addJSONFlag(flags)
	return func(ctx context.Context, args []string) error {
		if len(args) != 0 {
			return usageErrorf("unexpected arguments: %v", args)
		}
		absolute, err := absRoot(*root)
		if err != nil {
			return err
		}
		hunks, err := mergeinfo.Conflicts(ctx, absolute)
		if err != nil {
			return err
		}
		if *asJSON {
			if err := printJSON(hunks); err != nil {
				return err
			}
		} else {
			printHunks(hunks)
		}
		if len(hunks) > 0 {
			return errNegative
		}
		return nil
	}
}

func printHunks(hunks []mergeinfo.Hunk) {
	for _, hunk := range hunks {
		fmt.Printf("%s:%d-%d\tours=%s\ttheirs=%s\n", hunk.File, hunk.Start, hunk.End,
			sideLabel(hunk.OursDecl, hunk.OursDeleted), sideLabel(hunk.TheirsDecl, hunk.TheirsDeleted))
		for _, commit := range hunk.TheirCommits {
			fmt.Printf("  %s\t%s\t%s\n", shortHash(commit.Hash), prLabel(commit.PR), commit.Subject)
		}
	}
}

// sideLabel says "deleted" for an empty side, since its declaration is then meaningless.
func sideLabel(decl string, deleted bool) string {
	if deleted {
		return "deleted"
	}
	return decl
}

func shortHash(hash string) string {
	if len(hash) > 12 {
		return hash[:12]
	}
	return hash
}

func prLabel(number int) string {
	if number == 0 {
		return "-"
	}
	return fmt.Sprintf("#%d", number)
}
