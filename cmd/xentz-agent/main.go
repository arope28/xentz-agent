package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	"xentz-agent/internal/backup"
	"xentz-agent/internal/config"
	"xentz-agent/internal/enroll"
	"xentz-agent/internal/install"
	"xentz-agent/internal/report"
	"xentz-agent/internal/state"
)

func usage() {
	fmt.Print(`xentz-agent - Backup Agent

Commands:
  install    Install config + scheduled task (macOS: launchd, Windows: Task Scheduler, Linux: systemd/cron)
  backup     Run one backup now (used by scheduler)
  retention  Run retention/prune policy (forget old snapshots)
  status     Show last run status

Examples:
  # Token-based enrollment (recommended):
  xentz-agent install --token <install-token> --server https://control-plane.example.com --daily-at 02:00 --include "/Users/me/Documents"
  
  # Legacy mode (direct repository):
  xentz-agent install --repo rest:https://... --password "..." --daily-at 02:00 --include "/Users/me/Documents"
  
  xentz-agent backup
  xentz-agent backup --auto-init  # Auto-initialize repository if missing (use with caution)
  xentz-agent retention
  xentz-agent status

Flags (backup):
  --auto-init    Automatically initialize repository if it doesn't exist (default: false)
                 WARNING: Only use if you're certain the repository URL is correct.
                 Without this flag, backup will fail if repository doesn't exist.

Flags (install):
  --token         Install token for enrollment (recommended, provided by control plane)
  --server        Control plane base URL (required with --token)
  --daily-at      Time in HH:MM (24h), default 02:00
  --repo          Restic repository URL (legacy mode, use --token instead)
  --password      Restic repository password (optional if server provides via enrollment)
  --password-file Path to restic password file (optional, default: ~/.xentz-agent/restic.pw)
  --include       Repeatable. Add include paths. Example: --include "/Users/me/Documents" --include "/Users/me/Pictures"
  --exclude       Repeatable. Add exclude globs.
  --config        Config path override (default: ~/.xentz-agent/config.json)

Note: With token-based enrollment, configuration (including retention policy) is fetched from the server on each run.
      In legacy mode, retention policy must be configured in config.json before running 'retention' command.
`)
}

type multiFlag []string

func (m *multiFlag) String() string { return fmt.Sprint([]string(*m)) }
func (m *multiFlag) Set(v string) error {
	*m = append(*m, v)
	return nil
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
		configPath := fs.String("config", "", "Config path override")
		token := fs.String("token", "", "Install token for enrollment (primary method)")
		repo := fs.String("repo", "", "Restic repository URL (legacy mode, use --token instead)")
		password := fs.String("password", "", "Restic repository password (optional if server provides)")
		passwordFile := fs.String("password-file", "", "Path to restic password file (optional, default: ~/.xentz-agent/restic.pw)")

		var includes multiFlag
		var excludes multiFlag
		fs.Var(&includes, "include", "Include path (repeatable)")
		fs.Var(&excludes, "exclude", "Exclude glob (repeatable)")

		if err := fs.Parse(os.Args[2:]); err != nil {
			log.Fatalf("parse flags: %v", err)
		}

		cfgFile, err = config.ResolvePath(*configPath)
		if err != nil {
			log.Fatalf("resolve config path: %v", err)
		}

		// Try to load existing config to check if already enrolled
		var cfg config.Config
		if existingCfg, err := config.Read(cfgFile); err == nil {
			cfg = existingCfg
		}

		// Determine user ID
		home, err := os.UserHomeDir()
		if err != nil {
			log.Fatalf("get home directory: %v", err)
		}
		configDir := filepath.Join(home, ".xentz-agent")
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
				enrollmentResult, err := enroll.Enroll(*token, *server)
				if err != nil {
					log.Fatalf("enrollment failed: %v", err)
				}

				// Store enrollment data (do not store InstallToken after enrollment)
				cfg.TenantID = enrollmentResult.TenantID
				cfg.DeviceID = enrollmentResult.DeviceID
				cfg.DeviceAPIKey = enrollmentResult.DeviceAPIKey
				cfg.ServerURL = *server
				cfg.Restic.Repository = enrollmentResult.RepoPath

				log.Printf("Enrollment successful:")
				log.Printf("  Tenant ID: %s", cfg.TenantID)
				log.Printf("  Device ID: %s", cfg.DeviceID)
				log.Printf("  Repository: %s", cfg.Restic.Repository)

				// Handle password from server or user input
				if enrollmentResult.Password != "" {
					// Server provided password
					if *passwordFile == "" {
						pwFile := filepath.Join(home, ".xentz-agent", "restic.pw")
						passwordFile = &pwFile
					}
					if err := os.MkdirAll(filepath.Dir(*passwordFile), 0o700); err != nil {
						log.Fatalf("password dir: %v", err)
					}
					if err := os.WriteFile(*passwordFile, []byte(enrollmentResult.Password+"\n"), 0o600); err != nil {
						log.Fatalf("write password file: %v", err)
					}
					cfg.Restic.PasswordFile = *passwordFile
				} else if *password != "" {
					// User provided password
					if *passwordFile == "" {
						pwFile := filepath.Join(home, ".xentz-agent", "restic.pw")
						passwordFile = &pwFile
					}
					if err := os.MkdirAll(filepath.Dir(*passwordFile), 0o700); err != nil {
						log.Fatalf("password dir: %v", err)
					}
					if err := os.WriteFile(*passwordFile, []byte(*password+"\n"), 0o600); err != nil {
						log.Fatalf("write password file: %v", err)
					}
					cfg.Restic.PasswordFile = *passwordFile
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

			pwFile := *passwordFile
			if pwFile == "" {
				pwFile = filepath.Join(home, ".xentz-agent", "restic.pw")
			}

			if err := os.MkdirAll(filepath.Dir(pwFile), 0o700); err != nil {
				log.Fatalf("password dir: %v", err)
			}
			if err := os.WriteFile(pwFile, []byte(*password+"\n"), 0o600); err != nil {
				log.Fatalf("write password file: %v", err)
			}

			cfg.Restic.Repository = *repo
			cfg.Restic.PasswordFile = pwFile
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
		if cfg.Restic.PasswordFile == "" {
			log.Fatal("Password file is required")
		}

		if len(cfg.Include) == 0 {
			log.Println("note: no --include provided; backups will likely do nothing until you add include paths")
		}

		// Write config
		if err := config.Write(cfgFile, cfg); err != nil {
			log.Fatalf("write config: %v", err)
		}

		// Install scheduler
		if err := install.Install(cfgFile); err != nil {
			log.Fatalf("install scheduler: %v", err)
		}

		log.Println("install complete ✅")
		return

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

		// Fetch config from server (with fallback to cached config)
		var cfg config.Config
		if localCfg.DeviceAPIKey != "" && localCfg.ServerURL != "" {
			// Device is enrolled, fetch config from server
			fetchedCfg, fetchErr := config.LoadWithFallback(localCfg.ServerURL, localCfg.DeviceAPIKey)
			if fetchErr != nil {
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
		} else {
			// Legacy mode: use local config directly
			log.Println("Using local config (device not enrolled or legacy mode)")
			cfg = localCfg
		}

		st, err := state.New()
		if err != nil {
			log.Fatalf("state init: %v", err)
		}

		// Track start time for reporting
		startTime := time.Now()

		ctx, cancel := context.WithTimeout(context.Background(), 6*time.Hour)
		defer cancel()

		res := backup.Run(ctx, cfg, *autoInit)
		if err := st.SaveLastRun(res); err != nil {
			log.Printf("save last run: %v", err)
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
			log.Printf("backup failed ❌: %s", res.Error)
			os.Exit(1)
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

		// Fetch config from server (with fallback to cached config)
		var cfg config.Config
		if localCfg.DeviceAPIKey != "" && localCfg.ServerURL != "" {
			// Device is enrolled, fetch config from server
			fetchedCfg, fetchErr := config.LoadWithFallback(localCfg.ServerURL, localCfg.DeviceAPIKey)
			if fetchErr != nil {
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
		} else {
			// Legacy mode: use local config directly
			log.Println("Using local config (device not enrolled or legacy mode)")
			cfg = localCfg
		}

		st, err := state.New()
		if err != nil {
			log.Fatalf("state init: %v", err)
		}

		// Track start time for reporting
		startTime := time.Now()

		// Use a shorter timeout for retention - if it takes longer than 2 hours, something is wrong
		// The connectivity check will fail faster if the repository is unreachable
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Hour)
		defer cancel()

		res := backup.RunRetention(ctx, cfg)
		if err := st.SaveLastRetentionRun(res); err != nil {
			log.Printf("save last retention run: %v", err)
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
				DeviceID:   localCfg.DeviceID,
				Job:        "retention",
				StartedAt:  startTime.UTC().Format(time.RFC3339),
				FinishedAt: finishedTime.UTC().Format(time.RFC3339),
				Status:     reportStatus,
				DurationMS: res.DurationMS,
			}
			if res.Error != "" {
				retentionReport.Error = res.Error
			}

			// Send current report (spools if it fails)
			_ = report.SendReportWithSpool(localCfg.ServerURL, localCfg.DeviceAPIKey, retentionReport)

			// Cleanup old reports periodically
			_ = report.CleanupOldReports(30 * 24 * time.Hour)
		}

		if res.Status != "success" {
			log.Printf("retention failed ❌: %s", res.Error)
			os.Exit(1)
		}
		log.Printf("retention ok ✅: duration=%s", res.Duration)
		return

	case "status":
		fs := flag.NewFlagSet(cmd, flag.ExitOnError)
		_ = fs.String("config", "", "Config path override (unused, kept for compatibility)")
		if err := fs.Parse(os.Args[2:]); err != nil {
			log.Fatalf("parse flags: %v", err)
		}

		st, err := state.New()
		if err != nil {
			log.Fatalf("state init: %v", err)
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
		return

	default:
		usage()
		os.Exit(2)
	}
}
