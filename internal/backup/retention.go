package backup

import (
	"bytes"
	"context"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"xentz-agent/internal/config"
	"xentz-agent/internal/state"
)

func RunRetention(ctx context.Context, cfg config.Config) state.LastRun {
	start := time.Now()

	if cfg.Restic.Repository == "" {
		return state.NewLastRunError(time.Since(start), 0, "restic.repository is required")
	}
	if cfg.Restic.PasswordFile == "" {
		return state.NewLastRunError(time.Since(start), 0, "restic.password_file is required")
	}
	if _, err := exec.LookPath("restic"); err != nil {
		return state.NewLastRunError(time.Since(start), 0, "restic not found in PATH")
	}

	args := []string{"forget"}

	r := cfg.Retention
	// If user never set retention, refuse to run (prevents accidental nukes / weird defaults)
	if r.KeepLast == 0 && r.KeepDaily == 0 && r.KeepWeekly == 0 && r.KeepMonthly == 0 && r.KeepYearly == 0 {
		return state.NewLastRunError(time.Since(start), 0, "retention policy not configured (set keep_* values)")
	}

	if r.KeepLast > 0 {
		args = append(args, "--keep-last", itoa(r.KeepLast))
	}
	if r.KeepDaily > 0 {
		args = append(args, "--keep-daily", itoa(r.KeepDaily))
	}
	if r.KeepWeekly > 0 {
		args = append(args, "--keep-weekly", itoa(r.KeepWeekly))
	}
	if r.KeepMonthly > 0 {
		args = append(args, "--keep-monthly", itoa(r.KeepMonthly))
	}
	if r.KeepYearly > 0 {
		args = append(args, "--keep-yearly", itoa(r.KeepYearly))
	}

	if r.Prune {
		args = append(args, "--prune")
	}

	cmd := exec.CommandContext(ctx, "restic", args...)
	cmd.Env = append(cmd.Environ(),
		"RESTIC_REPOSITORY="+cfg.Restic.Repository,
		"RESTIC_PASSWORD_FILE="+expandHome(cfg.Restic.PasswordFile),
	)

	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out

	err := cmd.Run()
	dur := time.Since(start)

	if err != nil {
		return state.NewLastRunError(dur, 0, "restic forget/prune failed: "+err.Error()+"\n"+tail(out.String(), 8192))
	}
	return state.NewLastRunSuccess(dur, 0)
}

// tiny helpers (avoid fmt import in hot path)
func itoa(i int) string {
	// minimal
	buf := []byte{}
	n := i
	if n == 0 {
		return "0"
	}
	for n > 0 {
		buf = append([]byte{byte('0' + (n % 10))}, buf...)
		n /= 10
	}
	return string(buf)
}

func expandHome(p string) string {
	if strings.HasPrefix(p, "~/") {
		home, _ := filepath.Abs(strings.TrimSuffix(p, "/"))
		_ = home
	}
	// Minimal MVP: assume install writes absolute paths.
	return p
}

func tail(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[len(s)-max:]
}