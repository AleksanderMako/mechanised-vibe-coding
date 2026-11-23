package verify_test

import (
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/AleksanderMako/machanised-vibe-coding/verify"
)

// copyFixture copies testdata/<name> into a fresh temp dir and returns its symlink free absolute path.
func copyFixture(t *testing.T, name string) string {
	t.Helper()
	return copyFixtureInto(t, name, t.TempDir())
}

// copyFixtureInto copies testdata/<name> to <parent>/<name> and returns its symlink free absolute path.
func copyFixtureInto(t *testing.T, name, parent string) string {
	t.Helper()
	source := filepath.Join("testdata", name)
	target := filepath.Join(parent, name)
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
		content, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(destination, content, 0o644)
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

// writeFile writes content to root/relative, creating parent directories.
func writeFile(t *testing.T, root, relative, content string) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(relative))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// removePath deletes root/relative and everything under it.
func removePath(t *testing.T, root, relative string) {
	t.Helper()
	if err := os.RemoveAll(filepath.Join(root, filepath.FromSlash(relative))); err != nil {
		t.Fatal(err)
	}
}

// runGit runs git in dir with a fixed identity so commits work on any machine.
func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	fullArgs := append([]string{
		"-c", "user.name=Verify Test",
		"-c", "user.email=verify@example.com",
		"-c", "commit.gpgsign=false",
		"-c", "init.defaultBranch=main",
	}, args...)
	command := exec.Command("git", fullArgs...)
	command.Dir = dir
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, output)
	}
}

// commitAll turns dir into a git repository holding one commit of everything in it.
func commitAll(t *testing.T, dir string) {
	t.Helper()
	runGit(t, dir, "init", "-q")
	runGit(t, dir, "add", "-A")
	runGit(t, dir, "commit", "-q", "--no-verify", "-m", "fixture")
}

// runGo runs the go tool in dir and fails the test when it exits non-zero.
func runGo(t *testing.T, dir string, args ...string) string {
	t.Helper()
	command := exec.Command("go", args...)
	command.Dir = dir
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("go %s: %v\n%s", strings.Join(args, " "), err, output)
	}
	return string(output)
}

// stepNamed returns the step called name or fails the test.
func stepNamed(t *testing.T, report verify.Report, name verify.StepName) verify.Step {
	t.Helper()
	for _, step := range report.Steps {
		if step.Name == name {
			return step
		}
	}
	t.Fatalf("report has no step %q: %+v", name, report.Steps)
	return verify.Step{}
}

// assertStepOrder checks the report has exactly one step per entry of verify.Order, in that order.
func assertStepOrder(t *testing.T, report verify.Report) {
	t.Helper()
	if len(report.Steps) != len(verify.Order) {
		t.Fatalf("want %d steps, got %d: %+v", len(verify.Order), len(report.Steps), report.Steps)
	}
	for index, name := range verify.Order {
		if report.Steps[index].Name != name {
			t.Errorf("step %d: want %q, got %q", index, name, report.Steps[index].Name)
		}
	}
}

// assertSkipped checks a step did not run and carries no result.
func assertSkipped(t *testing.T, step verify.Step) {
	t.Helper()
	if !step.Skipped {
		t.Errorf("step %s: want Skipped, got %+v", step.Name, step)
	}
	if step.OK {
		t.Errorf("step %s: Skipped step must have OK false", step.Name)
	}
	if step.Duration != 0 {
		t.Errorf("step %s: Skipped step must have zero Duration, got %v", step.Name, step.Duration)
	}
	if step.Output != "" {
		t.Errorf("step %s: Skipped step must have empty Output, got %q", step.Name, step.Output)
	}
}

// assertPassed checks a step ran, passed and took measurable time.
func assertPassed(t *testing.T, step verify.Step) {
	t.Helper()
	if step.Skipped || !step.OK {
		t.Errorf("step %s: want ran and passed, got Skipped=%v OK=%v Output:\n%s", step.Name, step.Skipped, step.OK, step.Output)
	}
	if step.Duration <= 0 {
		t.Errorf("step %s: ran but Duration is %v", step.Name, step.Duration)
	}
}

// assertFailed checks a step ran, failed, took measurable time and explained itself.
func assertFailed(t *testing.T, step verify.Step) {
	t.Helper()
	if step.Skipped || step.OK {
		t.Errorf("step %s: want ran and failed, got Skipped=%v OK=%v Output:\n%s", step.Name, step.Skipped, step.OK, step.Output)
	}
	if step.Duration <= 0 {
		t.Errorf("step %s: ran but Duration is %v", step.Name, step.Duration)
	}
	if strings.TrimSpace(step.Output) == "" {
		t.Errorf("step %s: failed with empty Output", step.Name)
	}
}

// waitForFile polls until path exists or the deadline passes.
func waitForFile(t *testing.T, path string, deadline time.Duration) bool {
	t.Helper()
	stop := time.Now().Add(deadline)
	for time.Now().Before(stop) {
		if _, err := os.Stat(path); err == nil {
			return true
		}
		time.Sleep(50 * time.Millisecond)
	}
	return false
}

// outputLines splits output into its non-empty lines.
func outputLines(output string) []string {
	var lines []string
	for _, line := range strings.Split(output, "\n") {
		if strings.TrimSpace(line) != "" {
			lines = append(lines, line)
		}
	}
	return lines
}
