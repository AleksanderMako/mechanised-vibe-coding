// Package editset applies text edits across files as one transaction: every
// anchor is checked first, then all files are written, formatted and built.
package editset

import (
	"context"
	"io"
)

// Edit replaces exactly one occurrence of Old in File with New.
type Edit struct {
	File string // absolute, or relative to Options.Root
	Old  string // must occur exactly once in the file at the time the edit is applied
	New  string // may be empty, which deletes Old
}

// Options controls Apply. Root is required.
type Options struct {
	Root     string // module root; relative Edit.File paths resolve against it
	NoFormat bool   // skip goimports on touched .go files
	NoBuild  bool   // skip building the packages of touched .go files
}

// Result reports what Apply changed.
type Result struct {
	Touched  []string // files written, absolute paths, in first-edit order
	Packages []string // package directories built, absolute paths, sorted
}

// Problem is one edit that cannot be applied.
type Problem struct {
	Index  int    // position in the edits slice
	File   string // absolute path
	Reason string // one of the Reason* constants
	Count  int    // occurrences of Old found
}

// Reasons an edit cannot be applied.
const (
	ReasonMissingFile = "file does not exist or cannot be read"
	ReasonEmptyOld    = "old text is empty"
	ReasonNotFound    = "old text not found"
	ReasonNotUnique   = "old text occurs more than once"
	ReasonOutsideRoot = "file is outside the root"
)

// CheckError is returned when at least one edit cannot be applied. Nothing was written.
type CheckError struct {
	Problems []Problem // every failing edit, in index order
}

func (e *CheckError) Error() string { panic("not implemented") }

// BuildError is returned when the touched packages do not build after the edits.
type BuildError struct {
	Packages []string // package directories that failed, absolute paths
	Output   string   // combined go build output
}

func (e *BuildError) Error() string { panic("not implemented") }

// Check validates the edits without writing anything.
func Check(edits []Edit, opts Options) error { panic("not implemented") }

// Apply validates, writes, formats and builds; see the stage contract for the exact rules.
func Apply(ctx context.Context, edits []Edit, opts Options) (Result, error) {
	panic("not implemented")
}

// ParseJSON reads edits from a JSON array of {"file","old","new"} objects.
func ParseJSON(r io.Reader) ([]Edit, error) { panic("not implemented") }
