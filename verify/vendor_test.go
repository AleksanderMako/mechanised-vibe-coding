package verify_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/AleksanderMako/mechanised-vibe-coding/verify"
)

func TestRule19_NoVendorDirectoryReturnsFalse(t *testing.T) {
	t.Parallel()
	root := copyFixture(t, "module")
	changed, err := verify.SyncVendor(context.Background(), root)
	if err != nil {
		t.Fatalf("SyncVendor: %v", err)
	}
	if changed {
		t.Error("changed: want false without a vendor directory")
	}
	if _, err := os.Stat(filepath.Join(root, "vendor")); !os.IsNotExist(err) {
		t.Errorf("SyncVendor created a vendor directory: %v", err)
	}
}

func TestRule19_NoVendorDirectoryRunsNothingEvenWithABrokenModule(t *testing.T) {
	t.Parallel()
	root := copyFixture(t, "module")
	// Any go command would fail on this go.mod, so success proves nothing ran.
	writeFile(t, root, "go.mod", "module example.com/fixture\n\ngo 1.26\n\nbogus directive\n")
	changed, err := verify.SyncVendor(context.Background(), root)
	if err != nil {
		t.Fatalf("SyncVendor: want no error when there is no vendor directory, got %v", err)
	}
	if changed {
		t.Error("changed: want false without a vendor directory")
	}
}

func TestRule20_ConsistentVendorIsLeftAlone(t *testing.T) {
	t.Parallel()
	root := copyFixture(t, "module")
	// A standard library only module is consistent with an empty modules.txt.
	writeFile(t, root, "vendor/modules.txt", "")
	changed, err := verify.SyncVendor(context.Background(), root)
	if err != nil {
		t.Fatalf("SyncVendor: %v", err)
	}
	if changed {
		t.Error("changed: want false when vendor/modules.txt matches go.mod")
	}
	if _, err := os.Stat(filepath.Join(root, "vendor", "modules.txt")); err != nil {
		t.Errorf("vendor/modules.txt should be untouched: %v", err)
	}
}

func TestRule20_21_InconsistentVendoringIsSynced(t *testing.T) {
	t.Parallel()
	root := copyFixture(t, "module")
	writeFile(t, root, "vendor/modules.txt", staleModulesTxt)
	changed, err := verify.SyncVendor(context.Background(), root)
	if err != nil {
		t.Fatalf("SyncVendor: %v", err)
	}
	if !changed {
		t.Error("changed: want true after syncing a stale vendor/modules.txt")
	}
	// After go mod vendor the module graph and vendor agree again.
	runGo(t, root, "list", "-m")
	runGo(t, root, "build", "./...")
}

func TestRule20_21_MissingModulesTxtIsDrift(t *testing.T) {
	t.Parallel()
	root := copyFixture(t, "module")
	writeFile(t, root, "vendor/example.com/dep/dep.go", goSource("dep"))
	changed, err := verify.SyncVendor(context.Background(), root)
	if err != nil {
		t.Fatalf("SyncVendor: %v", err)
	}
	if !changed {
		t.Error("changed: want true when vendor/modules.txt is missing")
	}
}

func TestRule20_OtherGoListFailureIsAnError(t *testing.T) {
	t.Parallel()
	root := copyFixture(t, "module")
	writeFile(t, root, "go.mod", "module example.com/fixture\n\ngo 1.26\n\nbogus directive\n")
	writeFile(t, root, "vendor/modules.txt", "")
	if _, err := verify.SyncVendor(context.Background(), root); err == nil {
		t.Fatal("SyncVendor on a broken go.mod: want an error, got nil")
	}
}

func TestRule21_FailingGoModVendorIsAnErrorWithItsOutput(t *testing.T) {
	t.Parallel()
	root := copyFixture(t, "module")
	// The requirement points at a replacement directory that does not exist, so go mod vendor fails.
	writeFile(t, root, "go.mod", "module example.com/fixture\n\ngo 1.26\n\nrequire example.com/gone v0.0.0\n\nreplace example.com/gone => ./missing-replacement\n")
	writeFile(t, root, "lib/uses/uses.go", "// Package uses imports the missing module.\npackage uses\n\nimport \"example.com/gone\"\n\n// Value reads from the missing module.\nvar Value = gone.Value\n")
	writeFile(t, root, "vendor/modules.txt", "")
	_, err := verify.SyncVendor(context.Background(), root)
	if err == nil {
		t.Fatal("SyncVendor: want an error when go mod vendor fails, got nil")
	}
	if !strings.Contains(err.Error(), "missing-replacement") {
		t.Errorf("error should carry the go mod vendor output naming missing-replacement, got %v", err)
	}
}
