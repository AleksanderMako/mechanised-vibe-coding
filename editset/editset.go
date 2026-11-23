// Package editset applies text edits across files as one transaction: every
// anchor is checked first, then all files are written, formatted and built.
package editset

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"golang.org/x/tools/imports"
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

func (e *CheckError) Error() string {
	var lines []string
	for _, problem := range e.Problems {
		line := fmt.Sprintf("edit %d (%s): %s", problem.Index, problem.File, problem.Reason)
		if problem.Reason == ReasonNotUnique {
			line += fmt.Sprintf(" (%d occurrences)", problem.Count)
		}
		lines = append(lines, line)
	}
	return fmt.Sprintf("editset: %d edit(s) cannot be applied:\n%s", len(e.Problems), strings.Join(lines, "\n"))
}

// BuildError is returned when the touched packages do not build after the edits.
type BuildError struct {
	Packages []string // package directories that failed, absolute paths
	Output   string   // combined go build output
}

func (e *BuildError) Error() string {
	return fmt.Sprintf("editset: build failed for %s:\n%s", strings.Join(e.Packages, ", "), e.Output)
}

// touchedFile is one distinct file with its bytes before and after the edits.
type touchedFile struct {
	path     string
	original []byte
	content  []byte
	mode     os.FileMode
}

// plan is the validated outcome of the edits, ready to be written.
type plan struct {
	root  string
	files []*touchedFile // in first-edit order
}

// Check validates the edits without writing anything.
func Check(edits []Edit, opts Options) error {
	_, err := validate(edits, opts)
	return err
}

// Apply validates, writes, formats and builds; see the stage contract for the exact rules.
func Apply(ctx context.Context, edits []Edit, opts Options) (Result, error) {
	validated, err := validate(edits, opts)
	if err != nil {
		return Result{}, err
	}
	// Cancellation seen before any write means the tree is left untouched.
	if err := ctx.Err(); err != nil {
		return Result{}, err
	}

	if err := writeAll(validated.files); err != nil {
		restoreAll(validated.files)
		return Result{}, err
	}

	if !opts.NoFormat {
		if err := formatGoFiles(validated.files); err != nil {
			restoreAll(validated.files)
			return Result{}, err
		}
	}

	var built []string
	if !opts.NoBuild {
		built, err = buildTouched(ctx, validated.root, validated.files)
		if err != nil {
			restoreAll(validated.files)
			return Result{}, err
		}
	}

	result := Result{Packages: built}
	for _, file := range validated.files {
		result.Touched = append(result.Touched, file.path)
	}
	return result, nil
}

// validate resolves paths, reads every file once and replays the edits in memory.
func validate(edits []Edit, opts Options) (*plan, error) {
	root, err := resolveRoot(opts.Root)
	if err != nil {
		return nil, err
	}

	result := &plan{root: root}
	filesByPath := map[string]*touchedFile{}
	// Remember unreadable paths so each is read only once.
	unreadable := map[string]bool{}
	var problems []Problem

	for index, edit := range edits {
		path := absolutePath(root, edit.File)
		addProblem := func(reason string, count int) {
			problems = append(problems, Problem{Index: index, File: path, Reason: reason, Count: count})
		}

		if !isInside(root, path) {
			addProblem(ReasonOutsideRoot, 0)
			continue
		}

		file, known := filesByPath[path]
		if !known && !unreadable[path] {
			file, err = readFile(path)
			if err != nil {
				unreadable[path] = true
			} else {
				filesByPath[path] = file
				result.files = append(result.files, file)
			}
		}
		if unreadable[path] {
			addProblem(ReasonMissingFile, 0)
			continue
		}

		if edit.Old == "" {
			addProblem(ReasonEmptyOld, 0)
			continue
		}
		count := bytes.Count(file.content, []byte(edit.Old))
		switch {
		case count == 0:
			addProblem(ReasonNotFound, 0)
		case count > 1:
			addProblem(ReasonNotUnique, count)
		default:
			file.content = bytes.Replace(file.content, []byte(edit.Old), []byte(edit.New), 1)
		}
	}

	if len(problems) > 0 {
		return nil, &CheckError{Problems: problems}
	}
	return result, nil
}

// resolveRoot makes Root absolute and checks that it is an existing directory.
func resolveRoot(root string) (string, error) {
	if root == "" {
		return "", errors.New("editset: Options.Root is required")
	}
	absolute, err := filepath.Abs(root)
	if err != nil {
		return "", fmt.Errorf("editset: resolve root: %w", err)
	}
	info, err := os.Stat(absolute)
	if err != nil {
		return "", fmt.Errorf("editset: root: %w", err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("editset: root %s is not a directory", absolute)
	}
	return absolute, nil
}

// absolutePath resolves a file path against root and cleans it.
func absolutePath(root, file string) string {
	if filepath.IsAbs(file) {
		return filepath.Clean(file)
	}
	return filepath.Join(root, file)
}

// isInside reports whether path lies within root; root itself counts and later fails as unreadable.
func isInside(root, path string) bool {
	relative, err := filepath.Rel(root, path)
	if err != nil {
		return false
	}
	return relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator))
}

// readFile loads a regular file and remembers its mode for the later write.
func readFile(path string) (*touchedFile, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("%s is not a regular file", path)
	}
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return &touchedFile{path: path, original: content, content: bytes.Clone(content), mode: info.Mode().Perm()}, nil
}

// writeAll writes the edited content of every touched file.
func writeAll(files []*touchedFile) error {
	for _, file := range files {
		if err := writeKeepingMode(file.path, file.content, file.mode); err != nil {
			return fmt.Errorf("editset: write %s: %w", file.path, err)
		}
	}
	return nil
}

// restoreAll writes back the original bytes; errors are ignored because the first error is what the caller needs.
func restoreAll(files []*touchedFile) {
	for _, file := range files {
		_ = writeKeepingMode(file.path, file.original, file.mode)
	}
}

// writeKeepingMode writes data and re-applies the mode in case a umask or earlier change altered it.
func writeKeepingMode(path string, data []byte, mode os.FileMode) error {
	if err := os.WriteFile(path, data, mode); err != nil {
		return err
	}
	return os.Chmod(path, mode)
}

// formatGoFiles runs goimports on every touched .go file and reports parse failures as a BuildError.
func formatGoFiles(files []*touchedFile) error {
	var failedDirs []string
	var output []string
	for _, file := range files {
		if !strings.HasSuffix(file.path, ".go") {
			continue
		}
		formatted, err := imports.Process(file.path, file.content, nil)
		if err != nil {
			failedDirs = append(failedDirs, filepath.Dir(file.path))
			output = append(output, err.Error())
			continue
		}
		if bytes.Equal(formatted, file.content) {
			continue
		}
		file.content = formatted
		if err := writeKeepingMode(file.path, file.content, file.mode); err != nil {
			return fmt.Errorf("editset: write %s: %w", file.path, err)
		}
	}
	if len(failedDirs) > 0 {
		return &BuildError{Packages: sortedUnique(failedDirs), Output: strings.Join(output, "\n")}
	}
	return nil
}

// buildTouched builds plain package directories and vets directories with touched test files.
func buildTouched(ctx context.Context, root string, files []*touchedFile) ([]string, error) {
	// A directory with a touched test file needs go vet so the test files compile too.
	needsVet := map[string]bool{}
	for _, file := range files {
		if !strings.HasSuffix(file.path, ".go") {
			continue
		}
		dir := filepath.Dir(file.path)
		needsVet[dir] = needsVet[dir] || strings.HasSuffix(file.path, "_test.go")
	}
	if len(needsVet) == 0 {
		return nil, nil
	}

	var buildDirs, vetDirs []string
	for dir, vet := range needsVet {
		if vet {
			vetDirs = append(vetDirs, dir)
		} else {
			buildDirs = append(buildDirs, dir)
		}
	}
	sort.Strings(buildDirs)
	sort.Strings(vetDirs)

	var failedDirs []string
	var output strings.Builder
	if len(buildDirs) > 0 {
		// -o to the null device keeps go build from dropping a binary into Root for main packages.
		args := append([]string{"build", "-o", os.DevNull}, relativeArgs(root, buildDirs)...)
		failed, text, err := runGo(ctx, root, args)
		if err != nil {
			return nil, err
		}
		if failed {
			failedDirs = append(failedDirs, buildDirs...)
			output.WriteString(text)
		}
	}
	if len(vetDirs) > 0 {
		args := append([]string{"vet"}, relativeArgs(root, vetDirs)...)
		failed, text, err := runGo(ctx, root, args)
		if err != nil {
			return nil, err
		}
		if failed {
			failedDirs = append(failedDirs, vetDirs...)
			output.WriteString(text)
		}
	}
	if len(failedDirs) > 0 {
		return nil, &BuildError{Packages: sortedUnique(failedDirs), Output: output.String()}
	}
	return sortedUnique(append(buildDirs, vetDirs...)), nil
}

// relativeArgs turns absolute directories into ./relative package patterns for the go tool.
func relativeArgs(root string, dirs []string) []string {
	var args []string
	for _, dir := range dirs {
		relative, err := filepath.Rel(root, dir)
		if err != nil || relative == "." {
			args = append(args, ".")
			continue
		}
		args = append(args, "./"+filepath.ToSlash(relative))
	}
	return args
}

// runGo runs the go tool in root; failed is true for a non-zero exit, err for anything else.
func runGo(ctx context.Context, root string, args []string) (failed bool, output string, err error) {
	command := exec.CommandContext(ctx, "go", args...)
	command.Dir = root
	combined, runErr := command.CombinedOutput()
	if runErr == nil {
		return false, string(combined), nil
	}
	// A cancelled context kills the process, which must surface as the context error, not a build failure.
	if ctxErr := ctx.Err(); ctxErr != nil {
		return false, "", ctxErr
	}
	var exitErr *exec.ExitError
	if errors.As(runErr, &exitErr) {
		return true, string(combined), nil
	}
	return false, "", fmt.Errorf("editset: run go %s: %w", args[0], runErr)
}

// sortedUnique returns the strings sorted with duplicates removed.
func sortedUnique(values []string) []string {
	seen := map[string]bool{}
	var unique []string
	for _, value := range values {
		if !seen[value] {
			seen[value] = true
			unique = append(unique, value)
		}
	}
	sort.Strings(unique)
	return unique
}

// ParseJSON reads edits from a JSON array of {"file","old","new"} objects.
func ParseJSON(r io.Reader) ([]Edit, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, fmt.Errorf("editset: read edits: %w", err)
	}
	// Checking the first byte rejects null, which json.Unmarshal would accept as an empty slice.
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 || trimmed[0] != '[' {
		return nil, errors.New("editset: edits JSON must be an array")
	}
	var elements []json.RawMessage
	if err := json.Unmarshal(trimmed, &elements); err != nil {
		return nil, fmt.Errorf("editset: edits JSON: %w", err)
	}

	edits := make([]Edit, 0, len(elements))
	for index, element := range elements {
		edit, err := parseEdit(element)
		if err != nil {
			return nil, fmt.Errorf("editset: edit %d: %w", index, err)
		}
		edits = append(edits, edit)
	}
	return edits, nil
}

// parseEdit decodes one array element strictly: known keys only, string values only.
func parseEdit(element json.RawMessage) (Edit, error) {
	trimmed := bytes.TrimSpace(element)
	if len(trimmed) == 0 || trimmed[0] != '{' {
		return Edit{}, errors.New("element is not an object")
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(trimmed, &fields); err != nil {
		return Edit{}, err
	}

	// Sorted keys make the reported error the same on every run.
	keys := make([]string, 0, len(fields))
	for key := range fields {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	var edit Edit
	targets := map[string]*string{"file": &edit.File, "old": &edit.Old, "new": &edit.New}
	for _, key := range keys {
		target, known := targets[key]
		if !known {
			return Edit{}, fmt.Errorf("unknown key %q", key)
		}
		if err := decodeString(fields[key], target); err != nil {
			return Edit{}, fmt.Errorf("key %q: %w", key, err)
		}
	}
	for _, required := range []string{"file", "old"} {
		if _, present := fields[required]; !present {
			return Edit{}, fmt.Errorf("missing key %q", required)
		}
	}
	return edit, nil
}

// decodeString accepts only a JSON string, so null and numbers are type errors.
func decodeString(value json.RawMessage, target *string) error {
	trimmed := bytes.TrimSpace(value)
	if len(trimmed) == 0 || trimmed[0] != '"' {
		return errors.New("value must be a string")
	}
	return json.Unmarshal(trimmed, target)
}
