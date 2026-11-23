// Command mvc exposes the packages of this module as subcommands; it parses
// flags, calls one package function and prints the result, nothing more.
package main

import (
	"context"
	"os"
	"os/signal"
)

// groups is the dispatch table: every group and its actions, in usage order.
var groups = []group{
	editGroup,
	astGroup,
	verifyGroup,
	symbolsGroup,
	refactorGroup,
	envGroup,
	mergeGroup,
	deadGroup,
}

func main() {
	// Ctrl-C cancels the context so child go and git processes stop too.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	code := dispatch(ctx, os.Args[1:])
	stop()
	os.Exit(code)
}
