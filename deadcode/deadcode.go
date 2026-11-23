// Package deadcode finds functions no root reaches, and the functions a change
// between two git revisions newly orphaned.
package deadcode

import (
	"context"
	"errors"
	"sort"
)

// Func is one function or method.
type Func struct {
	Package string // import path
	Name    string // "Name" or "Type.Method"
	File    string // path relative to the module root
	Line    int
}

// Key is the stable identity of a Func across revisions: Package and Name.
func (f Func) Key() string { return f.Package + "." + f.Name }

// Options controls the analysis.
type Options struct {
	Patterns []string // package patterns relative to Root; empty means ./...
	Tests    bool     // treat test functions as roots too
}

// Dead lists the functions of the loaded packages that no root reaches.
func Dead(ctx context.Context, root string, opts Options) ([]Func, error) {
	return analyse(ctx, root, opts)
}

// Diff is what changed between two revisions.
type Diff struct {
	Before, After []Func // the full lists
	NewlyDead     []Func // dead after and not before
	Revived       []Func // dead before and not after
}

// Orphaned runs Dead at two git revisions in temporary worktrees and diffs the results.
func Orphaned(ctx context.Context, root, beforeRef, afterRef string, opts Options) (Diff, error) {
	before, err := deadAtRevision(ctx, root, beforeRef, opts)
	if err != nil {
		return Diff{}, err
	}
	after, err := deadAtRevision(ctx, root, afterRef, opts)
	if err != nil {
		return Diff{}, err
	}
	return Diff{
		Before:    before,
		After:     after,
		NewlyDead: missingFrom(after, before),
		Revived:   missingFrom(before, after),
	}, nil
}

// ErrNoRoots is wrapped when there is no main package and no test to start from.
var ErrNoRoots = errors.New("no main packages or tests to use as roots")

// missingFrom returns the functions of list whose Key does not occur in other, keeping list's order.
func missingFrom(list, other []Func) []Func {
	otherKeys := map[string]bool{}
	for _, function := range other {
		otherKeys[function.Key()] = true
	}
	missing := []Func{}
	for _, function := range list {
		if !otherKeys[function.Key()] {
			missing = append(missing, function)
		}
	}
	return missing
}

// sortFuncs orders functions by Package, then File, then Line, with Name breaking ties.
func sortFuncs(functions []Func) {
	sort.Slice(functions, func(i, j int) bool {
		a, b := functions[i], functions[j]
		if a.Package != b.Package {
			return a.Package < b.Package
		}
		if a.File != b.File {
			return a.File < b.File
		}
		if a.Line != b.Line {
			return a.Line < b.Line
		}
		return a.Name < b.Name
	})
}
