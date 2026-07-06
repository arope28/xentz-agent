package localui

import (
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type browseLocalResult struct {
	Path    string        `json:"path"`
	Parent  string        `json:"parent"`
	Entries []browseEntry `json:"entries"`
}

type restoreBrowseError struct {
	Status  int
	Code    string
	Message string
}

func browseLocalDirectory(homeDir, resolvedHome, rawPath string) (*browseLocalResult, *restoreBrowseError) {
	if strings.TrimSpace(rawPath) == "" {
		rawPath = homeDir
	}
	p := filepath.Clean(rawPath)
	if !filepath.IsAbs(p) {
		p = filepath.Join(homeDir, p)
	}
	abs, err := filepath.Abs(p)
	if err != nil {
		return nil, &restoreBrowseError{
			Status:  http.StatusBadRequest,
			Code:    "RESTORE_BROWSE_LOCAL_BAD_PATH",
			Message: "invalid local path",
		}
	}
	resolvedPath, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return nil, &restoreBrowseError{
			Status:  http.StatusBadRequest,
			Code:    "RESTORE_BROWSE_LOCAL_BAD_PATH",
			Message: "cannot resolve local path",
		}
	}
	if !isSubPath(resolvedPath, resolvedHome) {
		return nil, &restoreBrowseError{
			Status:  http.StatusForbidden,
			Code:    "RESTORE_BROWSE_LOCAL_FORBIDDEN",
			Message: "path outside home directory is not allowed",
		}
	}
	items, err := os.ReadDir(resolvedPath)
	if err != nil {
		return nil, &restoreBrowseError{
			Status:  http.StatusBadRequest,
			Code:    "RESTORE_BROWSE_LOCAL_READ_FAILED",
			Message: "cannot read local path",
		}
	}
	entries := make([]browseEntry, 0, len(items))
	for _, item := range items {
		name := item.Name()
		child := filepath.Join(resolvedPath, name)
		entries = append(entries, browseEntry{
			Path:  child,
			Name:  name,
			Type:  map[bool]string{true: "dir", false: "file"}[item.IsDir()],
			IsDir: item.IsDir(),
		})
	}
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].IsDir != entries[j].IsDir {
			return entries[i].IsDir
		}
		return strings.ToLower(entries[i].Name) < strings.ToLower(entries[j].Name)
	})
	parent := filepath.Dir(resolvedPath)
	if !isSubPath(parent, resolvedHome) {
		parent = ""
	}
	return &browseLocalResult{
		Path:    resolvedPath,
		Parent:  parent,
		Entries: entries,
	}, nil
}
