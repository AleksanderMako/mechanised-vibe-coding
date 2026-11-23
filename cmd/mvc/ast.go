package main

import (
	"context"
	"flag"

	"github.com/AleksanderMako/mechanised-vibe-coding/astedit"
)

const astHelpSuffix = `
Prints the written files one per line. Every error, including a target that
is not found, ambiguous or already present, goes to stderr with exit 2.`

var astGroup = group{
	name:    "ast",
	summary: "comment preserving edits of Go files by symbol (package astedit)",
	actions: []action{
		{
			name:     "table-case",
			synopsis: "-root DIR -file F -func TestX -case 'ELEMENT'",
			help:     "Appends ELEMENT to the table of test function TestX." + astHelpSuffix,
			setup:    setupASTTableCase,
		},
		{
			name:     "add-field",
			synopsis: "-root DIR -file F -type T -field 'FIELD' [-after NAME]",
			help:     "Adds FIELD to struct T, after field NAME or at the end." + astHelpSuffix,
			setup:    setupASTAddField,
		},
		{
			name:     "remove-field",
			synopsis: "-root DIR -file F -type T -name NAME",
			help:     "Removes field NAME from struct T." + astHelpSuffix,
			setup:    setupASTRemoveField,
		},
		{
			name:     "set-doc",
			synopsis: "-root DIR -file F -symbol SYM -doc 'TEXT'",
			help:     `Replaces the doc comment of SYM ("Name" or "Type.Method") with TEXT.` + astHelpSuffix,
			setup:    setupASTSetDoc,
		},
		{
			name:     "set-const",
			synopsis: "-root DIR -file F -name NAME -value EXPR [-doc 'TEXT']",
			help:     "Sets constant NAME to EXPR and, when -doc is given, its doc comment." + astHelpSuffix,
			setup:    setupASTSetConst,
		},
		{
			name:     "conformance",
			synopsis: "-root DIR -file F -type T -iface IFACE",
			help:     "Adds var _ IFACE = (*T)(nil) after the declaration of T." + astHelpSuffix,
			setup:    setupASTConformance,
		},
	},
}

// runASTOp applies one op with astedit.Apply and prints the written files (rule 8).
func runASTOp(ctx context.Context, root string, args []string, op astedit.Op) error {
	if len(args) != 0 {
		return usageErrorf("unexpected arguments: %v", args)
	}
	absolute, err := absRoot(root)
	if err != nil {
		return err
	}
	written, err := astedit.Apply(ctx, absolute, op)
	if err != nil {
		return err
	}
	printLines(written)
	return nil
}

// addFileFlag registers -file, the Go file every ast action edits.
func addFileFlag(flags *flag.FlagSet) *string {
	return flags.String("file", "", "Go `FILE` to edit, absolute or relative to -root")
}

func setupASTTableCase(flags *flag.FlagSet) func(context.Context, []string) error {
	root := addRootFlag(flags)
	file := addFileFlag(flags)
	testFunc := flags.String("func", "", "test function `NAME` holding the table")
	caseSrc := flags.String("case", "", "Go source of the new table `ELEMENT`")
	return func(ctx context.Context, args []string) error {
		if err := requireFlags(flags, "file", "func", "case"); err != nil {
			return err
		}
		return runASTOp(ctx, *root, args, astedit.AddTableCase(*file, *testFunc, *caseSrc))
	}
}

func setupASTAddField(flags *flag.FlagSet) func(context.Context, []string) error {
	root := addRootFlag(flags)
	file := addFileFlag(flags)
	typeName := flags.String("type", "", "struct type `T`")
	fieldSrc := flags.String("field", "", "Go source of the new `FIELD`, such as 'Name string'")
	after := flags.String("after", "", "insert after field `NAME`; empty appends at the end")
	return func(ctx context.Context, args []string) error {
		if err := requireFlags(flags, "file", "type", "field"); err != nil {
			return err
		}
		return runASTOp(ctx, *root, args, astedit.AddStructField(*file, *typeName, *fieldSrc, *after))
	}
}

func setupASTRemoveField(flags *flag.FlagSet) func(context.Context, []string) error {
	root := addRootFlag(flags)
	file := addFileFlag(flags)
	typeName := flags.String("type", "", "struct type `T`")
	fieldName := flags.String("name", "", "field `NAME` to remove")
	return func(ctx context.Context, args []string) error {
		if err := requireFlags(flags, "file", "type", "name"); err != nil {
			return err
		}
		return runASTOp(ctx, *root, args, astedit.RemoveStructField(*file, *typeName, *fieldName))
	}
}

func setupASTSetDoc(flags *flag.FlagSet) func(context.Context, []string) error {
	root := addRootFlag(flags)
	file := addFileFlag(flags)
	symbol := flags.String("symbol", "", `symbol `+"`SYM`"+`, "Name" or "Type.Method"`)
	doc := flags.String("doc", "", "new doc comment `TEXT`")
	return func(ctx context.Context, args []string) error {
		if err := requireFlags(flags, "file", "symbol", "doc"); err != nil {
			return err
		}
		return runASTOp(ctx, *root, args, astedit.SetDoc(*file, *symbol, *doc))
	}
}

func setupASTSetConst(flags *flag.FlagSet) func(context.Context, []string) error {
	root := addRootFlag(flags)
	file := addFileFlag(flags)
	name := flags.String("name", "", "constant `NAME`")
	value := flags.String("value", "", "new value as a Go `EXPR`")
	doc := flags.String("doc", "", "new doc comment `TEXT`; empty keeps the current one")
	return func(ctx context.Context, args []string) error {
		if err := requireFlags(flags, "file", "name", "value"); err != nil {
			return err
		}
		return runASTOp(ctx, *root, args, astedit.SetConst(*file, *name, *value, *doc))
	}
}

func setupASTConformance(flags *flag.FlagSet) func(context.Context, []string) error {
	root := addRootFlag(flags)
	file := addFileFlag(flags)
	typeName := flags.String("type", "", "type `T` that implements the interface")
	iface := flags.String("iface", "", "interface `IFACE` as written in the file, such as io.Reader")
	return func(ctx context.Context, args []string) error {
		if err := requireFlags(flags, "file", "type", "iface"); err != nil {
			return err
		}
		return runASTOp(ctx, *root, args, astedit.AddConformance(*file, *typeName, *iface))
	}
}
