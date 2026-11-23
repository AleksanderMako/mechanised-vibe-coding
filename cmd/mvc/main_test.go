package main_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// mvcBinary is the command under test, built once by TestMain.
var mvcBinary string

// commandTimeout bounds every subprocess so a hanging command fails the test instead of the run.
const commandTimeout = 2 * time.Minute

func TestMain(m *testing.M) {
	os.Exit(buildAndRun(m))
}

// buildAndRun builds the binary into a temporary directory, runs the tests and removes the directory.
func buildAndRun(m *testing.M) int {
	buildDir, err := os.MkdirTemp("", "mvc-test-bin")
	if err != nil {
		fmt.Fprintln(os.Stderr, "create build dir:", err)
		return 1
	}
	defer os.RemoveAll(buildDir)

	packageDir, err := os.Getwd()
	if err != nil {
		fmt.Fprintln(os.Stderr, "find package dir:", err)
		return 1
	}
	ctx, cancel := context.WithTimeout(context.Background(), commandTimeout)
	defer cancel()
	mvcBinary = filepath.Join(buildDir, "mvc")
	build := exec.CommandContext(ctx, "go", "build", "-o", mvcBinary, ".")
	build.Dir = packageDir
	build.Env = os.Environ()
	if output, err := build.CombinedOutput(); err != nil {
		fmt.Fprintf(os.Stderr, "build mvc: %v\n%s", err, output)
		return 1
	}
	return m.Run()
}

// runResult is what one run of the binary produced.
type runResult struct {
	stdout   string
	stderr   string
	exitCode int
}

// describe renders a result for failure messages.
func (result runResult) describe() string {
	return fmt.Sprintf("exit %d\n--- stdout ---\n%s\n--- stderr ---\n%s", result.exitCode, result.stdout, result.stderr)
}

// runMVC runs the binary in dir with args and no stdin.
func runMVC(t *testing.T, dir string, args ...string) runResult {
	t.Helper()
	return runMVCWithStdin(t, dir, "", args...)
}

// runMVCWithStdin runs the binary in dir with args, feeding stdin.
func runMVCWithStdin(t *testing.T, dir, stdin string, args ...string) runResult {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), commandTimeout)
	defer cancel()
	command := exec.CommandContext(ctx, mvcBinary, args...)
	command.Dir = dir
	// PWD must match Dir so the child's current directory is reported as the fixture path.
	command.Env = append(os.Environ(), "PWD="+dir)
	command.Stdin = strings.NewReader(stdin)
	var stdout, stderr bytes.Buffer
	command.Stdout = &stdout
	command.Stderr = &stderr
	err := command.Run()
	result := runResult{stdout: stdout.String(), stderr: stderr.String()}
	var exitErr *exec.ExitError
	switch {
	case err == nil:
		result.exitCode = 0
	case errors.As(err, &exitErr) && exitErr.ExitCode() >= 0:
		result.exitCode = exitErr.ExitCode()
	default:
		t.Fatalf("run mvc %s: %v\n%s", strings.Join(args, " "), err, result.describe())
	}
	return result
}

// realTempDir returns a temp dir with symlinks resolved, so paths printed by mvc compare equal.
func realTempDir(t *testing.T) string {
	t.Helper()
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatalf("resolve temp dir: %v", err)
	}
	return dir
}

// copyFixture copies testdata/<name> into a fresh temp dir and returns the copy's path.
func copyFixture(t *testing.T, name string) string {
	t.Helper()
	source := filepath.Join("testdata", name)
	target := realTempDir(t)
	err := filepath.WalkDir(source, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
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
	return target
}

// copyModule copies the shared Go module fixture.
func copyModule(t *testing.T) string {
	t.Helper()
	return copyFixture(t, "module")
}

// writeFile writes content to dir/relative, creating parent directories.
func writeFile(t *testing.T, dir, relative, content string) string {
	t.Helper()
	path := filepath.Join(dir, filepath.FromSlash(relative))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

// readFile returns the content of dir/relative.
func readFile(t *testing.T, dir, relative string) string {
	t.Helper()
	content, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(relative)))
	if err != nil {
		t.Fatal(err)
	}
	return string(content)
}

// nonEmptyLines splits output into lines and drops blank ones.
func nonEmptyLines(output string) []string {
	var lines []string
	for _, line := range strings.Split(output, "\n") {
		if strings.TrimSpace(line) != "" {
			lines = append(lines, line)
		}
	}
	return lines
}

// expectExit fails the test when the exit code is not want.
func expectExit(t *testing.T, result runResult, want int) {
	t.Helper()
	if result.exitCode != want {
		t.Fatalf("want exit %d, got %s", want, result.describe())
	}
}

// expectErrorReported checks rule 4 for an error: exit 2, nothing on stdout, and an `mvc: ` line on stderr.
func expectErrorReported(t *testing.T, result runResult) {
	t.Helper()
	expectExit(t, result, 2)
	if strings.TrimSpace(result.stdout) != "" {
		t.Errorf("error run printed to stdout:\n%s", result.describe())
	}
	for _, line := range strings.Split(result.stderr, "\n") {
		if strings.HasPrefix(line, "mvc: ") {
			return
		}
	}
	t.Errorf("stderr has no line starting with %q:\n%s", "mvc: ", result.describe())
}

// expectOrdinaryError checks an ordinary error: like expectErrorReported, and the first stderr line is the error.
func expectOrdinaryError(t *testing.T, result runResult) {
	t.Helper()
	expectErrorReported(t, result)
	if !strings.HasPrefix(result.stderr, "mvc: ") {
		t.Errorf("first stderr line does not start with %q:\n%s", "mvc: ", result.describe())
	}
}

// decodeJSON checks that stdout is valid JSON and nothing else, then decodes it into target.
func decodeJSON(t *testing.T, result runResult, target any) {
	t.Helper()
	if !json.Valid([]byte(result.stdout)) {
		t.Fatalf("stdout is not valid JSON:\n%s", result.describe())
	}
	decoder := json.NewDecoder(strings.NewReader(result.stdout))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		t.Fatalf("decode stdout into %T: %v\n%s", target, err, result.describe())
	}
}

// emptyDir returns a directory without go.mod, which every root taking package treats as an error.
func emptyDir(t *testing.T) string {
	t.Helper()
	return realTempDir(t)
}

// runGo runs the go tool in dir and fails the test when it fails.
func runGo(t *testing.T, dir string, args ...string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), commandTimeout)
	defer cancel()
	command := exec.CommandContext(ctx, "go", args...)
	command.Dir = dir
	command.Env = os.Environ()
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("go %s: %v\n%s", strings.Join(args, " "), err, output)
	}
}

// gitIdentity pins author and signing so the user's global git config cannot interfere.
var gitIdentity = []string{
	"-c", "user.name=t",
	"-c", "user.email=t@example.com",
	"-c", "commit.gpgsign=false",
	"-c", "core.autocrlf=false",
	"-c", "merge.conflictStyle=merge",
}

// gitCommand builds a git command in dir with the pinned identity in front of args.
func gitCommand(ctx context.Context, dir string, args ...string) *exec.Cmd {
	command := exec.CommandContext(ctx, "git", append(append([]string{}, gitIdentity...), args...)...)
	command.Dir = dir
	command.Env = append(os.Environ(), "GIT_EDITOR=true")
	return command
}

// runGit runs git in dir and fails the test on a non-zero exit.
func runGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), commandTimeout)
	defer cancel()
	output, err := gitCommand(ctx, dir, args...).CombinedOutput()
	if err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, output)
	}
	return strings.TrimSpace(string(output))
}
