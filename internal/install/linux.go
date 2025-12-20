package install

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"xentz-agent/internal/config"
)

const (
	linuxServiceName = "xentz-agent"
)

func LinuxSystemdInstall(configPath string) error {
	if runtime.GOOS != "linux" {
		return fmt.Errorf("LinuxSystemdInstall can only run on Linux")
	}

	// Read config to get schedule time (HH:MM)
	cfg, err := config.Read(configPath)
	if err != nil {
		return err
	}
	hour, minute, err := parseHHMM(cfg.Schedule.DailyAt)
	if err != nil {
		return fmt.Errorf("invalid --daily-at (%q): %w", cfg.Schedule.DailyAt, err)
	}

	exePath, err := os.Executable()
	if err != nil {
		return err
	}
	if !filepath.IsAbs(exePath) {
		absPath, err := filepath.Abs(exePath)
		if err != nil {
			return fmt.Errorf("get absolute path: %w", err)
		}
		exePath = absPath
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}

	logDir := filepath.Join(home, ".xentz-agent", "logs")
	if err := os.MkdirAll(logDir, 0o700); err != nil {
		return err
	}
	stdoutPath := filepath.Join(logDir, "agent.out.log")
	stderrPath := filepath.Join(logDir, "agent.err.log")

	// Check if systemd user services are available
	if hasSystemd() {
		return installSystemdUserService(exePath, configPath, hour, minute, stdoutPath, stderrPath, home)
	}

	// Fallback to cron
	return installCron(exePath, configPath, hour, minute, home)
}

func hasSystemd() bool {
	// Check if systemd is available (check for systemctl command)
	_, err := exec.LookPath("systemctl")
	if err != nil {
		return false
	}
	// Check if systemd user services are supported
	cmd := exec.Command("systemctl", "--user", "list-units", "--type=service", "--no-legend")
	return cmd.Run() == nil
}

func installSystemdUserService(exePath, configPath string, hour, minute int, stdoutPath, stderrPath, home string) error {
	// Create systemd user service directory
	serviceDir := filepath.Join(home, ".config", "systemd", "user")
	if err := os.MkdirAll(serviceDir, 0o755); err != nil {
		return fmt.Errorf("create systemd user dir: %w", err)
	}

	serviceFile := filepath.Join(serviceDir, linuxServiceName+".service")
	serviceContent := buildSystemdService(exePath, configPath, hour, minute, stdoutPath, stderrPath)

	if err := os.WriteFile(serviceFile, []byte(serviceContent), 0o644); err != nil {
		return fmt.Errorf("write systemd service: %w", err)
	}

	// Create timer file for scheduled execution
	timerFile := filepath.Join(serviceDir, linuxServiceName+".timer")
	timerContent := buildSystemdTimer(hour, minute)

	if err := os.WriteFile(timerFile, []byte(timerContent), 0o644); err != nil {
		return fmt.Errorf("write systemd timer: %w", err)
	}

	// Reload systemd user daemon
	reloadCmd := exec.Command("systemctl", "--user", "daemon-reload")
	if output, err := reloadCmd.CombinedOutput(); err != nil {
		return fmt.Errorf("reload systemd daemon: %w\noutput: %s", err, string(output))
	}

	// Enable and start the timer
	enableCmd := exec.Command("systemctl", "--user", "enable", linuxServiceName+".timer")
	if output, err := enableCmd.CombinedOutput(); err != nil {
		return fmt.Errorf("enable systemd timer: %w\noutput: %s", err, string(output))
	}

	startCmd := exec.Command("systemctl", "--user", "start", linuxServiceName+".timer")
	if output, err := startCmd.CombinedOutput(); err != nil {
		return fmt.Errorf("start systemd timer: %w\noutput: %s", err, string(output))
	}

	// Run the service once immediately
	_ = exec.Command("systemctl", "--user", "start", linuxServiceName+".service").Run()

	return nil
}

func buildSystemdService(exePath, configPath string, hour, minute int, stdoutPath, stderrPath string) string {
	return fmt.Sprintf(`[Unit]
Description=xentz-agent backup service
After=network.target

[Service]
Type=oneshot
ExecStart=%s backup --config %s
StandardOutput=append:%s
StandardError=append:%s

[Install]
WantedBy=default.target
`, exePath, configPath, stdoutPath, stderrPath)
}

func buildSystemdTimer(hour, minute int) string {
	return fmt.Sprintf(`[Unit]
Description=xentz-agent backup timer

[Timer]
OnCalendar=*-*-* %02d:%02d:00
Persistent=true

[Install]
WantedBy=timers.target
`, hour, minute)
}

func installCron(exePath, configPath string, hour, minute int, home string) error {
	// Get current user's crontab
	crontabCmd := exec.Command("crontab", "-l")
	currentCron, _ := crontabCmd.Output() // Ignore error if no crontab exists

	// Build cron entry
	// Format: minute hour * * * command
	cronEntry := fmt.Sprintf("%d %d * * * %s backup --config %s >> %s/.xentz-agent/logs/agent.out.log 2>> %s/.xentz-agent/logs/agent.err.log\n",
		minute, hour, exePath, configPath, home, home)

	// Check if entry already exists
	if strings.Contains(string(currentCron), exePath) {
		// Remove old entry
		lines := strings.Split(string(currentCron), "\n")
		var newLines []string
		for _, line := range lines {
			if !strings.Contains(line, exePath) {
				newLines = append(newLines, line)
			}
		}
		currentCron = []byte(strings.Join(newLines, "\n"))
	}

	// Add new entry
	newCron := string(currentCron)
	if newCron != "" && !strings.HasSuffix(newCron, "\n") {
		newCron += "\n"
	}
	newCron += cronEntry

	// Write new crontab
	writeCmd := exec.Command("crontab", "-")
	writeCmd.Stdin = strings.NewReader(newCron)
	if output, err := writeCmd.CombinedOutput(); err != nil {
		return fmt.Errorf("write crontab: %w\noutput: %s", err, string(output))
	}

	return nil
}

