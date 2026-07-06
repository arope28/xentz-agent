package localui

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
)

type restoreExecutionError struct {
	Code    string
	Message string
	Err     error
}

func executeRestore(ctx context.Context, env []string, req restoreRequest) *restoreExecutionError {
	switch req.Type {
	case "file":
		return executeFileRestore(ctx, env, req)
	case "folder", "snapshot":
		return executeTreeRestore(ctx, env, req)
	default:
		return &restoreExecutionError{
			Code:    "RESTORE_BAD_REQUEST",
			Message: "type must be file, folder, or snapshot",
		}
	}
}

func executeFileRestore(ctx context.Context, env []string, req restoreRequest) *restoreExecutionError {
	if err := os.MkdirAll(filepath.Dir(req.Target), 0o700); err != nil {
		return &restoreExecutionError{
			Code:    "RESTORE_TARGET_DIR_FAILED",
			Message: "failed to prepare target directory",
			Err:     err,
		}
	}
	tmp, err := os.CreateTemp(filepath.Dir(req.Target), ".xentz-restore-*")
	if err != nil {
		return &restoreExecutionError{
			Code:    "RESTORE_OUTPUT_OPEN_FAILED",
			Message: "failed to prepare restore output",
			Err:     err,
		}
	}
	tmpName := tmp.Name()
	closed := false
	defer func() {
		if !closed {
			_ = tmp.Close()
		}
		_ = os.Remove(tmpName)
	}()

	cmd := exec.CommandContext(ctx, "restic", "dump", req.SnapshotID, req.Path)
	cmd.Env = env
	var stderr bytes.Buffer
	cmd.Stdout = tmp
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return &restoreExecutionError{
			Code:    "RESTORE_EXEC_FAILED",
			Message: "restore failed",
			Err:     fmt.Errorf("%w (%s)", err, tailText(stderr.String(), 2048)),
		}
	}
	if err := tmp.Sync(); err != nil {
		return &restoreExecutionError{
			Code:    "RESTORE_OUTPUT_SYNC_FAILED",
			Message: "failed to finalize restored file",
			Err:     err,
		}
	}
	if err := tmp.Close(); err != nil {
		closed = true
		return &restoreExecutionError{
			Code:    "RESTORE_OUTPUT_CLOSE_FAILED",
			Message: "failed to finalize restored file",
			Err:     err,
		}
	}
	closed = true
	if err := os.Rename(tmpName, req.Target); err != nil {
		return &restoreExecutionError{
			Code:    "RESTORE_OUTPUT_RENAME_FAILED",
			Message: "failed to place restored file",
			Err:     err,
		}
	}
	return nil
}

func executeTreeRestore(ctx context.Context, env []string, req restoreRequest) *restoreExecutionError {
	if err := os.MkdirAll(req.Target, 0o700); err != nil {
		return &restoreExecutionError{
			Code:    "RESTORE_TARGET_DIR_FAILED",
			Message: "failed to prepare target directory",
			Err:     err,
		}
	}
	args := []string{"restore", req.SnapshotID, "--target", req.Target}
	if req.Type == "folder" {
		args = append(args, "--include", req.Path)
	}
	cmd := exec.CommandContext(ctx, "restic", args...)
	cmd.Env = env
	var stderr bytes.Buffer
	cmd.Stdout = io.Discard
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return &restoreExecutionError{
			Code:    "RESTORE_EXEC_FAILED",
			Message: "restore failed",
			Err:     fmt.Errorf("%w (%s)", err, tailText(stderr.String(), 2048)),
		}
	}
	return nil
}
