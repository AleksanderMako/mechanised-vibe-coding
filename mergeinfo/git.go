package mergeinfo

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
)

// runGit runs git with args in dir using the inherited environment and returns stdout.
// A non-zero exit becomes an error carrying stderr so the caller sees why git refused.
func runGit(ctx context.Context, dir string, args ...string) (string, error) {
	command := exec.CommandContext(ctx, "git", args...)
	command.Dir = dir
	var stdout, stderr bytes.Buffer
	command.Stdout = &stdout
	command.Stderr = &stderr
	if err := command.Run(); err != nil {
		if ctx.Err() != nil {
			return "", ctx.Err()
		}
		return "", fmt.Errorf("git %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(stderr.String()))
	}
	return stdout.String(), nil
}
