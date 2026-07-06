package localui

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPlanRestoreDefaultsSnapshotAndFlagsConfirmation(t *testing.T) {
	target := filepath.Join(t.TempDir(), "restore-target")
	if err := os.Mkdir(target, 0o700); err != nil {
		t.Fatalf("mkdir target: %v", err)
	}
	if err := os.WriteFile(filepath.Join(target, "existing.txt"), []byte("existing"), 0o600); err != nil {
		t.Fatalf("write existing file: %v", err)
	}

	plan := planRestore(restoreRequest{
		Type:   "snapshot",
		Target: target,
	})

	if !plan.OK {
		t.Fatalf("plan errors = %v", plan.Errors)
	}
	if !plan.ConfirmRequired {
		t.Fatal("ConfirmRequired = false, want true")
	}
	if plan.Request.SnapshotID != "latest" {
		t.Fatalf("snapshot_id = %q, want latest", plan.Request.SnapshotID)
	}
}

func TestPlanRestoreReturnsValidationErrors(t *testing.T) {
	plan := planRestore(restoreRequest{
		Type:   "file",
		Target: "relative-target",
	})

	if plan.OK {
		t.Fatal("OK = true, want false")
	}
	assertHasError(t, plan.Errors, "target must be an absolute path")
	assertHasError(t, plan.Errors, "path is required for file/folder restore")
}
