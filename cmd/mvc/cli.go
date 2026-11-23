package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// Exit codes of rule 3 of the stage 8 contract.
const (
	exitOK       = 0
	exitNegative = 1
	exitError    = 2
)

// group is one first level subcommand such as "edit".
type group struct {
	name    string
	summary string
	actions []action
}

// action is one second level subcommand such as "edit check".
type action struct {
	name     string
	synopsis string // flags and arguments, shown in usage
	help     string // what it does and how the text output looks
	// setup registers the flags on fs and returns the function that runs
	// the action with the positional arguments left after parsing.
	setup func(fs *flag.FlagSet) func(ctx context.Context, args []string) error
}

// errNegative tells dispatch that the result was printed and is negative, so exit 1.
var errNegative = errors.New("negative result")

// usageError is a wrong invocation found after flag parsing, such as a missing argument.
type usageError struct{ message string }

func (e *usageError) Error() string { return e.message }

// usageErrorf builds a usageError so actions can reject bad arguments in one line.
func usageErrorf(format string, args ...any) error {
	return &usageError{message: fmt.Sprintf(format, args...)}
}

// isHelp reports whether an argument asks for usage.
func isHelp(arg string) bool {
	return arg == "-h" || arg == "-help" || arg == "--h" || arg == "--help"
}

// dispatch routes the arguments to a group and action and returns the exit code.
func dispatch(ctx context.Context, args []string) int {
	if len(args) == 0 || isHelp(args[0]) {
		printTopUsage(os.Stdout)
		return exitOK
	}
	chosenGroup, found := findGroup(args[0])
	if !found {
		fmt.Fprintf(os.Stderr, "mvc: unknown group %q\n", args[0])
		printTopUsage(os.Stderr)
		return exitError
	}
	if len(args) == 1 || isHelp(args[1]) {
		printGroupUsage(os.Stdout, chosenGroup)
		return exitOK
	}
	chosenAction, found := findAction(chosenGroup, args[1])
	if !found {
		fmt.Fprintf(os.Stderr, "mvc: unknown action %q in group %s\n", args[1], chosenGroup.name)
		printGroupUsage(os.Stderr, chosenGroup)
		return exitError
	}
	return runAction(ctx, chosenGroup, chosenAction, args[2:])
}

func findGroup(name string) (group, bool) {
	for _, candidate := range groups {
		if candidate.name == name {
			return candidate, true
		}
	}
	return group{}, false
}

func findAction(g group, name string) (action, bool) {
	for _, candidate := range g.actions {
		if candidate.name == name {
			return candidate, true
		}
	}
	return action{}, false
}

// runAction parses the flags of one action, runs it and maps its error to an exit code.
func runAction(ctx context.Context, g group, a action, args []string) int {
	flags := flag.NewFlagSet("mvc "+g.name+" "+a.name, flag.ContinueOnError)
	// Silence the flag package so this file decides where usage goes.
	flags.SetOutput(io.Discard)
	flags.Usage = func() {}
	run := a.setup(flags)

	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			printActionUsage(os.Stdout, g, a, flags)
			return exitOK
		}
		fmt.Fprintln(os.Stderr, "mvc: "+oneLine(err.Error()))
		printActionUsage(os.Stderr, g, a, flags)
		return exitError
	}

	err := run(ctx, flags.Args())
	var badUsage *usageError
	switch {
	case err == nil:
		return exitOK
	case errors.Is(err, errNegative):
		return exitNegative
	case errors.As(err, &badUsage):
		fmt.Fprintln(os.Stderr, "mvc: "+oneLine(badUsage.message))
		printActionUsage(os.Stderr, g, a, flags)
		return exitError
	default:
		fmt.Fprintln(os.Stderr, "mvc: "+oneLine(err.Error()))
		return exitError
	}
}

// oneLine folds a multi-line error so stderr carries exactly one line per error.
func oneLine(message string) string {
	return strings.ReplaceAll(strings.TrimSpace(message), "\n", "; ")
}

func printTopUsage(w io.Writer) {
	fmt.Fprintln(w, "usage: mvc <group> <action> [flags] [args]")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "groups:")
	for _, g := range groups {
		fmt.Fprintf(w, "  %-9s %s\n", g.name, g.summary)
	}
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Run 'mvc <group>' for its actions and 'mvc <group> <action> -h' for flags.")
	fmt.Fprintln(w, "Flags come before positional arguments; -pkg, -skip, -env, -manifest,")
	fmt.Fprintln(w, "-distinct, -together and -requires may be repeated. -root defaults to the")
	fmt.Fprintln(w, "current directory.")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "output: text, one line per item, tab separated columns; with -json the")
	fmt.Fprintln(w, "package result encoded as indented JSON and nothing else on stdout.")
	fmt.Fprintln(w, "Errors go to stderr as one line starting with 'mvc: '.")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "exit codes: 0 success, 1 negative result (printed in full, also with")
	fmt.Fprintln(w, "-json), 2 usage error or any other error.")
}

func printGroupUsage(w io.Writer, g group) {
	fmt.Fprintf(w, "usage: mvc %s <action> [flags] [args]\n", g.name)
	fmt.Fprintln(w)
	fmt.Fprintln(w, g.summary)
	fmt.Fprintln(w)
	fmt.Fprintln(w, "actions:")
	for _, a := range g.actions {
		fmt.Fprintf(w, "  mvc %s %s %s\n", g.name, a.name, a.synopsis)
	}
	fmt.Fprintln(w)
	fmt.Fprintf(w, "Run 'mvc %s <action> -h' for the output format and flags.\n", g.name)
}

func printActionUsage(w io.Writer, g group, a action, flags *flag.FlagSet) {
	fmt.Fprintf(w, "usage: mvc %s %s %s\n", g.name, a.name, a.synopsis)
	fmt.Fprintln(w)
	fmt.Fprintln(w, strings.TrimSpace(a.help))
	fmt.Fprintln(w)
	fmt.Fprintln(w, "flags:")
	flags.SetOutput(w)
	flags.PrintDefaults()
	flags.SetOutput(io.Discard)
}

// listFlag collects every value of a flag that may be repeated.
type listFlag []string

func (l *listFlag) String() string { return strings.Join(*l, ",") }

func (l *listFlag) Set(value string) error {
	*l = append(*l, value)
	return nil
}

// addRootFlag registers -root, which defaults to the current directory (rule 1).
func addRootFlag(flags *flag.FlagSet) *string {
	return flags.String("root", ".", "module or repository root `DIR`")
}

// addJSONFlag registers -json, shared by every action that can print JSON.
func addJSONFlag(flags *flag.FlagSet) *bool {
	return flags.Bool("json", false, "print the result as indented JSON")
}

// absRoot resolves -root to an absolute path so packages never see a relative root.
func absRoot(root string) (string, error) {
	return filepath.Abs(root)
}

// requireFlags returns a usage error naming the first listed flag that was not given.
func requireFlags(flags *flag.FlagSet, names ...string) error {
	given := map[string]bool{}
	flags.Visit(func(f *flag.Flag) { given[f.Name] = true })
	for _, name := range names {
		if !given[name] {
			return usageErrorf("flag -%s is required", name)
		}
	}
	return nil
}

// printJSON writes value as indented JSON to stdout.
func printJSON(value any) error {
	encoded, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	_, err = fmt.Fprintln(os.Stdout, string(encoded))
	return err
}

// printLines writes each string on its own line, for lists of files or packages.
func printLines(lines []string) {
	for _, line := range lines {
		fmt.Fprintln(os.Stdout, line)
	}
}

// printVerbatim writes tool output as is, adding a final newline only when missing.
func printVerbatim(output string) {
	if output == "" {
		return
	}
	fmt.Fprint(os.Stdout, output)
	if !strings.HasSuffix(output, "\n") {
		fmt.Fprintln(os.Stdout)
	}
}
