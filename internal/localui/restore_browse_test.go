package localui

import (
	"net/http"
	"os"
	"path/filepath"
	"testing"
)

func TestBrowseLocalDirectoryListsDirsBeforeFiles(t *testing.T) {
	home := t.TempDir()
	if err := os.Mkdir(filepath.Join(home, "zeta"), 0o700); err != nil {
		t.Fatalf("mkdir zeta: %v", err)
	}
	if err := os.Mkdir(filepath.Join(home, "Alpha"), 0o700); err != nil {
		t.Fatalf("mkdir alpha: %v", err)
	}
	if err := os.WriteFile(filepath.Join(home, "beta.txt"), []byte("beta"), 0o600); err != nil {
		t.Fatalf("write beta: %v", err)
	}
	if err := os.WriteFile(filepath.Join(home, "Gamma.txt"), []byte("gamma"), 0o600); err != nil {
		t.Fatalf("write gamma: %v", err)
	}

	resolvedHome, err := filepath.EvalSymlinks(home)
	if err != nil {
		t.Fatalf("resolve home: %v", err)
	}

	result, browseErr := browseLocalDirectory(home, resolvedHome, "")
	if browseErr != nil {
		t.Fatalf("browse error: %+v", browseErr)
	}
	if result.Path != resolvedHome {
		t.Fatalf("path = %q, want %q", result.Path, resolvedHome)
	}
	if result.Parent != "" {
		t.Fatalf("parent = %q, want empty", result.Parent)
	}

	got := entryNames(result.Entries)
	want := []string{"Alpha", "zeta", "beta.txt", "Gamma.txt"}
	if len(got) != len(want) {
		t.Fatalf("entries = %v", got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("entries = %v, want %v", got, want)
		}
	}
}

func TestBrowseLocalDirectoryRejectsPathOutsideHome(t *testing.T) {
	root := t.TempDir()
	home := filepath.Join(root, "home")
	outside := filepath.Join(root, "outside")
	if err := os.Mkdir(home, 0o700); err != nil {
		t.Fatalf("mkdir home: %v", err)
	}
	if err := os.Mkdir(outside, 0o700); err != nil {
		t.Fatalf("mkdir outside: %v", err)
	}

	resolvedHome, err := filepath.EvalSymlinks(home)
	if err != nil {
		t.Fatalf("resolve home: %v", err)
	}

	result, browseErr := browseLocalDirectory(home, resolvedHome, outside)
	if result != nil {
		t.Fatalf("result = %+v, want nil", result)
	}
	if browseErr == nil {
		t.Fatal("browseErr = nil")
	}
	if browseErr.Status != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", browseErr.Status)
	}
	if browseErr.Code != "RESTORE_BROWSE_LOCAL_FORBIDDEN" {
		t.Fatalf("code = %q", browseErr.Code)
	}
}

func entryNames(entries []browseEntry) []string {
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		names = append(names, entry.Name)
	}
	return names
}
