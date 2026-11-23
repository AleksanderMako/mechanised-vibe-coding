package deadcode

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// deadAtRevision runs Dead on the module at root as it was at ref, or as it stands when ref is empty.
func deadAtRevision(ctx context.Context, root, ref string, opts Options) ([]Func, error) {
	if ref == "" {
		return Dead(ctx, root, opts)
	}
	absoluteRoot, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("deadcode: resolve root: %w", err)
	}
	if _, err := runGit(ctx, absoluteRoot, "rev-parse", "--verify", "--quiet", ref+"^{commit}"); err != nil {
		return nil, fmt.Errorf("deadcode: revision %q does not resolve: %w", ref, err)
	}
	// The module may sit in a subdirectory of the repository, so find the same place in the worktree.
	topLevel, err := runGit(ctx, absoluteRoot, "rev-parse", "--show-toplevel")
	if err != nil {
		return nil, fmt.Errorf("deadcode: find repository root: %w", err)
	}
	moduleInRepository, err := filepath.Rel(resolveSymlinks(topLevel), resolveSymlinks(absoluteRoot))
	if err != nil {
		return nil, fmt.Errorf("deadcode: locate module in repository: %w", err)
	}

	worktree, err := os.MkdirTemp("", "deadcode-worktree-")
	if err != nil {
		return nil, fmt.Errorf("deadcode: create temporary directory: %w", err)
	}
	defer removeWorktree(ctx, absoluteRoot, worktree)
	if _, err := runGit(ctx, absoluteRoot, "worktree", "add", "--detach", worktree, ref); err != nil {
		return nil, fmt.Errorf("deadcode: check out %q: %w", ref, err)
	}
	return Dead(ctx, filepath.Join(worktree, moduleInRepository), opts)
}

// removeWorktree deletes the temporary worktree and its git bookkeeping, even when ctx was cancelled.
func removeWorktree(ctx context.Context, repository, worktree string) {
	cleanupContext := context.WithoutCancel(ctx)
	if _, err := runGit(cleanupContext, repository, "worktree", "remove", "--force", worktree); err != nil {
		// The add may have failed halfway, so drop whatever is left and let git forget it.
		_ = os.RemoveAll(worktree)
		_, _ = runGit(cleanupContext, repository, "worktree", "prune")
	}
}

// runGit runs git with args in dir and returns its trimmed standard output.
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
		message := strings.TrimSpace(stderr.String())
		if message == "" {
			return "", fmt.Errorf("git %s: %w", strings.Join(args, " "), err)
		}
		return "", fmt.Errorf("git %s: %w: %s", strings.Join(args, " "), err, message)
	}
	return strings.TrimSpace(stdout.String()), nil
}
