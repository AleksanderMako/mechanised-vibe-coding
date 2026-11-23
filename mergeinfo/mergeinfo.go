// Package mergeinfo explains merge conflicts: the declaration each side is in,
// what was deleted, and the commits on the other side that touched it.
package mergeinfo

import (
	"context"
	"errors"
	"time"
)

// Commit is one commit on the other side that touched a hunk.
type Commit struct {
	Hash    string // full hash
	Subject string // first line of the message
	Author  string // author name
	Date    time.Time
	PR      int // the number in a trailing "(#123)" of the subject, 0 when none
}

// Hunk is one conflict in one file.
type Hunk struct {
	File          string   // absolute path
	Start, End    int      // 1-based lines of the <<<<<<< and >>>>>>> markers
	Ours, Theirs  string   // text between the markers for each side
	Base          string   // common ancestor text for diff3 markers
	OursDecl      string   // enclosing top level declaration on our side
	TheirsDecl    string   // same for their side
	OursDeleted   bool     // our side is empty
	TheirsDeleted bool     // their side is empty
	TheirCommits  []Commit // their commits that touched the hunk, newest first
}

// State describes the in-progress operation.
type State struct {
	Kind      string // "merge", "rebase" or "cherry-pick"
	OurHead   string // hash of HEAD
	TheirHead string // hash of MERGE_HEAD, REBASE_HEAD or CHERRY_PICK_HEAD
}

// ErrNoConflict is returned when the repository has no conflict in progress.
var ErrNoConflict = errors.New("no merge, rebase or cherry-pick with conflicts in progress")

// Detect reports the in-progress operation.
func Detect(ctx context.Context, root string) (State, error) { panic("not implemented") }

// Conflicts parses every conflicted file and explains each hunk.
func Conflicts(ctx context.Context, root string) ([]Hunk, error) { panic("not implemented") }

// ParseHunks reads conflict markers from file content.
func ParseHunks(content string) ([]Hunk, error) { panic("not implemented") }

// EnclosingDecl names the top level declaration of a Go file covering the 1-based line.
func EnclosingDecl(src []byte, line int) (string, error) { panic("not implemented") }
