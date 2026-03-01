package install

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"xentz-agent/internal/config"
	"xentz-agent/internal/paths"
)

const (
	label = "com.xentz.agent"
)

func MacOSLaunchdInstall(configPath string) error {
	// Read config to get schedule time (HH:MM)
	cfg, err := config.Read(configPath)
	if err != nil {
		return err
	}
	hour, minute, err := parseHHMM(cfg.Schedule.DailyAt)
	if err != nil {
		return fmt.Errorf("invalid --daily-at (%q): %w", cfg.Schedule.DailyAt, err)
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}

	plistDir := filepath.Join(home, "Library", "LaunchAgents")
	if err := os.MkdirAll(plistDir, 0o755); err != nil {
		return err
	}
	plistPath := filepath.Join(plistDir, label+".plist")

	exePath, err := os.Executable()
	if err != nil {
		return err
	}

	logDir, err := paths.LogDir("")
	if err != nil {
		return fmt.Errorf("resolve log dir: %w", err)
	}
	if err := os.MkdirAll(logDir, 0o700); err != nil {
		return err
	}
	stdoutPath := filepath.Join(logDir, "agent.out.log")
	stderrPath := filepath.Join(logDir, "agent.err.log")

	plist := buildPlist(exePath, configPath, hour, minute, stdoutPath, stderrPath)
	if err := os.WriteFile(plistPath, []byte(plist), 0o644); err != nil {
		return err
	}

	// Load via launchctl (per-user domain)
	// We’ll do: launchctl bootout gui/<uid> <plist> (ignore errors), then bootstrap, then enable, then kickstart.
	uid := os.Getuid()
	domain := fmt.Sprintf("gui/%d", uid)

	_ = exec.Command("launchctl", "bootout", domain, plistPath).Run()
	if err := exec.Command("launchctl", "bootstrap", domain, plistPath).Run(); err != nil {
		return fmt.Errorf("launchctl bootstrap: %w", err)
	}
	_ = exec.Command("launchctl", "enable", domain+"/"+label).Run()
	_ = exec.Command("launchctl", "kickstart", "-k", domain+"/"+label).Run()

	return nil
}

// MacOSLaunchdInstallSystem installs a LaunchDaemon for system mode
func MacOSLaunchdInstallSystem(configPath string) error {
	// Read config to get schedule time (HH:MM)
	cfg, err := config.Read(configPath)
	if err != nil {
		return err
	}
	hour, minute, err := parseHHMM(cfg.Schedule.DailyAt)
	if err != nil {
		return fmt.Errorf("invalid --daily-at (%q): %w", cfg.Schedule.DailyAt, err)
	}

	plistDir := "/Library/LaunchDaemons"
	if err := os.MkdirAll(plistDir, 0o755); err != nil {
		return err
	}
	plistPath := filepath.Join(plistDir, label+".plist")

	exePath, err := os.Executable()
	if err != nil {
		return err
	}

	logDir, err := paths.LogDir("system")
	if err != nil {
		return fmt.Errorf("resolve log dir: %w", err)
	}
	if err := os.MkdirAll(logDir, 0o700); err != nil {
		return err
	}
	stdoutPath := filepath.Join(logDir, "agent.out.log")
	stderrPath := filepath.Join(logDir, "agent.err.log")

	plist := buildPlist(exePath, configPath, hour, minute, stdoutPath, stderrPath)
	if err := os.WriteFile(plistPath, []byte(plist), 0o644); err != nil {
		return err
	}

	_ = exec.Command("launchctl", "bootout", "system", plistPath).Run()
	if err := exec.Command("launchctl", "bootstrap", "system", plistPath).Run(); err != nil {
		return fmt.Errorf("launchctl bootstrap: %w", err)
	}
	_ = exec.Command("launchctl", "enable", "system/"+label).Run()
	_ = exec.Command("launchctl", "kickstart", "-k", "system/"+label).Run()
	return nil
}

// MacOSLaunchdUninstall removes a LaunchAgent for user mode
func MacOSLaunchdUninstall(_ string) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	plistPath := filepath.Join(home, "Library", "LaunchAgents", label+".plist")
	domain := fmt.Sprintf("gui/%d", os.Getuid())
	_ = exec.Command("launchctl", "bootout", domain, plistPath).Run()
	_ = os.Remove(plistPath)
	return nil
}

// MacOSLaunchdUninstallSystem removes a LaunchDaemon for system mode
func MacOSLaunchdUninstallSystem(_ string) error {
	plistPath := filepath.Join("/Library/LaunchDaemons", label+".plist")
	_ = exec.Command("launchctl", "bootout", "system", plistPath).Run()
	_ = os.Remove(plistPath)
	return nil
}

func parseHHMM(s string) (hour, minute int, err error) {
	parts := strings.Split(s, ":")
	if len(parts) != 2 {
		return 0, 0, fmt.Errorf("expected HH:MM")
	}
	var h, m int
	_, err = fmt.Sscanf(parts[0], "%d", &h)
	if err != nil {
		return 0, 0, err
	}
	_, err = fmt.Sscanf(parts[1], "%d", &m)
	if err != nil {
		return 0, 0, err
	}
	if h < 0 || h > 23 || m < 0 || m > 59 {
		return 0, 0, fmt.Errorf("out of range")
	}
	return h, m, nil
}

// escapeXML escapes XML special characters in a string
func escapeXML(s string) string {
	var result strings.Builder
	for _, r := range s {
		switch r {
		case '&':
			result.WriteString("&amp;")
		case '<':
			result.WriteString("&lt;")
		case '>':
			result.WriteString("&gt;")
		case '"':
			result.WriteString("&quot;")
		case '\'':
			result.WriteString("&apos;")
		default:
			result.WriteRune(r)
		}
	}
	return result.String()
}

func buildPlist(exePath, configPath string, hour, minute int, stdoutPath, stderrPath string) string {
	// launchd expects ProgramArguments as array; we run `backup`
	// StartCalendarInterval handles daily schedule. RunAtLoad gives a run on install/boot.
	// Escape XML special characters in paths
	exePathEscaped := escapeXML(exePath)
	configPathEscaped := escapeXML(configPath)
	stdoutPathEscaped := escapeXML(stdoutPath)
	stderrPathEscaped := escapeXML(stderrPath)

	// Get user's home directory to include in PATH
	home, err := os.UserHomeDir()
	homeBin := ""
	if err == nil {
		// Include common user installation paths
		goBin := filepath.Join(home, "go", "bin")
		localBin := filepath.Join(home, ".local", "bin")
		homeBin = goBin + ":" + localBin + ":"
	}

	// Build PATH with common restic installation locations
	// Include system paths, Homebrew paths (Intel and Apple Silicon), and user paths
	pathEnv := homeBin + "/usr/local/bin:/opt/homebrew/bin:/usr/bin:/bin:/usr/sbin:/sbin"
	pathEnvEscaped := escapeXML(pathEnv)

	var b bytes.Buffer
	fmt.Fprintf(&b, `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
  <dict>
    <key>Label</key><string>%s</string>

    <key>ProgramArguments</key>
    <array>
      <string>%s</string>
      <string>backup</string>
      <string>--auto-init</string>
      <string>--config</string>
      <string>%s</string>
    </array>

    <key>EnvironmentVariables</key>
    <dict>
      <key>PATH</key>
      <string>%s</string>
    </dict>

    <key>RunAtLoad</key><true/>

    <key>StartCalendarInterval</key>
    <dict>
      <key>Hour</key><integer>%d</integer>
      <key>Minute</key><integer>%d</integer>
    </dict>

    <key>StandardOutPath</key><string>%s</string>
    <key>StandardErrorPath</key><string>%s</string>

    <key>ProcessType</key><string>Background</string>
  </dict>
</plist>
`, label, exePathEscaped, configPathEscaped, pathEnvEscaped, hour, minute, stdoutPathEscaped, stderrPathEscaped)

	// Small trick: add a comment-like timestamp to help debugging (doesn't affect plist parsing)
	_ = time.Now().UTC()
	return b.String()
}
