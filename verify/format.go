package verify

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

// unformattedFiles lists the .go files of the patterns that gofmt would change, relative to root and sorted.
// failed is true when gofmt itself reported a problem such as a syntax error.
func unformattedFiles(ctx context.Context, root string, patterns []string) (files []string, failed bool, output string, err error) {
	goFiles, err := patternGoFiles(root, patterns)
	if err != nil || len(goFiles) == 0 {
		return nil, false, "", err
	}
	result, err := runCommand(ctx, root, gofmtPath(ctx), append([]string{"-l"}, goFiles...)...)
	if err != nil {
		return nil, false, "", err
	}
	if result.exitCode != 0 {
		return nil, true, result.output, nil
	}
	for _, line := range strings.Split(result.output, "\n") {
		if line = strings.TrimSpace(line); line != "" {
			files = append(files, filepath.ToSlash(filepath.Clean(line)))
		}
	}
	sort.Strings(files)
	return files, false, "", nil
}

// patternGoFiles expands package patterns into .go file paths relative to root, without duplicates.
func patternGoFiles(root string, patterns []string) ([]string, error) {
	seen := map[string]bool{}
	var files []string
	add := func(path string) error {
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		if !seen[relative] {
			seen[relative] = true
			files = append(files, relative)
		}
		return nil
	}
	for _, pattern := range patterns {
		base, recursive := strings.CutSuffix(pattern, "...")
		directory := filepath.Join(root, filepath.FromSlash(strings.TrimSuffix(base, "/")))
		info, err := os.Stat(directory)
		if err != nil || !info.IsDir() {
			// A pattern without a directory has nothing to format; the build step reports it.
			continue
		}
		if recursive {
			err = walkGoFiles(directory, add)
		} else {
			err = listGoFiles(directory, add)
		}
		if err != nil {
			return nil, fmt.Errorf("verify: listing %s: %w", pattern, err)
		}
	}
	sort.Strings(files)
	return files, nil
}

// listGoFiles calls add for every .go file directly inside directory.
func listGoFiles(directory string, add func(string) error) error {
	entries, err := os.ReadDir(directory)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if !entry.IsDir() && isGoFile(entry.Name()) {
			if err := add(filepath.Join(directory, entry.Name())); err != nil {
				return err
			}
		}
	}
	return nil
}

// walkGoFiles calls add for every .go file under directory, skipping vendor, testdata and hidden directories.
func walkGoFiles(directory string, add func(string) error) error {
	return filepath.WalkDir(directory, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			name := entry.Name()
			if path != directory && (name == "vendor" || name == "testdata" || strings.HasPrefix(name, ".")) {
				return filepath.SkipDir
			}
			return nil
		}
		if isGoFile(entry.Name()) {
			return add(path)
		}
		return nil
	})
}

// isGoFile matches what gofmt itself treats as a Go source file.
func isGoFile(name string) bool {
	return strings.HasSuffix(name, ".go") && !strings.HasPrefix(name, ".")
}

// gofmtPath prefers the gofmt of the go toolchain in use, because PATH may hold a different version.
func gofmtPath(ctx context.Context) string {
	result, err := runCommand(ctx, "", "go", "env", "GOROOT")
	if err == nil && result.exitCode == 0 {
		candidate := filepath.Join(strings.TrimSpace(result.output), "bin", "gofmt")
		if _, statErr := os.Stat(candidate); statErr == nil {
			return candidate
		}
	}
	if path, err := exec.LookPath("gofmt"); err == nil {
		return path
	}
	return "gofmt"
}
