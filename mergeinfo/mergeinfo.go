// Package mergeinfo explains merge conflicts: the declaration each side is in,
// what was deleted, and the commits on the other side that touched it.
package mergeinfo

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
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
func Detect(ctx context.Context, root string) (State, error) {
	absoluteRoot, err := filepath.Abs(root)
	if err != nil {
		return State{}, fmt.Errorf("mergeinfo: root %s: %w", root, err)
	}
	gitDirOutput, err := runGit(ctx, absoluteRoot, "rev-parse", "--git-dir")
	if err != nil {
		return State{}, fmt.Errorf("mergeinfo: %s is not inside a git work tree: %w", root, err)
	}
	gitDir := strings.TrimSpace(gitDirOutput)
	// git prints the directory relative to where it ran unless it lies elsewhere.
	if !filepath.IsAbs(gitDir) {
		gitDir = filepath.Join(absoluteRoot, gitDir)
	}

	kind, headFile := "", ""
	switch {
	case fileExists(filepath.Join(gitDir, "MERGE_HEAD")):
		kind, headFile = "merge", "MERGE_HEAD"
	case (directoryExists(filepath.Join(gitDir, "rebase-merge")) || directoryExists(filepath.Join(gitDir, "rebase-apply"))) &&
		fileExists(filepath.Join(gitDir, "REBASE_HEAD")):
		kind, headFile = "rebase", "REBASE_HEAD"
	case fileExists(filepath.Join(gitDir, "CHERRY_PICK_HEAD")):
		kind, headFile = "cherry-pick", "CHERRY_PICK_HEAD"
	default:
		return State{}, ErrNoConflict
	}

	theirContent, err := os.ReadFile(filepath.Join(gitDir, headFile))
	if err != nil {
		return State{}, fmt.Errorf("mergeinfo: read %s: %w", headFile, err)
	}
	ourHead, err := runGit(ctx, absoluteRoot, "rev-parse", "--verify", "HEAD")
	if err != nil {
		return State{}, fmt.Errorf("mergeinfo: resolve HEAD: %w", err)
	}
	// MERGE_HEAD holds one hash per line for octopus merges; the first is the one merged in.
	theirHead, _, _ := strings.Cut(strings.TrimSpace(string(theirContent)), "\n")
	return State{
		Kind:      kind,
		OurHead:   strings.TrimSpace(ourHead),
		TheirHead: strings.TrimSpace(theirHead),
	}, nil
}

// Conflicts parses every conflicted file and explains each hunk.
func Conflicts(ctx context.Context, root string) ([]Hunk, error) {
	state, err := Detect(ctx, root)
	if err != nil {
		return nil, err
	}
	absoluteRoot, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("mergeinfo: root %s: %w", root, err)
	}
	// git diff prints paths relative to the top level; cdup leads from root up to it.
	cdupOutput, err := runGit(ctx, absoluteRoot, "rev-parse", "--show-cdup")
	if err != nil {
		return nil, fmt.Errorf("mergeinfo: find top level: %w", err)
	}
	toTopLevel := strings.TrimSpace(cdupOutput)
	namesOutput, err := runGit(ctx, absoluteRoot, "diff", "--name-only", "--diff-filter=U", "-z")
	if err != nil {
		return nil, fmt.Errorf("mergeinfo: list conflicted files: %w", err)
	}

	hunks := []Hunk{}
	seen := map[string]bool{}
	for _, topLevelPath := range strings.Split(namesOutput, "\x00") {
		if topLevelPath == "" || seen[topLevelPath] {
			continue
		}
		seen[topLevelPath] = true
		// Relative to root, so it works both as a file path and as a git pathspec run from root.
		relativePath := filepath.Join(toTopLevel, filepath.FromSlash(topLevelPath))
		fileHunks, err := explainFile(ctx, absoluteRoot, relativePath, state)
		if err != nil {
			return nil, err
		}
		hunks = append(hunks, fileHunks...)
	}
	return hunks, nil
}

// explainFile parses one conflicted file and fills in declarations and commits for its hunks.
func explainFile(ctx context.Context, root, relativePath string, state State) ([]Hunk, error) {
	absolutePath := filepath.Join(root, relativePath)
	content, err := os.ReadFile(absolutePath)
	if errors.Is(err, os.ErrNotExist) {
		// Both sides deleted the file, so there are no markers to explain.
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("mergeinfo: read %s: %w", absolutePath, err)
	}
	hunks, err := ParseHunks(string(content))
	if err != nil {
		return nil, fmt.Errorf("mergeinfo: %s: %w", absolutePath, err)
	}
	if len(hunks) == 0 {
		return hunks, nil
	}
	for index := range hunks {
		hunks[index].File = absolutePath
	}
	if strings.HasSuffix(absolutePath, ".go") {
		fillDeclarations(string(content), hunks)
	}
	if err := fillTheirCommits(ctx, root, relativePath, state, hunks); err != nil {
		return nil, err
	}
	return hunks, nil
}

// fillDeclarations resolves the file to each side and names the declaration around every hunk.
func fillDeclarations(content string, hunks []Hunk) {
	oursFile, oursLines := resolveToSide(content, hunks, func(hunk Hunk) string { return hunk.Ours })
	theirsFile, theirsLines := resolveToSide(content, hunks, func(hunk Hunk) string { return hunk.Theirs })
	for index := range hunks {
		// A parse error leaves the declaration empty, as the contract allows.
		hunks[index].OursDecl, _ = EnclosingDecl([]byte(oursFile), oursLines[index])
		hunks[index].TheirsDecl, _ = EnclosingDecl([]byte(theirsFile), theirsLines[index])
	}
}

// resolveToSide replaces every hunk by one side's text and returns the result together with
// the line where each hunk's text starts; for an empty side that is the line that follows it.
func resolveToSide(content string, hunks []Hunk, side func(Hunk) string) (string, []int) {
	lines := strings.SplitAfter(content, "\n")
	var resolved strings.Builder
	startLines := make([]int, len(hunks))
	resolvedLine := 1
	nextLine := 1
	for index, hunk := range hunks {
		for ; nextLine < hunk.Start; nextLine++ {
			resolved.WriteString(lines[nextLine-1])
			resolvedLine++
		}
		startLines[index] = resolvedLine
		text := side(hunk)
		resolved.WriteString(text)
		resolvedLine += strings.Count(text, "\n")
		nextLine = hunk.End + 1
	}
	for ; nextLine <= len(lines); nextLine++ {
		resolved.WriteString(lines[nextLine-1])
	}
	return resolved.String(), startLines
}

// fillTheirCommits lists their commits on the file and keeps, per hunk, those whose diff touches it.
func fillTheirCommits(ctx context.Context, root, relativePath string, state State, hunks []Hunk) error {
	logOutput, err := runGit(ctx, root, "log", "--no-color", "--format=%H%x00%s%x00%an%x00%aI",
		state.TheirHead, "^"+state.OurHead, "--", relativePath)
	if err != nil {
		return fmt.Errorf("mergeinfo: git log %s: %w", relativePath, err)
	}
	commits, err := parseLog(logOutput)
	if err != nil {
		return err
	}
	if len(commits) == 0 {
		return nil
	}
	// Each commit's diff as a set of its added and removed lines, fetched once per file.
	diffs := make([]commitDiff, len(commits))
	for index, commit := range commits {
		showOutput, err := runGit(ctx, root, "show", "--no-color", "--no-ext-diff", "--format=", commit.Hash, "--", relativePath)
		if err != nil {
			return fmt.Errorf("mergeinfo: git show %s: %w", commit.Hash, err)
		}
		diffs[index] = parseDiff(showOutput)
	}
	for index := range hunks {
		hunk := &hunks[index]
		var matching []Commit
		for commitIndex, commit := range commits {
			if diffTouchesHunk(diffs[commitIndex], *hunk) {
				matching = append(matching, commit)
			}
		}
		if len(matching) == 0 {
			// Never come back empty when their side changed the file at all.
			matching = commits
		}
		hunk.TheirCommits = append([]Commit(nil), matching...)
	}
	return nil
}

// commitDiff holds the added and removed lines of one commit's diff, without the +/- prefix.
type commitDiff struct {
	added   map[string]bool
	removed map[string]bool
}

// parseDiff collects the added and removed lines of `git show` output, skipping file headers.
func parseDiff(output string) commitDiff {
	diff := commitDiff{added: map[string]bool{}, removed: map[string]bool{}}
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSuffix(line, "\r")
		switch {
		case strings.HasPrefix(line, "+++ "), strings.HasPrefix(line, "--- "):
			continue
		case strings.HasPrefix(line, "+"):
			diff.added[line[1:]] = true
		case strings.HasPrefix(line, "-"):
			diff.removed[line[1:]] = true
		}
	}
	return diff
}

// diffTouchesHunk is true when the commit added a line of their side, or removed a line of ours when theirs is empty.
func diffTouchesHunk(diff commitDiff, hunk Hunk) bool {
	if hunk.TheirsDeleted {
		return anyLineIn(hunk.Ours, diff.removed)
	}
	return anyLineIn(hunk.Theirs, diff.added)
}

// anyLineIn reports whether a non-blank line of text is in lines; blank lines match almost every diff.
func anyLineIn(text string, lines map[string]bool) bool {
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSuffix(line, "\r")
		if strings.TrimSpace(line) == "" {
			continue
		}
		if lines[line] {
			return true
		}
	}
	return false
}

// parseLog reads the NUL separated `git log` records, newest first as git prints them.
func parseLog(output string) ([]Commit, error) {
	var commits []Commit
	for _, record := range strings.Split(output, "\n") {
		if strings.TrimSpace(record) == "" {
			continue
		}
		fields := strings.Split(record, "\x00")
		if len(fields) != 4 {
			return nil, fmt.Errorf("mergeinfo: unexpected git log line %q", record)
		}
		date, err := time.Parse(time.RFC3339, strings.TrimSpace(fields[3]))
		if err != nil {
			return nil, fmt.Errorf("mergeinfo: commit %s date: %w", fields[0], err)
		}
		commits = append(commits, Commit{
			Hash:    fields[0],
			Subject: fields[1],
			Author:  fields[2],
			Date:    date,
			PR:      pullRequestNumber(fields[1]),
		})
	}
	return commits, nil
}

// pullRequestPattern matches "(#123)" as merge tools append it to subjects.
var pullRequestPattern = regexp.MustCompile(`\(#(\d+)\)`)

// pullRequestNumber returns the number of the last "(#N)" in subject, 0 when there is none.
func pullRequestNumber(subject string) int {
	matches := pullRequestPattern.FindAllStringSubmatch(subject, -1)
	if len(matches) == 0 {
		return 0
	}
	number, err := strconv.Atoi(matches[len(matches)-1][1])
	if err != nil {
		return 0
	}
	return number
}

// fileExists reports whether path is an existing regular file.
func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.Mode().IsRegular()
}

// directoryExists reports whether path is an existing directory.
func directoryExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}
