// Package refactor rewrites Go source with type information: rename, move to
// another package, and turn an unused-receiver method into a function. Every
// operation rewrites all references, writes all or nothing, and builds.
package refactor

import (
	"context"
	"errors"

	"github.com/AleksanderMako/machanised-vibe-coding/symbols"
)

// Options controls loading and the final build.
type Options struct {
	Patterns []string // package patterns relative to Root; empty means ./...
	NoBuild  bool     // skip the build after writing
}

// Sentinel errors, wrapped in the returned errors.
var (
	ErrConflict        = errors.New("name already in use")
	ErrReceiverUsed    = errors.New("receiver is used in the method body")
	ErrUnsupportedUse  = errors.New("a use of the symbol cannot be rewritten")
	ErrLeftBehind      = errors.New("moved code depends on unexported symbols left behind")
	ErrBreaksInterface = errors.New("the type would stop satisfying an interface")
	ErrBadName         = errors.New("not a valid Go identifier")
)

// Rename renames sym everywhere it is declared and used.
func Rename(ctx context.Context, root string, sym symbols.Symbol, newName string, opts Options) ([]string, error) {
	return rename(ctx, root, sym, newName, opts)
}

// Move moves top level declarations to the package at toImportPath, into toFile.
func Move(ctx context.Context, root string, syms []symbols.Symbol, toImportPath, toFile string, opts Options) ([]string, error) {
	return move(ctx, root, syms, toImportPath, toFile, opts)
}

// Demethodize turns method Type.Method into a package level function and rewrites every call.
func Demethodize(ctx context.Context, root string, method symbols.Symbol, newName string, opts Options) ([]string, error) {
	return demethodize(ctx, root, method, newName, opts)
}
