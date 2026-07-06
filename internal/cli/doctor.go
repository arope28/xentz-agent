package cli

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"xentz-agent/internal/config"
	"xentz-agent/internal/controlapi"
	"xentz-agent/internal/enroll"
	"xentz-agent/internal/identity"
	"xentz-agent/internal/paths"
	"xentz-agent/internal/secretstore"
)

func RunDoctor(args []string) error {
	fs := flag.NewFlagSet("doctor", flag.ExitOnError)
	mode := fs.String("mode", "user", "Inspect mode: user or system")
	configPath := fs.String("config", "", "Config path override")
	checkServer := fs.Bool("check-server", false, "Attempt GET /control/v1/config and print HTTP status")
	if err := fs.Parse(args); err != nil {
		return fmt.Errorf("parse flags: %w", err)
	}
	cfgFile, err := resolveConfigPathWithMode(*configPath, *mode)
	if err != nil {
		return fmt.Errorf("resolve config path: %w", err)
	}
	if err := doctorCommand(*mode, cfgFile, *checkServer); err != nil {
		return fmt.Errorf("doctor failed: %w", err)
	}
	return nil
}

func doctorCommand(mode, cfgFile string, checkServer bool) error {
	effectiveMode := strings.TrimSpace(mode)
	if effectiveMode == "" {
		effectiveMode = string(paths.ResolveMode(""))
	}
	cfgDir, err := paths.ConfigDir(effectiveMode)
	if err != nil {
		return fmt.Errorf("resolve config dir: %w", err)
	}
	stateDir, err := paths.StateDir(effectiveMode)
	if err != nil {
		return fmt.Errorf("resolve state dir: %w", err)
	}
	logDir, err := paths.LogDir(effectiveMode)
	if err != nil {
		return fmt.Errorf("resolve log dir: %w", err)
	}
	fmt.Printf("Mode:        %s\n", effectiveMode)
	fmt.Printf("Config file: %s\n", cfgFile)
	fmt.Printf("Config dir:  %s\n", cfgDir)
	fmt.Printf("State dir:   %s\n", stateDir)
	fmt.Printf("Logs dir:    %s\n", logDir)
	fmt.Printf("Identity:    %s\n", filepath.Join(stateDir, "identity.json"))

	cfg, err := config.Read(cfgFile)
	if err != nil {
		fmt.Printf("Config read: missing/unreadable (%v)\n", err)
	} else {
		fmt.Printf("Enrolled in config: %v\n", enroll.IsEnrolled(cfg.TenantID, cfg.DeviceID))
		fmt.Printf("Tenant ID:   %s\n", strings.TrimSpace(cfg.TenantID))
		fmt.Printf("Device ID:   %s\n", strings.TrimSpace(cfg.DeviceID))
		fmt.Printf("Server URL:  %s\n", strings.TrimSpace(cfg.ServerURL))
		if strings.TrimSpace(cfg.DeviceAPIKey) != "" {
			fmt.Printf("Config has device_api_key field: yes (legacy/fallback)\n")
		} else {
			fmt.Printf("Config has device_api_key field: no\n")
		}
	}

	keyName := "device_api_key (" + strings.ToLower(strings.TrimSpace(effectiveMode)) + ")"
	apiKey, err := config.GetDeviceAPIKeyForModeReadOnly(config.Config{Mode: effectiveMode}, effectiveMode)
	if err == nil {
		fmt.Printf("Secret store %s: present (len=%d)\n", keyName, len(strings.TrimSpace(string(apiKey))))
	} else if errors.Is(err, secretstore.ErrNotFound) {
		fmt.Printf("Secret store %s: missing\n", keyName)
	} else {
		fmt.Printf("Secret store %s: error (%v)\n", keyName, err)
	}

	if _, err := exec.LookPath("restic"); err != nil {
		fmt.Println("Restore readiness: restic not found in PATH")
	} else {
		fmt.Println("Restore readiness: restic found in PATH")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		fmt.Printf("Restore readiness: cannot resolve home directory (%v)\n", err)
	} else {
		restoreTestDir := filepath.Join(home, "Desktop", "xentz-restore-check")
		if err := os.MkdirAll(restoreTestDir, 0o700); err != nil {
			fmt.Printf("Restore readiness: destination not writable (%v)\n", err)
		} else {
			f, err := os.CreateTemp(restoreTestDir, ".write-test-*")
			if err != nil {
				fmt.Printf("Restore readiness: destination not writable (%v)\n", err)
			} else {
				_ = f.Close()
				_ = os.Remove(f.Name())
				fmt.Printf("Restore readiness: writable destination OK (%s)\n", restoreTestDir)
			}
		}
	}
	if runtime.GOOS == "darwin" {
		likelyProtected := false
		for _, inc := range cfg.Include {
			if strings.Contains(inc, "/Documents") || strings.Contains(inc, "/Desktop") {
				likelyProtected = true
				break
			}
		}
		if likelyProtected {
			fmt.Println("macOS hint: include paths contain Documents/Desktop. If restore/backup shows 'operation not permitted', see docs/MACOS_FULL_DISK_ACCESS_CHECKLIST.md")
		}
	}
	if !checkServer {
		return nil
	}
	serverURL := strings.TrimSpace(cfg.ServerURL)
	if serverURL == "" {
		if id, idErr := identity.Load(effectiveMode); idErr == nil {
			serverURL = strings.TrimSpace(id.ServerURL)
		}
	}
	if serverURL == "" {
		fmt.Println("Server check: skipped (server_url missing in config and identity)")
		return nil
	}
	key := strings.TrimSpace(apiKey)
	if key == "" {
		key = strings.TrimSpace(cfg.DeviceAPIKey)
	}
	if key == "" {
		fmt.Println("Server check: skipped (no device API key in secretstore/config)")
		return nil
	}
	client, err := controlapi.New(serverURL, key, 10*time.Second)
	if err != nil {
		fmt.Printf("Server check: skipped (%v)\n", err)
		return nil
	}
	status, err := client.GetStatus("/control/v1/config")
	if err != nil {
		var statusErr *controlapi.StatusError
		if errors.As(err, &statusErr) {
			fmt.Printf("Server check status: %d\n", statusErr.StatusCode)
			return nil
		}
		fmt.Printf("Server check: request error (%v)\n", err)
		return nil
	}
	fmt.Printf("Server check status: %d\n", status)
	return nil
}
