package localui

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"xentz-agent/internal/backup"
	"xentz-agent/internal/config"
)

type restoreRequest struct {
	Type             string `json:"type"` // file|folder|snapshot
	SnapshotID       string `json:"snapshot_id"`
	Path             string `json:"path,omitempty"`
	Target           string `json:"target"`
	ConfirmOverwrite bool   `json:"confirm_overwrite,omitempty"`
}

type browseEntry struct {
	Path  string `json:"path"`
	Name  string `json:"name"`
	Type  string `json:"type"` // file|dir
	IsDir bool   `json:"is_dir"`
}

func validateRestoreRequest(req restoreRequest) ([]string, bool) {
	var errs []string
	confirmRequired := false
	if req.Type != "file" && req.Type != "folder" && req.Type != "snapshot" {
		errs = append(errs, "type must be file, folder, or snapshot")
	}
	if strings.TrimSpace(req.Target) == "" {
		errs = append(errs, "target is required")
	} else if !filepath.IsAbs(req.Target) {
		errs = append(errs, "target must be an absolute path")
	}
	if req.Type == "file" || req.Type == "folder" {
		if strings.TrimSpace(req.Path) == "" {
			errs = append(errs, "path is required for file/folder restore")
		}
	}
	if isDangerousTarget(req.Target) {
		errs = append(errs, "target path is unsafe")
	}
	if st, err := os.Stat(req.Target); err == nil {
		if req.Type == "file" && st.IsDir() {
			errs = append(errs, "file restore target cannot be an existing directory")
		}
		if (req.Type == "folder" || req.Type == "snapshot") && !st.IsDir() {
			errs = append(errs, "folder/snapshot restore target cannot be an existing file")
		}
		if req.Type == "file" && !st.IsDir() {
			confirmRequired = true
		}
		if (req.Type == "folder" || req.Type == "snapshot") && st.IsDir() {
			if hasEntries, e := dirHasEntries(req.Target); e == nil && hasEntries {
				confirmRequired = true
			}
		}
	}
	return errs, confirmRequired
}

func isDangerousTarget(target string) bool {
	clean := filepath.Clean(target)
	return clean == "/" || clean == "/System" || clean == "/Library" || clean == "/usr"
}

func dirHasEntries(path string) (bool, error) {
	f, err := os.Open(path)
	if err != nil {
		return false, err
	}
	defer f.Close()
	_, err = f.Readdirnames(1)
	if err == io.EOF {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

func tailText(s string, max int) string {
	if max <= 0 || len(s) <= max {
		return s
	}
	return s[len(s)-max:]
}

func openHint(target string) string {
	if runtime.GOOS == "darwin" {
		return "open \"" + target + "\""
	}
	return ""
}

func isSubPath(path, root string) bool {
	p := filepath.Clean(path)
	r := filepath.Clean(root)
	if p == r {
		return true
	}
	rel, err := filepath.Rel(r, p)
	if err != nil {
		return false
	}
	return rel != "." && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

func writeRestoreError(w http.ResponseWriter, status int, code, message string) {
	w.WriteHeader(status)
	writeJSON(w, map[string]interface{}{
		"ok":      false,
		"code":    code,
		"message": message,
	})
}

func logRestoreDetail(code string, err error) {
	if err == nil {
		return
	}
	fmt.Printf("local-ui %s: %v\n", code, err)
}

func resticEnvForUI(cfgPath string) ([]string, error) {
	cfg, err := config.Read(cfgPath)
	if err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}
	if cfg.Restic.Repository == "" {
		return nil, fmt.Errorf("restic.repository is empty in config")
	}
	var pw string
	if cfg.Restic.PasswordFile == "" {
		pw, err = config.GetResticPassword(cfg)
		if err != nil {
			return nil, fmt.Errorf("resolve restic password: %w", err)
		}
	}
	return append(os.Environ(), backup.ResticEnv(cfg, pw)...), nil
}
