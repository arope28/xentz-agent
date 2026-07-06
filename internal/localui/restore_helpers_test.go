package localui

import (
	"os"
	"path/filepath"
	"testing"
)

func TestValidateRestoreRequestRequiresValidTypeTargetAndPath(t *testing.T) {
	errs, confirmRequired := validateRestoreRequest(restoreRequest{
		Type:   "other",
		Target: "relative/path",
	})

	assertHasError(t, errs, "type must be file, folder, or snapshot")
	assertHasError(t, errs, "target must be an absolute path")
	if confirmRequired {
		t.Fatal("confirmRequired = true, want false")
	}

	errs, confirmRequired = validateRestoreRequest(restoreRequest{
		Type:   "file",
		Target: filepath.Join(t.TempDir(), "restore.txt"),
	})
	assertHasError(t, errs, "path is required for file/folder restore")
	if confirmRequired {
		t.Fatal("confirmRequired = true, want false")
	}
}

func TestValidateRestoreRequestFileTargetRules(t *testing.T) {
	dir := t.TempDir()
	fileTarget := filepath.Join(dir, "restored.txt")
	if err := os.WriteFile(fileTarget, []byte("existing"), 0o600); err != nil {
		t.Fatalf("write target file: %v", err)
	}

	errs, confirmRequired := validateRestoreRequest(restoreRequest{
		Type:   "file",
		Path:   "/source/file.txt",
		Target: fileTarget,
	})
	if len(errs) != 0 {
		t.Fatalf("errs = %v, want none", errs)
	}
	if !confirmRequired {
		t.Fatal("confirmRequired = false, want true for existing file")
	}

	errs, confirmRequired = validateRestoreRequest(restoreRequest{
		Type:   "file",
		Path:   "/source/file.txt",
		Target: dir,
	})
	assertHasError(t, errs, "file restore target cannot be an existing directory")
	if confirmRequired {
		t.Fatal("confirmRequired = true, want false")
	}
}

func TestValidateRestoreRequestFolderTargetRules(t *testing.T) {
	dir := t.TempDir()
	emptyDir := filepath.Join(dir, "empty")
	nonEmptyDir := filepath.Join(dir, "non-empty")
	fileTarget := filepath.Join(dir, "file.txt")
	if err := os.Mkdir(emptyDir, 0o700); err != nil {
		t.Fatalf("mkdir empty: %v", err)
	}
	if err := os.Mkdir(nonEmptyDir, 0o700); err != nil {
		t.Fatalf("mkdir non-empty: %v", err)
	}
	if err := os.WriteFile(filepath.Join(nonEmptyDir, "child.txt"), []byte("child"), 0o600); err != nil {
		t.Fatalf("write child: %v", err)
	}
	if err := os.WriteFile(fileTarget, []byte("existing"), 0o600); err != nil {
		t.Fatalf("write file target: %v", err)
	}

	errs, confirmRequired := validateRestoreRequest(restoreRequest{
		Type:   "folder",
		Path:   "/source/folder",
		Target: emptyDir,
	})
	if len(errs) != 0 {
		t.Fatalf("errs = %v, want none", errs)
	}
	if confirmRequired {
		t.Fatal("confirmRequired = true, want false for empty directory")
	}

	errs, confirmRequired = validateRestoreRequest(restoreRequest{
		Type:   "snapshot",
		Target: nonEmptyDir,
	})
	if len(errs) != 0 {
		t.Fatalf("errs = %v, want none", errs)
	}
	if !confirmRequired {
		t.Fatal("confirmRequired = false, want true for non-empty directory")
	}

	errs, confirmRequired = validateRestoreRequest(restoreRequest{
		Type:   "folder",
		Path:   "/source/folder",
		Target: fileTarget,
	})
	assertHasError(t, errs, "folder/snapshot restore target cannot be an existing file")
	if confirmRequired {
		t.Fatal("confirmRequired = true, want false")
	}
}

func TestValidateRestoreRequestRejectsDangerousTarget(t *testing.T) {
	errs, _ := validateRestoreRequest(restoreRequest{
		Type:   "snapshot",
		Target: "/",
	})
	assertHasError(t, errs, "target path is unsafe")
}

func TestIsSubPath(t *testing.T) {
	root := filepath.Join(string(filepath.Separator), "tmp", "home")

	if !isSubPath(root, root) {
		t.Fatal("root should be a subpath of itself")
	}
	if !isSubPath(filepath.Join(root, "Documents", "file.txt"), root) {
		t.Fatal("child path should be inside root")
	}
	if isSubPath(filepath.Join(string(filepath.Separator), "tmp", "home-other"), root) {
		t.Fatal("prefix sibling should not be inside root")
	}
	if isSubPath(filepath.Join(string(filepath.Separator), "tmp"), root) {
		t.Fatal("parent should not be inside root")
	}
}

func assertHasError(t *testing.T, errs []string, want string) {
	t.Helper()

	for _, err := range errs {
		if err == want {
			return
		}
	}
	t.Fatalf("errs = %v, want %q", errs, want)
}
