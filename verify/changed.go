package verify

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ChangedPackages lists the package patterns of .go files changed against HEAD:
// staged, unstaged and untracked files, as "./relative/dir", sorted and unique,
// excluding directories under testdata or vendor. A clean tree returns an empty slice.
func ChangedPackages(ctx context.Context, root string) ([]string, error) {
	if root == "" {
		return nil, fmt.Errorf("verify: root is empty")
	}
	absoluteRoot, err := realPath(root)
	if err != nil {
		return nil, fmt.Errorf("verify: root %s: %w", root, err)
	}
	topLevel, err := gitTopLevel(ctx, absoluteRoot)
	if err != nil {
		return nil, err
	}
	// -z keeps paths unquoted and splits renames into new and original path.
	status, err := runCommand(ctx, absoluteRoot, "git", "status", "--porcelain", "-z", "--untracked-files=all")
	if err != nil {
		return nil, fmt.Errorf("verify: git status: %w", err)
	}
	if status.exitCode != 0 {
		return nil, fmt.Errorf("verify: git status failed: %s", strings.TrimSpace(status.output))
	}
	var patterns []string
	for _, repoPath := range statusPaths(status.output) {
		pattern, ok := packagePattern(absoluteRoot, filepath.Join(topLevel, filepath.FromSlash(repoPath)))
		if ok {
			patterns = append(patterns, pattern)
		}
	}
	return sortedUnique(patterns), nil
}

// gitTopLevel returns the resolved top directory of the work tree holding dir.
func gitTopLevel(ctx context.Context, dir string) (string, error) {
	result, err := runCommand(ctx, dir, "git", "rev-parse", "--show-toplevel")
	if err != nil {
		return "", fmt.Errorf("verify: git rev-parse: %w", err)
	}
	if result.exitCode != 0 {
		return "", fmt.Errorf("verify: %s is not inside a git work tree: %s", dir, strings.TrimSpace(result.output))
	}
	return realPath(strings.TrimSpace(result.output))
}

// statusPaths extracts every path from `git status --porcelain -z`, including the original side of renames.
func statusPaths(output string) []string {
	entries := strings.Split(output, "\x00")
	var paths []string
	for index := 0; index < len(entries); index++ {
		entry := entries[index]
		if len(entry) < 4 {
			continue
		}
		paths = append(paths, entry[3:])
		// A rename or copy is followed by an extra entry holding the original path.
		if entry[0] == 'R' || entry[0] == 'C' || entry[1] == 'R' || entry[1] == 'C' {
			if index+1 < len(entries) {
				paths = append(paths, entries[index+1])
				index++
			}
		}
	}
	return paths
}

// packagePattern turns a changed file into "./dir" or ".", or reports false when it does not count.
func packagePattern(root, file string) (string, bool) {
	if !strings.HasSuffix(file, ".go") {
		return "", false
	}
	directory := filepath.Dir(file)
	relative, err := filepath.Rel(root, directory)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return "", false
	}
	for _, element := range strings.Split(filepath.ToSlash(relative), "/") {
		if element == "testdata" || element == "vendor" {
			return "", false
		}
	}
	// A deleted file only counts while its directory still exists.
	if info, err := os.Stat(directory); err != nil || !info.IsDir() {
		return "", false
	}
	if relative == "." {
		return ".", true
	}
	return "./" + filepath.ToSlash(relative), true
}

// realPath makes path absolute and resolves symlinks so it compares equal to what git prints.
func realPath(path string) (string, error) {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	return filepath.EvalSymlinks(absolute)
}
