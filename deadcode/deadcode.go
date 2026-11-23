// Package deadcode finds functions no root reaches, and the functions a change
// between two git revisions newly orphaned.
package deadcode

import (
	"context"
	"errors"
)

// Func is one function or method.
type Func struct {
	Package string // import path
	Name    string // "Name" or "Type.Method"
	File    string // path relative to the module root
	Line    int
}

// Key is the stable identity of a Func across revisions: Package and Name.
func (f Func) Key() string { panic("not implemented") }

// Options controls the analysis.
type Options struct {
	Patterns []string // package patterns relative to Root; empty means ./...
	Tests    bool     // treat test functions as roots too
}

// Dead lists the functions of the loaded packages that no root reaches.
func Dead(ctx context.Context, root string, opts Options) ([]Func, error) { panic("not implemented") }

// Diff is what changed between two revisions.
type Diff struct {
	Before, After []Func // the full lists
	NewlyDead     []Func // dead after and not before
	Revived       []Func // dead before and not after
}

// Orphaned runs Dead at two git revisions in temporary worktrees and diffs the results.
func Orphaned(ctx context.Context, root, beforeRef, afterRef string, opts Options) (Diff, error) {
	panic("not implemented")
}

// ErrNoRoots is wrapped when there is no main package and no test to start from.
var ErrNoRoots = errors.New("no main packages or tests to use as roots")
