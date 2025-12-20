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
	"xentz-agent/internal/install"
	"xentz-agent/internal/state"
)

func usage() {
	fmt.Println(`xentz-agent (thin MVP)

Commands:
  install    Install config + scheduled task (macOS: launchd, Windows: Task Scheduler, Linux: systemd/cron)
  backup     Run one backup now (used by scheduler)
  retention  Run retention/prune policy (forget old snapshots)
  status     Show last run status

Examples:
  xentz-agent install --repo rest:https://... --password "..." --daily-at 02:00 --include "/Users/me/Documents"
  xentz-agent backup
  xentz-agent retention
  xentz-agent status

Flags (install):
  --server        Proxy base URL (optional for MVP if you only run local backups)
  --daily-at      Time in HH:MM (24h), default 02:00
  --repo          Restic repository URL (required, e.g., rest:https://...)
  --password      Restic repository password (MVP; stored locally)
  --password-file Path to restic password file (optional, default: ~/.xentz-agent/restic.pw)
  --include       Repeatable. Add include paths. Example: --include "/Users/me/Documents" --include "/Users/me/Pictures"
  --exclude       Repeatable. Add exclude globs.
  --config        Config path override (default: ~/.xentz-agent/config.json)

Note: Retention policy must be configured in config.json before running 'retention' command.
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
		server := fs.String("server", "", "Proxy base URL")
		dailyAt := fs.String("daily-at", "02:00", "Daily time HH:MM (24h)")
		configPath := fs.String("config", "", "Config path override")
		repo := fs.String("repo", "", "Restic repository URL (rest:https://...)")
		password := fs.String("password", "", "Restic repository password (MVP; stored locally)")
		passwordFile := fs.String("password-file", "", "Path to restic password file (optional)")

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

		pwFile := *passwordFile
		if pwFile == "" {
			home, _ := os.UserHomeDir()
			pwFile = filepath.Join(home, ".xentz-agent", "restic.pw")
		}

		if *password != "" {
			if err := os.MkdirAll(filepath.Dir(pwFile), 0o700); err != nil {
				log.Fatalf("password dir: %v", err)
			}
			if err := os.WriteFile(pwFile, []byte(*password+"\n"), 0o600); err != nil {
				log.Fatalf("write password file: %v", err)
			}
		}
		cfg := config.Config{
			ServerURL: *server,
			Schedule:  config.Schedule{DailyAt: *dailyAt},
			Include:   []string(includes),
			Exclude:   []string(excludes),
			Restic: config.Restic{
				Repository:   *repo,
				PasswordFile: pwFile,
			},
		}
		if cfg.Restic.Repository == "" {
			log.Fatal("--repo is required for install")
		}
		if len(cfg.Include) == 0 {
			log.Println("note: no --include provided; backups will likely do nothing until you add include paths")
		}

		if err := config.Write(cfgFile, cfg); err != nil {
			log.Fatalf("write config: %v", err)
		}

		if err := install.Install(cfgFile); err != nil {
			log.Fatalf("install scheduler: %v", err)
		}

		log.Println("install complete ✅")
		return

	case "backup":
		fs := flag.NewFlagSet(cmd, flag.ExitOnError)
		configPath := fs.String("config", "", "Config path override")
		if err := fs.Parse(os.Args[2:]); err != nil {
			log.Fatalf("parse flags: %v", err)
		}

		cfgFile, err = config.ResolvePath(*configPath)
		if err != nil {
			log.Fatalf("resolve config path: %v", err)
		}

		cfg, err := config.Read(cfgFile)
		if err != nil {
			log.Fatalf("read config: %v", err)
		}

		st, err := state.New()
		if err != nil {
			log.Fatalf("state init: %v", err)
		}

		ctx, cancel := context.WithTimeout(context.Background(), 6*time.Hour)
		defer cancel()

		res := backup.Run(ctx, cfg)
		if err := st.SaveLastRun(res); err != nil {
			log.Printf("save last run: %v", err)
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

		cfg, err := config.Read(cfgFile)
		if err != nil {
			log.Fatalf("read config: %v", err)
		}
		st, err := state.New()
		if err != nil {
			log.Fatalf("state init: %v", err)
		}

		ctx, cancel := context.WithTimeout(context.Background(), 12*time.Hour)
		defer cancel()

		res := backup.RunRetention(ctx, cfg)
		if err := st.SaveLastRetentionRun(res); err != nil {
			log.Printf("save last retention run: %v", err)
		}
		if res.Status != "success" {
			log.Printf("retention failed ❌: %s", res.Error)
			os.Exit(1)
		}
		log.Printf("retention ok ✅: duration=%s", res.Duration)
		return

	case "status":
		fs := flag.NewFlagSet(cmd, flag.ExitOnError)
		configPath := fs.String("config", "", "Config path override")
		if err := fs.Parse(os.Args[2:]); err != nil {
			log.Fatalf("parse flags: %v", err)
		}

		cfgFile, err = config.ResolvePath(*configPath)
		if err != nil {
			log.Fatalf("resolve config path: %v", err)
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