package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"xentz-agent/internal/backup"
	"xentz-agent/internal/config"
	"xentz-agent/internal/diagnostics"
	"xentz-agent/internal/enroll"
	"xentz-agent/internal/install"
	"xentz-agent/internal/localui"
	"xentz-agent/internal/logging"
	"xentz-agent/internal/paths"
	"xentz-agent/internal/report"
	windowsservice "xentz-agent/internal/service/windows"
	"xentz-agent/internal/state"
)

func usage() {
	fmt.Print(`xentz-agent - Backup Agent

Commands:
  install     Install config + scheduled task (macOS: launchd, Windows: Task Scheduler/Service, Linux: systemd/cron)
  uninstall   Remove service/scheduler and optionally purge config/state
  upgrade     Replace binary and restart service/scheduler
  diagnostics Create a support bundle (logs + config checksum + state)
  local-ui    Run localhost-only status UI
  backup      Run one backup now (used by scheduler)
  retention   Run retention/prune policy (forget old snapshots)
  status      Show last run status
  config      Manage backup paths (add/remove include/exclude paths)

Examples:
  # Token-based enrollment (recommended):
  xentz-agent install --token <install-token> --server https://control-plane.example.com --daily-at 02:00 --include "/Users/me/Documents"
  
  # Legacy mode (direct repository):
  xentz-agent install --repo rest:https://... --password "..." --daily-at 02:00 --include "/Users/me/Documents"
  
  xentz-agent backup
  xentz-agent backup --auto-init  # Auto-initialize repository if missing (use with caution)
  xentz-agent retention
  xentz-agent status
  
  # Manage backup paths:
  xentz-agent config --add-include "/Users/me/Documents" --add-include "/Users/me/Pictures"
  xentz-agent config --remove-include "/Users/me/Pictures"
  xentz-agent config --add-exclude "*.tmp"
  xentz-agent config --list-all
  
  # Uninstall (keep config by default):
  xentz-agent uninstall --mode user
  
  # Upgrade:
  xentz-agent upgrade --binary /path/to/new/xentz-agent --mode system
  
  # Diagnostics:
  xentz-agent diagnostics --out /tmp/xentz-agent-diag.zip
  
  # Local UI:
  xentz-agent local-ui --addr 127.0.0.1:9800

Flags (backup):
  --auto-init    Automatically initialize repository if it doesn't exist (default: false)
                 WARNING: Only use if you're certain the repository URL is correct.
                 Without this flag, backup will fail if repository doesn't exist.

Flags (install):
  --token         Install token for enrollment (recommended, provided by control plane)
  --server        Control plane base URL (required with --token)
  --daily-at      Time in HH:MM (24h), default 02:00
  --mode          Install mode: user or system (default: user)
  --repo          Restic repository URL (legacy mode, use --token instead)
  --password      Restic repository password (optional if server provides via enrollment)
  --password-file Path to restic password file (optional, default: <CONFIG_DIR>/restic.pw)
  --include       Repeatable. Add include paths. Example: --include "/Users/me/Documents" --include "/Users/me/Pictures"
  --exclude       Repeatable. Add exclude globs.
  --config        Config path override (default: <CONFIG_DIR>/config.json)

Flags (uninstall):
  --mode         Uninstall mode: user or system (default: user)
  --keep-config  Keep config directory (default: true)
  --purge-state  Remove state/log directories (default: false)
  --config       Config path override (default: <CONFIG_DIR>/config.json)

Flags (upgrade):
  --binary  Path to new xentz-agent binary (required)
  --mode    Upgrade mode: user or system (default: user)
  --config  Config path override (default: <CONFIG_DIR>/config.json)

Flags (diagnostics):
  --out     Output path for diagnostics bundle

Flags (local-ui):
  --addr    Bind address (default: 127.0.0.1:9800)
  --config  Config path override (default: <CONFIG_DIR>/config.json)

Flags (config):
  --add-include <path>      Add include path (repeatable). Paths are normalized (expanded and made absolute)
  --remove-include <path>   Remove include path (repeatable)
  --add-exclude <pattern>    Add exclude pattern (repeatable)
  --remove-exclude <pattern> Remove exclude pattern (repeatable)
  --list-includes            List current include paths
  --list-excludes            List current exclude patterns
  --list-all                 List both include paths and exclude patterns
  --config                   Config path override (default: <CONFIG_DIR>/config.json)

Note: With token-based enrollment, configuration (including retention policy) is fetched from the server on each run.
      In legacy mode, retention policy must be configured in config.json before running 'retention' command.
      The 'config' command updates server configuration for enrolled devices, or local config for legacy mode.
`)
}

type multiFlag []string

func (m *multiFlag) String() string { return fmt.Sprint([]string(*m)) }
func (m *multiFlag) Set(v string) error {
	*m = append(*m, v)
	return nil
}

func resolveConfigPathWithMode(override, mode string) (string, error) {
	if override != "" {
		return override, nil
	}
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case "system":
		dir, err := paths.ConfigDir("system")
		if err != nil {
			return "", err
		}
		return filepath.Join(dir, "config.json"), nil
	case "user":
		dir, err := paths.ConfigDir("user")
		if err != nil {
			return "", err
		}
		return filepath.Join(dir, "config.json"), nil
	default:
		return config.ResolvePath("")
	}
}

func replaceBinary(newPath string) error {
	exePath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("get executable: %w", err)
	}
	info, err := os.Stat(newPath)
	if err != nil {
		return fmt.Errorf("stat new binary: %w", err)
	}
	tmpPath := exePath + ".new"
	if err := copyFile(newPath, tmpPath, info.Mode()); err != nil {
		return err
	}
	if err := os.Rename(tmpPath, exePath); err != nil {
		return fmt.Errorf("replace binary: %w", err)
	}
	return nil
}

func copyFile(src, dst string, mode os.FileMode) error {
	in, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("open source: %w", err)
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode)
	if err != nil {
		return fmt.Errorf("open destination: %w", err)
	}
	defer out.Close()
	if _, err := io.Copy(out, in); err != nil {
		return fmt.Errorf("copy binary: %w", err)
	}
	return out.Sync()
}

func writePasswordFile(path, password string) (string, error) {
	if strings.TrimSpace(password) == "" {
		return "", fmt.Errorf("password is empty")
	}
	if strings.TrimSpace(path) == "" {
		dir, err := paths.ConfigDir("")
		if err != nil {
			return "", err
		}
		path = filepath.Join(dir, "restic.pw")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return "", err
	}
	contents := strings.TrimSpace(password) + "\n"
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		return "", err
	}
	return path, nil
}

func main() {
	log.SetFlags(log.LstdFlags | log.Lmicroseconds)

	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	cmd := os.Args[1]

	var cfgFile string
	var err error

	switch cmd {
	case "install":
		fs := flag.NewFlagSet(cmd, flag.ExitOnError)
		server := fs.String("server", "", "Control plane base URL (required for token-based enrollment)")
		dailyAt := fs.String("daily-at", "02:00", "Daily time HH:MM (24h)")
		mode := fs.String("mode", "user", "Install mode: user or system")
		configPath := fs.String("config", "", "Config path override")
		token := fs.String("token", "", "Install token for enrollment (primary method)")
		repo := fs.String("repo", "", "Restic repository URL (legacy mode, use --token instead)")
		password := fs.String("password", "", "Restic repository password (optional if server provides)")
		passwordFile := fs.String("password-file", "", "Path to restic password file (optional, default: <CONFIG_DIR>/restic.pw)")

		var includes multiFlag
		var excludes multiFlag
		fs.Var(&includes, "include", "Include path (repeatable)")
		fs.Var(&excludes, "exclude", "Exclude glob (repeatable)")

		if err := fs.Parse(os.Args[2:]); err != nil {
			log.Fatalf("parse flags: %v", err)
		}

		cfgFile, err = resolveConfigPathWithMode(*configPath, *mode)
		if err != nil {
			log.Fatalf("resolve config path: %v", err)
		}

		// Try to load existing config to check if already enrolled
		var cfg config.Config
		if existingCfg, err := config.Read(cfgFile); err == nil {
			cfg = existingCfg
		}

		// Determine user ID
		configDir, err := paths.ConfigDir("")
		if err != nil {
			log.Fatalf("resolve config dir: %v", err)
		}
		userID, err := enroll.GetOrCreateUserID(configDir)
		if err != nil {
			log.Fatalf("get user ID: %v", err)
		}
		cfg.UserID = userID

		// Handle enrollment flow (token-based) or legacy flow (direct repo)
		if *token != "" {
			// Token-based enrollment
			if *server == "" {
				log.Fatal("--server is required when using --token")
			}

			// Check if already enrolled
			if enroll.IsEnrolled(cfg.TenantID, cfg.DeviceID) {
				log.Println("Device is already enrolled. Using existing configuration.")
				log.Printf("  Tenant ID: %s", cfg.TenantID)
				log.Printf("  Device ID: %s", cfg.DeviceID)

				// Update server URL if a new one is provided (allows switching servers)
				if *server != "" && cfg.ServerURL != *server {
					log.Printf("  Updating server URL: %s -> %s", cfg.ServerURL, *server)
					cfg.ServerURL = *server
				}
			} else {
				// Perform enrollment
				log.Println("Enrolling device with control plane...")
				// Pass include paths to enrollment so control plane can store them
				enrollmentResult, err := enroll.Enroll(*token, *server, includes)
				if err != nil {
					log.Fatalf("enrollment failed: %v", err)
				}

				// Store enrollment data (do not store InstallToken after enrollment)
				cfg.TenantID = enrollmentResult.TenantID
				cfg.DeviceID = enrollmentResult.DeviceID
				if err := config.StoreDeviceAPIKey(enrollmentResult.DeviceAPIKey); err != nil {
					log.Fatalf("store device api key: %v", err)
				}
				cfg.DeviceAPIKey = ""
				cfg.ServerURL = *server
				cfg.Restic.Repository = enrollmentResult.RepoPath

				log.Printf("Enrollment successful:")
				log.Printf("  Tenant ID: %s", cfg.TenantID)
				log.Printf("  Device ID: %s", cfg.DeviceID)
				log.Printf("  Repository: %s", cfg.Restic.Repository)

				// Handle password from server or user input
				if enrollmentResult.Password != "" {
					// Server provided password
					if err := config.StoreResticPassword(enrollmentResult.Password); err != nil {
						log.Printf("warning: store restic password failed: %v", err)
						if *passwordFile != "" {
							if _, err := writePasswordFile(*passwordFile, enrollmentResult.Password); err != nil {
								log.Fatalf("write password file: %v", err)
							}
							cfg.Restic.PasswordFile = *passwordFile
						} else {
							log.Fatal("restic password could not be stored (secretstore unavailable)")
						}
					} else {
						cfg.Restic.PasswordFile = ""
					}
				} else if *password != "" {
					// User provided password
					if err := config.StoreResticPassword(*password); err != nil {
						log.Printf("warning: store restic password failed: %v", err)
						if *passwordFile != "" {
							if _, err := writePasswordFile(*passwordFile, *password); err != nil {
								log.Fatalf("write password file: %v", err)
							}
							cfg.Restic.PasswordFile = *passwordFile
						} else {
							log.Fatal("restic password could not be stored (secretstore unavailable)")
						}
					} else {
						cfg.Restic.PasswordFile = ""
					}
				} else {
					log.Fatal("Password required: either server must provide it or use --password flag")
				}
			}
		} else if *repo != "" {
			// Legacy mode: direct repository URL
			log.Println("Using legacy mode with direct repository URL")
			if *password == "" {
				log.Fatal("--password is required when using --repo (legacy mode)")
			}

			if err := config.StoreResticPassword(*password); err != nil {
				log.Printf("warning: store restic password failed: %v", err)
				if *passwordFile != "" {
					if _, err := writePasswordFile(*passwordFile, *password); err != nil {
						log.Fatalf("write password file: %v", err)
					}
					cfg.Restic.PasswordFile = *passwordFile
				} else {
					log.Fatal("restic password could not be stored (secretstore unavailable)")
				}
			} else {
				cfg.Restic.PasswordFile = ""
			}

			cfg.Restic.Repository = *repo
			if *server != "" {
				cfg.ServerURL = *server
			}
		} else {
			log.Fatal("Either --token (recommended) or --repo (legacy) is required")
		}

		// Update schedule and paths
		if *dailyAt != "" {
			cfg.Schedule.DailyAt = *dailyAt
		}
		cfg.Mode = *mode
		if len(includes) > 0 {
			cfg.Include = []string(includes)
		}
		if len(excludes) > 0 {
			cfg.Exclude = []string(excludes)
		}

		// Validate repository is set
		if cfg.Restic.Repository == "" {
			log.Fatal("Repository URL is required")
		}
		if len(cfg.Include) == 0 {
			log.Println("note: no --include provided; backups will likely do nothing until you add include paths")
		}

		// Write config
		if err := config.Write(cfgFile, cfg); err != nil {
			log.Fatalf("write config: %v", err)
		}

		// Install scheduler/service
		if err := install.InstallWithMode(cfgFile, *mode); err != nil {
			log.Fatalf("install scheduler: %v", err)
		}

		log.Println("install complete ✅")
		return

	case "uninstall":
		fs := flag.NewFlagSet(cmd, flag.ExitOnError)
		mode := fs.String("mode", "user", "Uninstall mode: user or system")
		keepConfig := fs.Bool("keep-config", true, "Keep config directory")
		purgeState := fs.Bool("purge-state", false, "Remove state/log directories")
		configPath := fs.String("config", "", "Config path override")
		if err := fs.Parse(os.Args[2:]); err != nil {
			log.Fatalf("parse flags: %v", err)
		}

		cfgFile, err = resolveConfigPathWithMode(*configPath, *mode)
		if err != nil {
			log.Fatalf("resolve config path: %v", err)
		}

		if err := install.UninstallWithMode(cfgFile, *mode); err != nil {
			log.Fatalf("uninstall failed: %v", err)
		}

		if !*keepConfig {
			cfgDir, err := paths.ConfigDir(*mode)
			if err == nil {
				_ = os.RemoveAll(cfgDir)
			}
		}

		if *purgeState {
			stateDir, err := paths.StateDir(*mode)
			if err == nil {
				_ = os.RemoveAll(stateDir)
			}
			logDir, err := paths.LogDir(*mode)
			if err == nil {
				_ = os.RemoveAll(logDir)
			}
		}

		log.Println("uninstall complete ✅")
		return

	case "upgrade":
		fs := flag.NewFlagSet(cmd, flag.ExitOnError)
		mode := fs.String("mode", "user", "Upgrade mode: user or system")
		newBinary := fs.String("binary", "", "Path to new xentz-agent binary")
		configPath := fs.String("config", "", "Config path override")
		if err := fs.Parse(os.Args[2:]); err != nil {
			log.Fatalf("parse flags: %v", err)
		}
		if *newBinary == "" {
			log.Fatal("--binary is required for upgrade")
		}
		cfgFile, err = resolveConfigPathWithMode(*configPath, *mode)
		if err != nil {
			log.Fatalf("resolve config path: %v", err)
		}
		if err := replaceBinary(*newBinary); err != nil {
			log.Fatalf("upgrade failed: %v", err)
		}
		if err := install.InstallWithMode(cfgFile, *mode); err != nil {
			log.Fatalf("restart failed: %v", err)
		}
		log.Println("upgrade complete ✅")
		return

	case "diagnostics":
		fs := flag.NewFlagSet(cmd, flag.ExitOnError)
		outPath := fs.String("out", "", "Output diagnostics bundle path")
		if err := fs.Parse(os.Args[2:]); err != nil {
			log.Fatalf("parse flags: %v", err)
		}
		if *outPath == "" {
			log.Fatal("--out is required")
		}
		if err := diagnostics.CreateBundle(*outPath); err != nil {
			log.Fatalf("diagnostics failed: %v", err)
		}
		log.Printf("diagnostics bundle created: %s", *outPath)
		return

	case "local-ui":
		fs := flag.NewFlagSet(cmd, flag.ExitOnError)
		addr := fs.String("addr", "127.0.0.1:9800", "Bind address")
		configPath := fs.String("config", "", "Config path override")
		if err := fs.Parse(os.Args[2:]); err != nil {
			log.Fatalf("parse flags: %v", err)
		}
		cfgFile, err = resolveConfigPathWithMode(*configPath, "")
		if err != nil {
			log.Fatalf("resolve config path: %v", err)
		}
		log.Printf("local UI listening on %s (header X-Local-Token required)", *addr)
		if err := localui.Start(*addr, cfgFile); err != nil {
			log.Fatalf("local UI failed: %v", err)
		}
		return

	case "service":
		fs := flag.NewFlagSet(cmd, flag.ExitOnError)
		if err := fs.Parse(os.Args[2:]); err != nil {
			log.Fatalf("parse flags: %v", err)
		}
		if len(fs.Args()) < 1 {
			log.Fatal("service requires subcommand: install|uninstall|start|stop")
		}
		if runtime.GOOS != "windows" {
			log.Fatal("service command is only supported on Windows")
		}
		switch fs.Args()[0] {
		case "install":
			cfg := ""
			if len(fs.Args()) > 1 {
				cfg = fs.Args()[1]
			}
			if cfg == "" {
				cfgFile, err = config.ResolvePath("")
				if err != nil {
					log.Fatalf("resolve config path: %v", err)
				}
				cfg = cfgFile
			}
			if err := install.WindowsServiceInstall(cfg); err != nil {
				log.Fatalf("service install failed: %v", err)
			}
			log.Println("service install complete ✅")
			return
		case "uninstall":
			if err := install.WindowsServiceUninstall(); err != nil {
				log.Fatalf("service uninstall failed: %v", err)
			}
			log.Println("service uninstall complete ✅")
			return
		case "start":
			_ = exec.Command("sc", "start", "XentzAgent").Run()
			return
		case "stop":
			_ = exec.Command("sc", "stop", "XentzAgent").Run()
			return
		case "run":
			runFS := flag.NewFlagSet("service run", flag.ExitOnError)
			runConfig := runFS.String("config", "", "Config path for service run")
			if err := runFS.Parse(fs.Args()[1:]); err != nil {
				log.Fatalf("parse flags: %v", err)
			}
			if *runConfig == "" {
				cfgFile, err = config.ResolvePath("")
				if err != nil {
					log.Fatalf("resolve config path: %v", err)
				}
				*runConfig = cfgFile
			}
			if err := windowsservice.RunService(*runConfig); err != nil {
				log.Fatalf("service run failed: %v", err)
			}
			return
		default:
			log.Fatal("unsupported service subcommand")
		}

	case "backup":
		fs := flag.NewFlagSet(cmd, flag.ExitOnError)
		configPath := fs.String("config", "", "Config path override")
		autoInit := fs.Bool("auto-init", false, "Automatically initialize repository if it doesn't exist (use with caution)")
		if err := fs.Parse(os.Args[2:]); err != nil {
			log.Fatalf("parse flags: %v", err)
		}

		cfgFile, err = config.ResolvePath(*configPath)
		if err != nil {
			log.Fatalf("resolve config path: %v", err)
		}

		// Read local config to get enrollment data (device_id, device_api_key, server_url)
		localCfg, err := config.Read(cfgFile)
		if err != nil {
			log.Fatalf("read config: %v", err)
		}
		if apiKey, err := config.GetDeviceAPIKey(localCfg); err == nil {
			localCfg.DeviceAPIKey = apiKey
		}

		// Fetch config from server (with fallback to cached config)
		var cfg config.Config
		if localCfg.DeviceAPIKey != "" && localCfg.ServerURL != "" {
			// Device is enrolled, fetch config from server
			fetchedCfg, fetchErr := config.LoadWithFallback(localCfg.ServerURL, localCfg.DeviceAPIKey)
			if fetchErr != nil {
				if strings.Contains(fetchErr.Error(), "authentication failed") || strings.Contains(fetchErr.Error(), "revoked") {
					if st, err := state.New(); err == nil {
						_ = st.SetRevoked(true)
					}
				}
				log.Fatalf("failed to load config: %v", fetchErr)
			}
			cfg = fetchedCfg
			// Preserve enrollment data from local config
			cfg.TenantID = localCfg.TenantID
			cfg.DeviceID = localCfg.DeviceID
			cfg.DeviceAPIKey = localCfg.DeviceAPIKey
			cfg.ServerURL = localCfg.ServerURL
			cfg.UserID = localCfg.UserID
			// Always preserve password file path from local config (it's a local file path)
			cfg.Restic.PasswordFile = localCfg.Restic.PasswordFile

			// Ensure password file exists and is valid (recover from server if missing)
			if err := config.EnsurePasswordFile(cfg, localCfg.ServerURL, localCfg.DeviceAPIKey); err != nil {
				log.Fatalf("password file validation failed: %v", err)
			}
		} else {
			// Legacy mode: use local config directly
			log.Println("Using local config (device not enrolled or legacy mode)")
			cfg = localCfg
		}

		// KILL-SWITCH: Final safety check - if device is disabled, exit immediately
		if cfg.Enabled != nil && !*cfg.Enabled {
			log.Fatalf("device is disabled by server (kill-switch activated). All operations stopped.")
		}

		// Initialize structured logger
		logger, err := logging.NewLogger(cfg.TenantID, cfg.DeviceID)
		if err != nil {
			log.Printf("warning: failed to initialize logger: %v", err)
			logger = nil // Continue without structured logging
		} else {
			defer logger.Close()
			logger.SetComponent("backup")
		}

		st, err := state.New()
		if err != nil {
			if logger != nil {
				logger.Error("state init failed", err, nil)
			}
			log.Fatalf("state init: %v", err)
		}

		// Track start time for reporting
		startTime := time.Now()

		if logger != nil {
			logger.Info("backup started", map[string]interface{}{
				"auto_init": *autoInit,
			})
		}

		ctx, cancel := context.WithTimeout(context.Background(), 6*time.Hour)
		defer cancel()

		res := backup.Run(ctx, cfg, *autoInit)
		if err := st.SaveLastRun(res); err != nil {
			if logger != nil {
				logger.Warn("failed to save last run", map[string]interface{}{
					"error": err.Error(),
				})
			}
			log.Printf("save last run: %v", err)
		}

		// Ship logs to control plane (non-blocking)
		if logger != nil && localCfg.DeviceID != "" && localCfg.DeviceAPIKey != "" && localCfg.ServerURL != "" {
			go func() {
				if err := logger.ShipLogs(localCfg.ServerURL, localCfg.DeviceAPIKey); err != nil {
					log.Printf("warning: failed to ship logs: %v", err)
				}
			}()
		}

		// Send reports (non-blocking)
		if localCfg.DeviceID != "" && localCfg.DeviceAPIKey != "" && localCfg.ServerURL != "" {
			// Send pending reports first (max 20, oldest first)
			_ = report.SendPendingReports(localCfg.ServerURL, localCfg.DeviceAPIKey, 20)

			// Create report for current run
			finishedTime := time.Now()
			reportStatus := "success"
			if res.Status == "error" {
				reportStatus = "failure"
			}
			backupReport := report.Report{
				DeviceID:       localCfg.DeviceID,
				ConfigRevision: cfg.ConfigRevision,
				Job:            "backup",
				StartedAt:      startTime.UTC().Format(time.RFC3339),
				FinishedAt:     finishedTime.UTC().Format(time.RFC3339),
				Status:         reportStatus,
				DurationMS:     res.DurationMS,
				FilesTotal:     res.FilesTotal,
				BytesTotal:     res.BytesTotal,
				DataAddedBytes: res.DataAddedBytes,
				SnapshotID:     res.SnapshotID,
			}
			if res.Error != "" {
				backupReport.Error = res.Error
			}

			// Send current report (spools if it fails)
			_ = report.SendReportWithSpool(localCfg.ServerURL, localCfg.DeviceAPIKey, backupReport)

			// Cleanup old reports periodically (every run for simplicity in MVP)
			_ = report.CleanupOldReports(30 * 24 * time.Hour)
		}

		if res.Status != "success" {
			if logger != nil {
				logger.Error("backup failed", fmt.Errorf("%s", res.Error), map[string]interface{}{
					"duration_ms": res.DurationMS,
				})
			}
			log.Printf("backup failed ❌: %s", res.Error)
			os.Exit(1)
		}

		if logger != nil {
			logger.Info("backup completed successfully", map[string]interface{}{
				"duration_ms":      res.DurationMS,
				"bytes_sent":       res.BytesSent,
				"files_total":      res.FilesTotal,
				"bytes_total":      res.BytesTotal,
				"data_added_bytes": res.DataAddedBytes,
				"snapshot_id":      res.SnapshotID,
			})
		}
		log.Printf("backup ok ✅: duration=%s bytes_sent=%d", res.Duration, res.BytesSent)
		return

	case "retention":
		fs := flag.NewFlagSet(cmd, flag.ExitOnError)
		configPath := fs.String("config", "", "Config path override")
		if err := fs.Parse(os.Args[2:]); err != nil {
			log.Fatalf("parse flags: %v", err)
		}

		cfgFile, err = config.ResolvePath(*configPath)
		if err != nil {
			log.Fatalf("resolve config path: %v", err)
		}

		// Read local config to get enrollment data (device_id, device_api_key, server_url)
		localCfg, err := config.Read(cfgFile)
		if err != nil {
			log.Fatalf("read config: %v", err)
		}
		if apiKey, err := config.GetDeviceAPIKey(localCfg); err == nil {
			localCfg.DeviceAPIKey = apiKey
		}

		// Fetch config from server (with fallback to cached config)
		var cfg config.Config
		if localCfg.DeviceAPIKey != "" && localCfg.ServerURL != "" {
			// Device is enrolled, fetch config from server
			fetchedCfg, fetchErr := config.LoadWithFallback(localCfg.ServerURL, localCfg.DeviceAPIKey)
			if fetchErr != nil {
				if strings.Contains(fetchErr.Error(), "authentication failed") || strings.Contains(fetchErr.Error(), "revoked") {
					if st, err := state.New(); err == nil {
						_ = st.SetRevoked(true)
					}
				}
				log.Fatalf("failed to load config: %v", fetchErr)
			}
			cfg = fetchedCfg
			// Preserve enrollment data from local config
			cfg.TenantID = localCfg.TenantID
			cfg.DeviceID = localCfg.DeviceID
			cfg.DeviceAPIKey = localCfg.DeviceAPIKey
			cfg.ServerURL = localCfg.ServerURL
			cfg.UserID = localCfg.UserID
			// Always preserve password file path from local config (it's a local file path)
			cfg.Restic.PasswordFile = localCfg.Restic.PasswordFile

			// Ensure password file exists and is valid (recover from server if missing)
			if err := config.EnsurePasswordFile(cfg, localCfg.ServerURL, localCfg.DeviceAPIKey); err != nil {
				log.Fatalf("password file validation failed: %v", err)
			}
		} else {
			// Legacy mode: use local config directly
			log.Println("Using local config (device not enrolled or legacy mode)")
			cfg = localCfg
		}

		// KILL-SWITCH: Final safety check - if device is disabled, exit immediately
		if cfg.Enabled != nil && !*cfg.Enabled {
			log.Fatalf("device is disabled by server (kill-switch activated). All operations stopped.")
		}

		// Initialize structured logger
		logger, err := logging.NewLogger(cfg.TenantID, cfg.DeviceID)
		if err != nil {
			log.Printf("warning: failed to initialize logger: %v", err)
			logger = nil // Continue without structured logging
		} else {
			defer logger.Close()
			logger.SetComponent("retention")
		}

		st, err := state.New()
		if err != nil {
			if logger != nil {
				logger.Error("state init failed", err, nil)
			}
			log.Fatalf("state init: %v", err)
		}

		// Track start time for reporting
		startTime := time.Now()

		if logger != nil {
			logger.Info("retention started", nil)
		}

		// Use a shorter timeout for retention - if it takes longer than 2 hours, something is wrong
		// The connectivity check will fail faster if the repository is unreachable
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Hour)
		defer cancel()

		res := backup.RunRetention(ctx, cfg)
		if err := st.SaveLastRetentionRun(res); err != nil {
			if logger != nil {
				logger.Warn("failed to save last retention run", map[string]interface{}{
					"error": err.Error(),
				})
			}
			log.Printf("save last retention run: %v", err)
		}

		// Ship logs to control plane (non-blocking)
		if logger != nil && localCfg.DeviceID != "" && localCfg.DeviceAPIKey != "" && localCfg.ServerURL != "" {
			go func() {
				if err := logger.ShipLogs(localCfg.ServerURL, localCfg.DeviceAPIKey); err != nil {
					log.Printf("warning: failed to ship logs: %v", err)
				}
			}()
		}

		// Send reports (non-blocking)
		if localCfg.DeviceID != "" && localCfg.DeviceAPIKey != "" && localCfg.ServerURL != "" {
			// Send pending reports first (max 20, oldest first)
			_ = report.SendPendingReports(localCfg.ServerURL, localCfg.DeviceAPIKey, 20)

			// Create report for current run (simpler payload, no file/byte stats)
			finishedTime := time.Now()
			reportStatus := "success"
			if res.Status == "error" {
				reportStatus = "failure"
			}
			retentionReport := report.Report{
				DeviceID:       localCfg.DeviceID,
				ConfigRevision: cfg.ConfigRevision,
				Job:            "retention",
				StartedAt:      startTime.UTC().Format(time.RFC3339),
				FinishedAt:     finishedTime.UTC().Format(time.RFC3339),
				Status:         reportStatus,
				DurationMS:     res.DurationMS,
			}
			if res.Error != "" {
				retentionReport.Error = res.Error
			}

			// Send current report (spools if it fails)
			_ = report.SendReportWithSpool(localCfg.ServerURL, localCfg.DeviceAPIKey, retentionReport)

			// Cleanup old reports periodically
			_ = report.CleanupOldReports(30 * 24 * time.Hour)
		}

		// Ship logs to control plane (non-blocking)
		if logger != nil && localCfg.DeviceID != "" && localCfg.DeviceAPIKey != "" && localCfg.ServerURL != "" {
			go func() {
				if err := logger.ShipLogs(localCfg.ServerURL, localCfg.DeviceAPIKey); err != nil {
					log.Printf("warning: failed to ship logs: %v", err)
				}
			}()
		}

		if res.Status != "success" {
			if logger != nil {
				logger.Error("retention failed", fmt.Errorf("%s", res.Error), map[string]interface{}{
					"duration_ms": res.DurationMS,
				})
			}
			log.Printf("retention failed ❌: %s", res.Error)
			os.Exit(1)
		}

		if logger != nil {
			logger.Info("retention completed successfully", map[string]interface{}{
				"duration_ms": res.DurationMS,
			})
		}
		log.Printf("retention ok ✅: duration=%s", res.Duration)
		return

	case "status":
		fs := flag.NewFlagSet(cmd, flag.ExitOnError)
		configPath := fs.String("config", "", "Config path override")
		if err := fs.Parse(os.Args[2:]); err != nil {
			log.Fatalf("parse flags: %v", err)
		}

		st, err := state.New()
		if err != nil {
			log.Fatalf("state init: %v", err)
		}

		// Load config revision (fallback to cached config)
		cfgFile, err := config.ResolvePath(*configPath)
		if err != nil {
			log.Printf("warning: resolve config path: %v", err)
		}
		configRevision := 0
		if cfgFile != "" {
			if cfg, err := config.Read(cfgFile); err == nil {
				configRevision = cfg.ConfigRevision
			}
		}
		if configRevision == 0 {
			if cachedCfg, err := config.ReadCached(); err == nil {
				configRevision = cachedCfg.ConfigRevision
			}
		}

		// Spool stats
		spoolCount, spoolBytes, _ := report.SpoolStats()

		// Revoked state
		revoked := false
		if agentState, ok, _ := st.LoadAgentState(); ok {
			revoked = agentState.Revoked
		}

		// Show backup status
		last, ok, err := st.LoadLastRun()
		if err != nil {
			log.Fatalf("load last run: %v", err)
		}
		if !ok {
			fmt.Println("No backups have run yet.")
		} else {
			fmt.Printf("Last backup:\n  status: %s\n  time:   %s\n  dur:    %s\n  bytes:  %d\n  error:  %s\n",
				last.Status, last.TimeUTC, last.Duration, last.BytesSent, last.Error)
		}

		// Show retention status
		lastRetention, ok, err := st.LoadLastRetentionRun()
		if err != nil {
			log.Fatalf("load last retention run: %v", err)
		}
		if ok {
			fmt.Println("")
			fmt.Printf("Last retention:\n  status: %s\n  time:   %s\n  dur:    %s\n  error:  %s\n",
				lastRetention.Status, lastRetention.TimeUTC, lastRetention.Duration, lastRetention.Error)
		}

		fmt.Println("")
		fmt.Printf("Config revision: %d\n", configRevision)
		fmt.Printf("Spool backlog: %d report(s), %d bytes\n", spoolCount, spoolBytes)
		fmt.Printf("Revoked: %v\n", revoked)
		return

	case "config":
		fs := flag.NewFlagSet(cmd, flag.ExitOnError)
		configPath := fs.String("config", "", "Config path override")
		var addIncludes multiFlag
		var removeIncludes multiFlag
		var addExcludes multiFlag
		var removeExcludes multiFlag
		listIncludes := fs.Bool("list-includes", false, "List current include paths")
		listExcludes := fs.Bool("list-excludes", false, "List current exclude paths")
		listAll := fs.Bool("list-all", false, "List both include and exclude paths")
		fs.Var(&addIncludes, "add-include", "Add include path (repeatable)")
		fs.Var(&removeIncludes, "remove-include", "Remove include path (repeatable)")
		fs.Var(&addExcludes, "add-exclude", "Add exclude pattern (repeatable)")
		fs.Var(&removeExcludes, "remove-exclude", "Remove exclude pattern (repeatable)")

		if err := fs.Parse(os.Args[2:]); err != nil {
			log.Fatalf("parse flags: %v", err)
		}

		cfgFile, err = config.ResolvePath(*configPath)
		if err != nil {
			log.Fatalf("resolve config path: %v", err)
		}

		// Load local config
		localCfg, err := config.Read(cfgFile)
		if err != nil {
			log.Fatalf("read config: %v", err)
		}

		// Helper function to normalize path (expand ~, make absolute)
		normalizePath := func(p string) string {
			// Expand ~
			if strings.HasPrefix(p, "~/") {
				home, err := os.UserHomeDir()
				if err == nil {
					p = filepath.Join(home, p[2:])
				}
			} else if p == "~" {
				home, err := os.UserHomeDir()
				if err == nil {
					p = home
				}
			}
			// Make absolute
			if abs, err := filepath.Abs(p); err == nil {
				return abs
			}
			return p
		}

		// Helper function to check if path exists (for includes)
		checkPathExists := func(p string) bool {
			_, err := os.Stat(p)
			return err == nil
		}

		// Helper function to remove duplicates from slice
		removeDuplicates := func(slice []string) []string {
			seen := make(map[string]bool)
			result := []string{}
			for _, item := range slice {
				if !seen[item] {
					seen[item] = true
					result = append(result, item)
				}
			}
			return result
		}

		// Helper function to remove item from slice
		removeFromSlice := func(slice []string, item string) []string {
			result := []string{}
			for _, s := range slice {
				if s != item {
					result = append(result, s)
				}
			}
			return result
		}

		// If just listing, show current config and exit
		if *listAll || *listIncludes || *listExcludes {
			var cfg config.Config
			if localCfg.DeviceAPIKey != "" && localCfg.ServerURL != "" {
				// Fetch from server
				fetchedCfg, fetchErr := config.LoadWithFallback(localCfg.ServerURL, localCfg.DeviceAPIKey)
				if fetchErr != nil {
					log.Printf("warning: failed to fetch config from server: %v", fetchErr)
					log.Println("Showing local config instead...")
					cfg = localCfg
				} else {
					cfg = fetchedCfg
				}
			} else {
				cfg = localCfg
			}

			if *listAll || *listIncludes {
				fmt.Println("Include paths:")
				if len(cfg.Include) == 0 {
					fmt.Println("  (none)")
				} else {
					for _, p := range cfg.Include {
						fmt.Printf("  %s\n", p)
					}
				}
			}

			if *listAll || *listExcludes {
				if *listAll {
					fmt.Println("")
				}
				fmt.Println("Exclude patterns:")
				if len(cfg.Exclude) == 0 {
					fmt.Println("  (none)")
				} else {
					for _, p := range cfg.Exclude {
						fmt.Printf("  %s\n", p)
					}
				}
			}
			return
		}

		// Check if any operations are requested
		if len(addIncludes) == 0 && len(removeIncludes) == 0 && len(addExcludes) == 0 && len(removeExcludes) == 0 {
			log.Fatal("No operations specified. Use --add-include, --remove-include, --add-exclude, --remove-exclude, or --list-all")
		}

		// Determine if device is enrolled
		isEnrolled := localCfg.DeviceAPIKey != "" && localCfg.ServerURL != ""

		var currentCfg config.Config
		if isEnrolled {
			// Fetch current config from server
			fetchedCfg, fetchErr := config.LoadWithFallback(localCfg.ServerURL, localCfg.DeviceAPIKey)
			if fetchErr != nil {
				log.Fatalf("failed to fetch config from server: %v", fetchErr)
			}
			currentCfg = fetchedCfg
		} else {
			// Use local config
			currentCfg = localCfg
		}

		// Apply operations to include paths
		newIncludes := make([]string, len(currentCfg.Include))
		copy(newIncludes, currentCfg.Include)

		// Add include paths
		for _, path := range addIncludes {
			normalized := normalizePath(path)
			// Check for duplicates
			duplicate := false
			for _, existing := range newIncludes {
				if existing == normalized {
					duplicate = true
					log.Printf("warning: include path already exists: %s", normalized)
					break
				}
			}
			if !duplicate {
				// Warn if path doesn't exist (but allow it - user might create it later)
				if !checkPathExists(normalized) {
					log.Printf("warning: include path does not exist: %s (will be added anyway)", normalized)
				}
				newIncludes = append(newIncludes, normalized)
			}
		}

		// Remove include paths
		for _, path := range removeIncludes {
			normalized := normalizePath(path)
			found := false
			for _, existing := range newIncludes {
				if existing == normalized {
					found = true
					break
				}
			}
			if found {
				newIncludes = removeFromSlice(newIncludes, normalized)
				log.Printf("removed include path: %s", normalized)
			} else {
				log.Printf("warning: include path not found: %s", normalized)
			}
		}

		// Remove duplicates
		newIncludes = removeDuplicates(newIncludes)

		// Apply operations to exclude paths
		newExcludes := make([]string, len(currentCfg.Exclude))
		copy(newExcludes, currentCfg.Exclude)

		// Add exclude patterns
		for _, pattern := range addExcludes {
			// Check for duplicates
			duplicate := false
			for _, existing := range newExcludes {
				if existing == pattern {
					duplicate = true
					log.Printf("warning: exclude pattern already exists: %s", pattern)
					break
				}
			}
			if !duplicate {
				newExcludes = append(newExcludes, pattern)
			}
		}

		// Remove exclude patterns
		for _, pattern := range removeExcludes {
			found := false
			for _, existing := range newExcludes {
				if existing == pattern {
					found = true
					break
				}
			}
			if found {
				newExcludes = removeFromSlice(newExcludes, pattern)
				log.Printf("removed exclude pattern: %s", pattern)
			} else {
				log.Printf("warning: exclude pattern not found: %s", pattern)
			}
		}

		// Remove duplicates
		newExcludes = removeDuplicates(newExcludes)

		// Validate: must have at least one include path
		if len(newIncludes) == 0 {
			log.Fatal("error: at least one include path is required")
		}

		// Update config
		if isEnrolled {
			// Update on server
			log.Println("Updating configuration on server...")
			updatedCfg, err := config.UpdateConfigOnServer(localCfg.ServerURL, localCfg.DeviceAPIKey, newIncludes, newExcludes)
			if err != nil {
				log.Fatalf("failed to update config on server: %v", err)
			}
			log.Println("✓ Configuration updated on server")
			log.Printf("  Include paths: %d", len(updatedCfg.Include))
			log.Printf("  Exclude patterns: %d", len(updatedCfg.Exclude))
		} else {
			// Update local config
			currentCfg.Include = newIncludes
			currentCfg.Exclude = newExcludes
			if err := config.Write(cfgFile, currentCfg); err != nil {
				log.Fatalf("failed to write config: %v", err)
			}
			log.Println("✓ Configuration updated locally")
			log.Printf("  Include paths: %d", len(newIncludes))
			log.Printf("  Exclude patterns: %d", len(newExcludes))
		}

		return

	default:
		usage()
		os.Exit(2)
	}
}
