package refactor

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"

	"golang.org/x/tools/imports"

	"github.com/AleksanderMako/machanised-vibe-coding/editset"
)

// fileChange is one file before and after an operation.
type fileChange struct {
	path     string
	existed  bool        // the file was on disk before
	original []byte      // bytes before, when it existed
	content  []byte      // bytes after; nil when the file is deleted
	mode     os.FileMode // permissions to write with
}

// commit writes every change, runs goimports, builds, and restores everything when the build fails.
// It returns the written files, absolute and sorted, even when the build fails.
func commit(ctx context.Context, root string, changes []*fileChange, opts Options) ([]string, error) {
	slices.SortFunc(changes, func(a, b *fileChange) int { return strings.Compare(a.path, b.path) })
	written := []string{}
	for _, change := range changes {
		if change.content != nil {
			written = append(written, change.path)
		}
	}
	// Cancellation seen before any write leaves the tree untouched.
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	var createdDirs []string
	restore := func() { restoreAll(changes, createdDirs) }

	for _, change := range changes {
		if change.content == nil {
			if err := os.Remove(change.path); err != nil {
				restore()
				return nil, fmt.Errorf("refactor: delete %s: %w", change.path, err)
			}
			continue
		}
		dirs, err := makeDirs(filepath.Dir(change.path))
		createdDirs = append(createdDirs, dirs...)
		if err != nil {
			restore()
			return nil, err
		}
		if err := writeKeepingMode(change.path, change.content, change.mode); err != nil {
			restore()
			return nil, fmt.Errorf("refactor: write %s: %w", change.path, err)
		}
	}

	// goimports runs only after every file is on disk, so new packages are visible to it.
	if err := formatAll(changes); err != nil {
		restore()
		return written, err
	}

	if !opts.NoBuild {
		if err := buildWritten(ctx, root, written); err != nil {
			restore()
			return written, err
		}
	}
	return written, nil
}

// makeDirs creates dir and its missing parents and returns the created ones, outermost first.
func makeDirs(dir string) ([]string, error) {
	var missing []string
	for current := dir; ; current = filepath.Dir(current) {
		if _, err := os.Stat(current); err == nil {
			break
		}
		missing = append(missing, current)
		if filepath.Dir(current) == current {
			break
		}
	}
	slices.Reverse(missing)
	var created []string
	for _, missingDir := range missing {
		if err := os.Mkdir(missingDir, 0o755); err != nil {
			return created, fmt.Errorf("refactor: create %s: %w", missingDir, err)
		}
		created = append(created, missingDir)
	}
	return created, nil
}

// restoreAll puts back original bytes, deletes created files and removes created directories.
// Errors are ignored because the first error is what the caller needs.
func restoreAll(changes []*fileChange, createdDirs []string) {
	for _, change := range changes {
		if change.existed {
			_ = writeKeepingMode(change.path, change.original, change.mode)
		} else {
			_ = os.Remove(change.path)
		}
	}
	for index := len(createdDirs) - 1; index >= 0; index-- {
		_ = os.Remove(createdDirs[index])
	}
}

// writeKeepingMode writes data and re-applies the mode in case a umask altered it.
func writeKeepingMode(path string, data []byte, mode os.FileMode) error {
	if mode == 0 {
		mode = 0o644
	}
	if err := os.WriteFile(path, data, mode); err != nil {
		return err
	}
	return os.Chmod(path, mode)
}

// formatAll runs goimports on every written .go file; a parse failure is reported as a build failure.
func formatAll(changes []*fileChange) error {
	var failedDirs, output []string
	for _, change := range changes {
		if change.content == nil || !strings.HasSuffix(change.path, ".go") {
			continue
		}
		formatted, err := imports.Process(change.path, change.content, nil)
		if err != nil {
			failedDirs = append(failedDirs, filepath.Dir(change.path))
			output = append(output, err.Error())
			continue
		}
		if bytes.Equal(formatted, change.content) {
			continue
		}
		change.content = formatted
		if err := writeKeepingMode(change.path, change.content, change.mode); err != nil {
			return fmt.Errorf("refactor: write %s: %w", change.path, err)
		}
	}
	if len(failedDirs) > 0 {
		return &editset.BuildError{Packages: sortedUnique(failedDirs), Output: strings.Join(output, "\n")}
	}
	return nil
}

// buildWritten builds the directories of written .go files, vetting those with test files so tests compile too.
func buildWritten(ctx context.Context, root string, written []string) error {
	needsVet := map[string]bool{}
	for _, file := range written {
		if !strings.HasSuffix(file, ".go") {
			continue
		}
		dir := filepath.Dir(file)
		needsVet[dir] = needsVet[dir] || strings.HasSuffix(file, "_test.go")
	}
	var buildDirs, vetDirs []string
	for dir, vet := range needsVet {
		if vet {
			vetDirs = append(vetDirs, dir)
		} else {
			buildDirs = append(buildDirs, dir)
		}
	}
	slices.Sort(buildDirs)
	slices.Sort(vetDirs)

	var failedDirs []string
	var output strings.Builder
	if len(buildDirs) > 0 {
		// -o to the null device keeps go build from dropping a binary into Root for main packages.
		args := append([]string{"build", "-o", os.DevNull}, relativeArgs(root, buildDirs)...)
		failed, text, err := runGo(ctx, root, args)
		if err != nil {
			return err
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
			return err
		}
		if failed {
			failedDirs = append(failedDirs, vetDirs...)
			output.WriteString(text)
		}
	}
	if len(failedDirs) > 0 {
		return &editset.BuildError{Packages: sortedUnique(failedDirs), Output: output.String()}
	}
	return nil
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
	return false, "", fmt.Errorf("refactor: run go %s: %w", args[0], runErr)
}

// sortedUnique returns the strings sorted with duplicates removed.
func sortedUnique(values []string) []string {
	sorted := slices.Clone(values)
	slices.Sort(sorted)
	return slices.Compact(sorted)
}
