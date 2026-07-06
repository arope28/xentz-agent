package localui

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

func (s *Server) handleRestoreSnapshots(cfgPath string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		env, err := resticEnvForUI(cfgPath)
		if err != nil {
			writeErr(w, err)
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), restoreTimeout)
		defer cancel()
		cmd := exec.CommandContext(ctx, "restic", "snapshots", "--json")
		cmd.Env = env
		var stdout bytes.Buffer
		var stderr bytes.Buffer
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr
		if err := cmd.Run(); err != nil {
			logRestoreDetail("RESTORE_SNAPSHOTS_FAILED", fmt.Errorf("%w (%s)", err, tailText(stderr.String(), 1024)))
			writeRestoreError(w, http.StatusInternalServerError, "RESTORE_SNAPSHOTS_FAILED", "failed to list snapshots")
			return
		}
		var parsed interface{}
		if err := json.Unmarshal(stdout.Bytes(), &parsed); err != nil {
			logRestoreDetail("RESTORE_SNAPSHOTS_PARSE_FAILED", err)
			writeRestoreError(w, http.StatusInternalServerError, "RESTORE_SNAPSHOTS_PARSE_FAILED", "failed to parse snapshots")
			return
		}
		writeJSON(w, parsed)
	}
}

func (s *Server) handleRestoreBrowseLocal() http.HandlerFunc {
	homeDir, err := os.UserHomeDir()
	if err != nil || strings.TrimSpace(homeDir) == "" {
		homeDir = "/tmp"
	}
	resolvedHome := homeDir
	if h, err := filepath.EvalSymlinks(homeDir); err == nil && strings.TrimSpace(h) != "" {
		resolvedHome = h
	}
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		result, browseErr := browseLocalDirectory(homeDir, resolvedHome, r.URL.Query().Get("path"))
		if browseErr != nil {
			writeRestoreError(w, browseErr.Status, browseErr.Code, browseErr.Message)
			return
		}
		writeJSON(w, result)
	}
}

func (s *Server) handleRestoreBrowseSnapshot(cfgPath string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		snapshotID := strings.TrimSpace(r.URL.Query().Get("snapshot_id"))
		if snapshotID == "" {
			snapshotID = "latest"
		}
		basePath := strings.TrimSpace(r.URL.Query().Get("path"))
		if basePath == "" {
			basePath = "/"
		}
		basePath = filepath.Clean(basePath)
		if !filepath.IsAbs(basePath) {
			basePath = "/" + strings.TrimLeft(basePath, "/")
		}

		env, err := resticEnvForUI(cfgPath)
		if err != nil {
			writeRestoreError(w, http.StatusInternalServerError, "RESTORE_BROWSE_SNAPSHOT_ENV_FAILED", "restore environment unavailable")
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), restoreTimeout)
		defer cancel()
		cmd := exec.CommandContext(ctx, "restic", "ls", snapshotID, basePath, "--json")
		cmd.Env = env
		var stdout bytes.Buffer
		var stderr bytes.Buffer
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr
		if err := cmd.Run(); err != nil {
			logRestoreDetail("RESTORE_BROWSE_SNAPSHOT_FAILED", fmt.Errorf("%w (%s)", err, tailText(stderr.String(), 1024)))
			writeRestoreError(w, http.StatusBadRequest, "RESTORE_BROWSE_SNAPSHOT_FAILED", "cannot browse snapshot path")
			return
		}

		children := make(map[string]browseEntry)
		sc := bufio.NewScanner(bytes.NewReader(stdout.Bytes()))
		for sc.Scan() {
			line := strings.TrimSpace(sc.Text())
			if line == "" {
				continue
			}
			var obj map[string]interface{}
			if err := json.Unmarshal([]byte(line), &obj); err != nil {
				continue
			}
			if st, _ := obj["struct_type"].(string); st != "node" {
				continue
			}
			p, _ := obj["path"].(string)
			if p == "" || p == basePath {
				continue
			}
			p = filepath.Clean(p)
			if !isSubPath(p, basePath) {
				continue
			}
			rel := strings.TrimPrefix(p, basePath)
			rel = strings.TrimPrefix(rel, "/")
			if rel == "" {
				continue
			}
			parts := strings.Split(rel, "/")
			childName := parts[0]
			childPath := filepath.Join(basePath, childName)
			isDir := len(parts) > 1
			if !isDir {
				t, _ := obj["type"].(string)
				isDir = t == "dir"
			}
			existing, ok := children[childPath]
			if ok {
				existing.IsDir = existing.IsDir || isDir
				existing.Type = map[bool]string{true: "dir", false: "file"}[existing.IsDir]
				children[childPath] = existing
				continue
			}
			children[childPath] = browseEntry{
				Path:  childPath,
				Name:  childName,
				Type:  map[bool]string{true: "dir", false: "file"}[isDir],
				IsDir: isDir,
			}
		}
		entries := make([]browseEntry, 0, len(children))
		for _, e := range children {
			entries = append(entries, e)
		}
		sort.Slice(entries, func(i, j int) bool {
			if entries[i].IsDir != entries[j].IsDir {
				return entries[i].IsDir
			}
			return strings.ToLower(entries[i].Name) < strings.ToLower(entries[j].Name)
		})
		parent := filepath.Dir(basePath)
		if basePath == "/" {
			parent = "/"
		}
		writeJSON(w, map[string]interface{}{
			"path":        basePath,
			"parent":      parent,
			"entries":     entries,
			"snapshot_id": snapshotID,
		})
	}
}

func (s *Server) handleRestorePlan(cfgPath string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		var req restoreRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeRestoreError(w, http.StatusBadRequest, "RESTORE_BAD_REQUEST", "invalid JSON request")
			return
		}
		if _, err := exec.LookPath("restic"); err != nil {
			writeRestoreError(w, http.StatusInternalServerError, "RESTORE_PLAN_PREREQ_FAILED", "restic not found on PATH")
			return
		}
		if _, err := resticEnvForUI(cfgPath); err != nil {
			logRestoreDetail("RESTORE_PLAN_PREREQ_FAILED", err)
			writeRestoreError(w, http.StatusInternalServerError, "RESTORE_PLAN_PREREQ_FAILED", "restore prerequisites unavailable")
			return
		}
		writeJSON(w, planRestore(req))
	}
}

func (s *Server) handleRestoreRun(cfgPath string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		var req restoreRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeRestoreError(w, http.StatusBadRequest, "RESTORE_BAD_REQUEST", "invalid JSON request")
			return
		}
		if req.SnapshotID == "" {
			req.SnapshotID = "latest"
		}
		validateErrs, confirmRequired := validateRestoreRequest(req)
		if len(validateErrs) > 0 {
			w.WriteHeader(http.StatusBadRequest)
			writeJSON(w, map[string]interface{}{"ok": false, "errors": validateErrs})
			return
		}
		if confirmRequired && !req.ConfirmOverwrite {
			w.WriteHeader(http.StatusConflict)
			writeJSON(w, map[string]interface{}{
				"ok":               false,
				"confirm_required": true,
				"errors":           []string{"target exists and is non-empty; set confirm_overwrite=true to continue"},
			})
			return
		}

		env, err := resticEnvForUI(cfgPath)
		if err != nil {
			logRestoreDetail("RESTORE_ENV_FAILED", err)
			writeRestoreError(w, http.StatusInternalServerError, "RESTORE_ENV_FAILED", "restore environment unavailable")
			return
		}
		start := time.Now()
		ctx, cancel := context.WithTimeout(r.Context(), restoreTimeout)
		defer cancel()
		if execErr := executeRestore(ctx, env, req); execErr != nil {
			logRestoreDetail(execErr.Code, execErr.Err)
			writeRestoreError(w, http.StatusInternalServerError, execErr.Code, execErr.Message)
			return
		}

		writeJSON(w, map[string]interface{}{
			"ok":          true,
			"type":        req.Type,
			"snapshot_id": req.SnapshotID,
			"path":        req.Path,
			"target":      req.Target,
			"duration_ms": time.Since(start).Milliseconds(),
			"open_hint":   openHint(req.Target),
		})
	}
}
