//go:build windows

package windows

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"golang.org/x/sys/windows/svc"
	"golang.org/x/sys/windows/svc/mgr"

	"xentz-agent/internal/config"
)

const ServiceName = "XentzAgent"

type serviceHandler struct {
	configPath string
	cancel     context.CancelFunc
}

func RunService(configPath string) error {
	isService, err := svc.IsWindowsService()
	if err != nil {
		return fmt.Errorf("detect service: %w", err)
	}
	if !isService {
		return fmt.Errorf("not running as a Windows service")
	}
	return svc.Run(ServiceName, &serviceHandler{configPath: configPath})
}

func (s *serviceHandler) Execute(args []string, r <-chan svc.ChangeRequest, changes chan<- svc.Status) (bool, uint32) {
	const accepts = svc.AcceptStop | svc.AcceptShutdown
	changes <- svc.Status{State: svc.StartPending}

	ctx, cancel := context.WithCancel(context.Background())
	s.cancel = cancel
	go s.runScheduler(ctx)

	changes <- svc.Status{State: svc.Running, Accepts: accepts}

loop:
	for c := range r {
		switch c.Cmd {
		case svc.Interrogate:
			changes <- c.CurrentStatus
		case svc.Stop, svc.Shutdown:
			changes <- svc.Status{State: svc.StopPending}
			cancel()
			break loop
		default:
		}
	}

	changes <- svc.Status{State: svc.Stopped}
	return false, 0
}

func (s *serviceHandler) runScheduler(ctx context.Context) {
	for {
		nextRun := time.Now().Add(1 * time.Hour)
		cfg, err := config.Read(s.configPath)
		if err == nil {
			if t, err := nextRunTime(cfg.Schedule.DailyAt, time.Now()); err == nil {
				nextRun = t
			}
		}

		timer := time.NewTimer(time.Until(nextRun))
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
			_ = runBackup(ctx, s.configPath)
		}
	}
}

func runBackup(ctx context.Context, configPath string) error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	cmd := exec.CommandContext(ctx, exe, "backup", "--auto-init", "--config", configPath)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func nextRunTime(dailyAt string, now time.Time) (time.Time, error) {
	hour, minute, err := parseHHMM(dailyAt)
	if err != nil {
		return time.Time{}, err
	}
	run := time.Date(now.Year(), now.Month(), now.Day(), hour, minute, 0, 0, now.Location())
	if !run.After(now) {
		run = run.Add(24 * time.Hour)
	}
	return run, nil
}

func parseHHMM(s string) (int, int, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 2, 0, nil
	}
	parts := strings.Split(s, ":")
	if len(parts) != 2 {
		return 0, 0, fmt.Errorf("invalid time format")
	}
	hour, err := parseInt(parts[0], 0, 23)
	if err != nil {
		return 0, 0, err
	}
	minute, err := parseInt(parts[1], 0, 59)
	if err != nil {
		return 0, 0, err
	}
	return hour, minute, nil
}

func parseInt(s string, min, max int) (int, error) {
	var v int
	_, err := fmt.Sscanf(strings.TrimSpace(s), "%d", &v)
	if err != nil {
		return 0, fmt.Errorf("invalid number")
	}
	if v < min || v > max {
		return 0, fmt.Errorf("value out of range")
	}
	return v, nil
}

func InstallService(configPath string) error {
	exePath, err := os.Executable()
	if err != nil {
		return err
	}
	m, err := mgr.Connect()
	if err != nil {
		return err
	}
	defer m.Disconnect()

	if s, err := m.OpenService(ServiceName); err == nil {
		s.Close()
		return fmt.Errorf("service already exists")
	}

	args := []string{"service", "run", "--config", configPath}
	s, err := m.CreateService(ServiceName, exePath, mgr.Config{
		DisplayName: "Xentz Agent",
		StartType:   mgr.StartAutomatic,
	}, args...)
	if err != nil {
		return err
	}
	defer s.Close()

	if err := s.Start(); err != nil {
		return fmt.Errorf("start service: %w", err)
	}
	return nil
}

func UninstallService() error {
	m, err := mgr.Connect()
	if err != nil {
		return err
	}
	defer m.Disconnect()

	s, err := m.OpenService(ServiceName)
	if err != nil {
		return err
	}
	defer s.Close()

	_, _ = s.Control(svc.Stop)
	if err := s.Delete(); err != nil {
		return err
	}
	return nil
}
