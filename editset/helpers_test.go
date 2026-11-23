package editset_test

import (
	"io/fs"
	"os"
	"path/filepath"
	"testing"
)

// copyFixture copies testdata/<name> into a fresh temp dir and returns its symlink free absolute path.
func copyFixture(t *testing.T, name string) string {
	t.Helper()
	source := filepath.Join("testdata", name)
	target := filepath.Join(t.TempDir(), name)
	err := filepath.WalkDir(source, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		relative, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}
		destination := filepath.Join(target, relative)
		if entry.IsDir() {
			return os.MkdirAll(destination, 0o755)
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		content, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(destination, content, info.Mode().Perm())
	})
	if err != nil {
		t.Fatalf("copy fixture %s: %v", name, err)
	}
	// Temp dirs on macOS sit behind a symlink; resolving it keeps path comparisons stable.
	resolved, err := filepath.EvalSymlinks(target)
	if err != nil {
		t.Fatalf("resolve fixture path: %v", err)
	}
	return resolved
}

// readFile returns the content of path or fails the test.
func readFile(t *testing.T, path string) string {
	t.Helper()
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(content)
}

// snapshotFiles reads every listed file so a test can later prove they are byte identical.
func snapshotFiles(t *testing.T, paths ...string) map[string]string {
	t.Helper()
	snapshot := make(map[string]string, len(paths))
	for _, path := range paths {
		snapshot[path] = readFile(t, path)
	}
	return snapshot
}

// assertUnchanged fails when any file differs from its snapshot.
func assertUnchanged(t *testing.T, snapshot map[string]string) {
	t.Helper()
	for path, before := range snapshot {
		if after := readFile(t, path); after != before {
			t.Errorf("%s changed:\nbefore:\n%s\nafter:\n%s", path, before, after)
		}
	}
}
