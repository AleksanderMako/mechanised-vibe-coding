package verify

import (
	"bytes"
	"context"
	"errors"
	"os/exec"
	"sort"
)

// commandResult is what one external command produced.
type commandResult struct {
	output   string // stdout and stderr interleaved
	exitCode int    // 0 on success, -1 when the command did not start or was killed
}

// runCommand runs name with args in dir using the inherited environment.
// The error is only set when the command could not run at all; a non-zero exit is not an error.
func runCommand(ctx context.Context, dir, name string, args ...string) (commandResult, error) {
	command := exec.CommandContext(ctx, name, args...)
	command.Dir = dir
	var combined bytes.Buffer
	command.Stdout = &combined
	command.Stderr = &combined
	runErr := command.Run()
	result := commandResult{output: combined.String()}
	if runErr == nil {
		return result, nil
	}
	if ctx.Err() != nil {
		// A killed command looks like an ordinary failure, so report the cancellation instead.
		result.exitCode = -1
		return result, ctx.Err()
	}
	var exitErr *exec.ExitError
	if errors.As(runErr, &exitErr) {
		result.exitCode = exitErr.ExitCode()
		return result, nil
	}
	result.exitCode = -1
	return result, runErr
}

// sortedUnique returns a sorted copy of values without duplicates or empty strings.
func sortedUnique(values []string) []string {
	seen := map[string]bool{}
	unique := []string{}
	for _, value := range values {
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		unique = append(unique, value)
	}
	sort.Strings(unique)
	return unique
}
