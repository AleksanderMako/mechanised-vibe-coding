# Stage 6: mergeinfo

One package that explains merge conflicts before anyone edits them. For every
conflict hunk it reports the enclosing Go declaration on each side, whether a
side deleted the code, and the commits on the other side that touched it,
with pull request numbers taken from the subjects. Carrying the intent across
stays human; finding out what the intent was is what this package does.

Works on a repository given by `Root` that is in the middle of a merge (or
rebase or cherry-pick: whatever left conflict markers and a `MERGE_HEAD`,
`REBASE_HEAD` or `CHERRY_PICK_HEAD`). Shells out to `git` only.

---

## Package mergeinfo

### Exported API

```go
package mergeinfo

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
	Start, End    int      // 1-based line numbers of the <<<<<<< and >>>>>>> markers in the conflicted file
	Ours, Theirs  string   // the text between the markers for each side, without the markers
	Base          string   // the common ancestor text when the markers are diff3 style, "" otherwise
	OursDecl      string   // enclosing top level declaration on our side, "" when none or not a Go file
	TheirsDecl    string   // same for their side
	OursDeleted   bool     // our side is empty
	TheirsDeleted bool     // their side is empty
	TheirCommits  []Commit // commits reachable from their head and not ours that touched the hunk, newest first
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
func Detect(ctx context.Context, root string) (State, error)

// Conflicts parses every conflicted file and explains each hunk.
func Conflicts(ctx context.Context, root string) ([]Hunk, error)

// ParseHunks reads conflict markers from file content; exported so the parsing can be tested without git.
func ParseHunks(content string) ([]Hunk, error)

// EnclosingDecl returns the top level declaration of a Go file that covers the
// given 1-based line, such as "func (c *Consumer) Close", "type Page", "var x",
// "const subCampaignGroupsPerPage" or "import"; "" when the line is between declarations.
func EnclosingDecl(src []byte, line int) (string, error)
```

### Behaviour

`Detect`:

1. `Root` must be inside a git work tree, else an ordinary error. The git directory is found with `git rev-parse --git-dir`.
2. `MERGE_HEAD` present means `merge`; else a `rebase-merge` or `rebase-apply` directory with `REBASE_HEAD` means `rebase`; else `CHERRY_PICK_HEAD` means `cherry-pick`; else `ErrNoConflict`.
3. `OurHead` is `HEAD` resolved; `TheirHead` is the content of the respective ref file.

`ParseHunks`:

4. Markers are lines starting with `<<<<<<< `, `=======`, `>>>>>>> ` and optionally `||||||| ` for diff3 style, each at line start. Line endings are `\n`; `\r\n` input is accepted and the sides keep `\r\n`.
5. A `<<<<<<<` without a matching `>>>>>>>`, or markers in a wrong order, is an error naming the line. Text outside hunks is ignored.
6. `Ours` is the text between `<<<<<<<` and `|||||||` or `=======`; `Base` between `|||||||` and `=======`; `Theirs` between `=======` and `>>>>>>>`. Each side includes its trailing newline when it has lines and is `""` when it has none. `OursDeleted` and `TheirsDeleted` follow from emptiness.
7. `Start` and `End` are the lines of the `<<<<<<<` and `>>>>>>>` markers. `File` is left empty by `ParseHunks`.

`EnclosingDecl`:

8. `src` is parsed with `go/parser` with comments; a parse error is returned as is. Each top level declaration covers the lines from the start of its doc comment to its end line.
9. The result is `func Name`, `func (r *T) Name` or `func (T) Name` as written with the receiver type, `type Name`, `var Name` or `const Name` for single specs, the name of the first spec for grouped `var`/`const` with a trailing `...` when more than one (`const a...`), and `import`. The line being outside every declaration gives `""`.

`Conflicts`:

10. Conflicted files come from `git diff --name-only --diff-filter=U` from `Root`; an empty list returns an empty slice and no error, provided `Detect` succeeds. `Detect` failing returns its error.
11. Each file is read and parsed with `ParseHunks`; `File` is set to the absolute path.
12. For `.go` files, the declaration on each side is found by resolving the whole file to that side (every hunk replaced by its own side's text), parsing it, and taking `EnclosingDecl` at the line where the hunk's text starts in the resolved file. For a deleted side the line examined is the one after the hunk's position, so the declaration the deletion sits inside, if any, is still named. A resolved file that does not parse gives `""` for that side without error.
13. `TheirCommits` is `git log --format=<hash>%x00<subject>%x00<author>%x00<iso date> <TheirHead> ^<OurHead> -- <file>`, newest first, limited to the commits whose diff touches the hunk's text: the commit is kept when `git show <hash> -- <file>` contains at least one line of `Theirs` as an added line (`+` prefixed), or, when their side is deleted, at least one line of `Ours` as a removed line. When no commit matches, every commit of the file in that range is returned, so the list is never empty when their side changed the file.
14. `PR` is parsed from the last `(#N)` in the subject.

### Out of scope

Resolving conflicts, non-Go declaration lookup, submodules, the index stages of git.
