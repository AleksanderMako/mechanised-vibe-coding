package verify

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// SyncVendor fixes vendor drift. Without a vendor directory it returns false.
// With one, it runs `go mod vendor` when the module graph and vendor/modules.txt
// disagree, and reports whether it ran.
func SyncVendor(ctx context.Context, root string) (changed bool, err error) {
	if root == "" {
		return false, fmt.Errorf("verify: root is empty")
	}
	vendorInfo, err := os.Stat(filepath.Join(root, "vendor"))
	if err != nil || !vendorInfo.IsDir() {
		return false, nil
	}
	drifted, err := vendorDrifted(ctx, root)
	if err != nil || !drifted {
		return false, err
	}
	result, err := runCommand(ctx, root, "go", "mod", "vendor")
	if err != nil {
		return false, fmt.Errorf("verify: go mod vendor: %w", err)
	}
	if result.exitCode != 0 {
		return false, fmt.Errorf("verify: go mod vendor failed:\n%s", result.output)
	}
	return true, nil
}

// vendorDrifted reports whether vendor/modules.txt is missing or disagrees with go.mod.
func vendorDrifted(ctx context.Context, root string) (bool, error) {
	_, err := os.Stat(filepath.Join(root, "vendor", "modules.txt"))
	if errors.Is(err, fs.ErrNotExist) {
		return true, nil
	}
	if err != nil {
		return false, fmt.Errorf("verify: %w", err)
	}
	result, err := runCommand(ctx, root, "go", "list", "-m", "all")
	if err != nil {
		return false, fmt.Errorf("verify: go list -m all: %w", err)
	}
	if result.exitCode == 0 {
		return false, nil
	}
	if strings.Contains(result.output, "inconsistent vendoring") {
		return true, nil
	}
	// In vendor mode go refuses to list "all", but only after the vendor consistency check passed.
	if strings.Contains(result.output, "can't compute 'all' using the vendor directory") {
		return false, nil
	}
	return false, fmt.Errorf("verify: go list -m all failed:\n%s", result.output)
}
