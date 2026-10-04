package cli

import (
	"fmt"
	"os/exec"
	"runtime"
	"strings"
)

// resticInstalled reports whether the restic binary is on PATH.
func resticInstalled() bool {
	_, err := exec.LookPath("restic")
	return err == nil
}

// resticInstallHint returns a copy-pasteable command for installing restic on
// the current platform, so a user who hits "restic not found" is not left
// searching. Returns "" where we have no confident instruction.
func resticInstallHint() string {
	switch runtime.GOOS {
	case "darwin":
		return "brew install restic"
	case "linux":
		return "sudo apt install restic   (or: sudo yum install restic)"
	case "windows":
		return "winget install restic.restic"
	default:
		return ""
	}
}

// warnIfResticMissing prints an actionable warning when restic is absent.
// Backups cannot run without it, and the failure otherwise surfaces at the
// scheduled backup time rather than at install time.
func warnIfResticMissing() {
	if resticInstalled() {
		return
	}
	fmt.Println("⚠  restic was not found in PATH - scheduled backups will fail until it is installed.")
	if hint := resticInstallHint(); hint != "" {
		fmt.Println("   Install it with:  " + hint)
	} else {
		fmt.Println("   Download it from https://restic.net")
	}
	fmt.Println("   Check again with:  xentz-agent doctor")
}

// describeSchedulePhrase renders the daily schedule for user-facing summaries.
func describeSchedulePhrase(dailyAt string) string {
	dailyAt = strings.TrimSpace(dailyAt)
	if dailyAt == "" {
		return "no daily time configured"
	}
	return "daily at " + dailyAt + " local time"
}
