package cli

import (
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"xentz-agent/internal/config"
	"xentz-agent/internal/enroll"
	"xentz-agent/internal/identity"
	"xentz-agent/internal/install"
	"xentz-agent/internal/paths"
)

type installMultiFlag []string

func (m *installMultiFlag) String() string { return fmt.Sprint([]string(*m)) }
func (m *installMultiFlag) Set(v string) error {
	*m = append(*m, v)
	return nil
}

func RunInstall(args []string) error {
	fs := newFlagSet("install")
	server := fs.String("server", "", "Control plane base URL (required for token-based enrollment)")
	dailyAt := fs.String("daily-at", "02:00", "Daily time HH:MM (24h)")
	mode := fs.String("mode", "user", "Install mode: user or system")
	force := fs.Bool("force", false, "Replace existing enrollment (clear stored API key + local identity before enroll)")
	configPath := fs.String("config", "", "Config path override")
	token := fs.String("token", "", "Install token for enrollment (primary method)")
	repo := fs.String("repo", "", "Restic repository URL (legacy mode, use --token instead)")
	password := fs.String("password", "", "Restic repository password (optional if server provides)")
	passwordFile := fs.String("password-file", "", "Path to restic password file (optional, default: <CONFIG_DIR>/restic.pw)")

	var includes installMultiFlag
	var excludes installMultiFlag
	fs.Var(&includes, "include", "Include path (repeatable)")
	fs.Var(&excludes, "exclude", "Exclude glob (repeatable)")

	if help, err := parseFlags(fs, args); err != nil {
		return err
	} else if help {
		return nil
	}

	cfgFile, err := resolveConfigPathWithMode(*configPath, *mode)
	if err != nil {
		return fmt.Errorf("resolve config path: %w", err)
	}

	// Validate the schedule before anything is enrolled or written. A bad
	// --daily-at used to be caught by the scheduler install at the very end,
	// leaving the device enrolled with no working schedule.
	if strings.TrimSpace(*dailyAt) != "" {
		if err := install.ValidateDailyAt(*dailyAt); err != nil {
			return fmt.Errorf("invalid --daily-at %q: %w (expected HH:MM in 24h, e.g. 02:00)", *dailyAt, err)
		}
	}

	warnIfResticMissing()

	if *password != "" {
		fmt.Println("⚠  --password is visible in shell history and process listings.")
		fmt.Printf("    Prefer:  %s=<password> xentz-agent install ...\n", envResticPassword)
	}

	var cfg config.Config
	if existingCfg, err := config.Read(cfgFile); err == nil {
		cfg = existingCfg
	}

	configDir, err := paths.ConfigDir(*mode)
	if err != nil {
		return fmt.Errorf("resolve config dir: %w", err)
	}
	userID, err := enroll.GetOrCreateUserID(configDir)
	if err != nil {
		return fmt.Errorf("get user ID: %w", err)
	}
	cfg.UserID = userID

	principalID, err := identity.GetOrCreatePrincipalID(*mode)
	if err != nil {
		return fmt.Errorf("get principal ID: %w", err)
	}

	if *token != "" {
		if *server == "" {
			return fmt.Errorf("--server is required when using --token")
		}
		if *force {
			log.Println("⚠ --force specified: clearing stored enrollment identity and API key before re-enrolling.")
			if err := ResetEnrollment(*mode, cfgFile); err != nil {
				return fmt.Errorf("force reset failed: %w", err)
			}
			cfg = config.Config{}
			cfg.UserID = userID
		}

		if enroll.IsEnrolled(cfg.TenantID, cfg.DeviceID) {
			log.Println("Device is already enrolled; install token will NOT be used.")
			log.Println("Use --force to replace enrollment with a new token.")
			log.Printf("  Tenant ID: %s", cfg.TenantID)
			log.Printf("  Device ID: %s", cfg.DeviceID)

			if *server != "" && cfg.ServerURL != *server {
				log.Printf("  Updating server URL: %s -> %s", cfg.ServerURL, *server)
				cfg.ServerURL = *server
			}

			_ = identity.Save(*mode, identity.Identity{
				ServerURL:   cfg.ServerURL,
				TenantID:    cfg.TenantID,
				DeviceID:    cfg.DeviceID,
				PrincipalID: principalID,
				Mode:        *mode,
			})
		} else {
			log.Println("Enrolling device with control plane...")
			enrollmentResult, err := enroll.Enroll(*token, *server, includes, principalID, userID)
			if err != nil {
				return fmt.Errorf("enrollment failed: %w", err)
			}

			cfg.TenantID = enrollmentResult.TenantID
			cfg.DeviceID = enrollmentResult.DeviceID
			if err := config.StoreDeviceAPIKeyForMode(enrollmentResult.DeviceAPIKey, *mode); err != nil {
				return fmt.Errorf("store device api key: %w", err)
			}
			cfg.DeviceAPIKey = ""
			cfg.ServerURL = *server
			cfg.Restic.Repository = enrollmentResult.RepoPath

			_ = identity.Save(*mode, identity.Identity{
				ServerURL:   cfg.ServerURL,
				TenantID:    cfg.TenantID,
				DeviceID:    cfg.DeviceID,
				PrincipalID: principalID,
				Mode:        *mode,
			})

			log.Printf("Enrollment successful:")
			log.Printf("  Tenant ID: %s", cfg.TenantID)
			log.Printf("  Device ID: %s", cfg.DeviceID)
			log.Printf("  Repository: %s", cfg.Restic.Repository)

			if enrollmentResult.Password != "" {
				if err := storeInstallPassword(enrollmentResult.Password, *passwordFile, &cfg); err != nil {
					return err
				}
			} else if pw := resolveResticPassword(*password); pw != "" {
				if err := storeInstallPassword(pw, *passwordFile, &cfg); err != nil {
					return err
				}
			} else {
				return fmt.Errorf("password required: the server did not provide one. Pass --password, or set %s=<password> to keep it out of shell history", envResticPassword)
			}
		}
	} else if *repo != "" {
		log.Println("Using legacy mode with direct repository URL")
		pw := resolveResticPassword(*password)
		if pw == "" {
			return fmt.Errorf("--password is required when using --repo (legacy mode). You can also set %s=<password>", envResticPassword)
		}
		if err := storeInstallPassword(pw, *passwordFile, &cfg); err != nil {
			return err
		}
		cfg.Restic.Repository = *repo
		if *server != "" {
			cfg.ServerURL = *server
		}
	} else {
		return fmt.Errorf("either --token (recommended) or --repo (legacy) is required")
	}

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

	if cfg.Restic.Repository == "" {
		return fmt.Errorf("repository URL is required")
	}
	if len(cfg.Include) == 0 {
		log.Println("note: no --include provided; backups will likely do nothing until you add include paths")
	}

	if err := config.Write(cfgFile, cfg); err != nil {
		return fmt.Errorf("write config: %w", err)
	}

	if err := install.InstallWithMode(cfgFile, *mode); err != nil {
		return fmt.Errorf("install scheduler: %w", err)
	}

	log.Println("install complete ✅")
	printNextSteps(*mode, cfg.Schedule.DailyAt, cfg.Include)
	return nil
}

// envResticPassword is the environment variable users can set to supply the
// repository password without exposing it in shell history or ps output.
const envResticPassword = "XENTZ_AGENT_RESTIC_PASSWORD"

// resolveResticPassword returns the password from the flag if given, else from
// the environment variable. Whitespace-only values are treated as unset.
func resolveResticPassword(flagValue string) string {
	if strings.TrimSpace(flagValue) != "" {
		return flagValue
	}
	return strings.TrimSpace(os.Getenv(envResticPassword))
}

// printNextSteps spells out what to do after install. Previously the command
// ended at "install complete", leaving users to guess how to verify it works.
func printNextSteps(mode, dailyAt string, includePaths []string) {
	steps := []string{}
	if len(includePaths) == 0 {
		steps = append(steps,
			`Add a folder to back up:  xentz-agent config --add-include "/path/to/folder"`,
			"Run your first backup now:   xentz-agent backup")
	} else {
		steps = append(steps, "Run your first backup now:   xentz-agent backup")
	}
	steps = append(steps,
		"Check the last run:           xentz-agent status",
		"Open the dashboard:           xentz-agent local-ui",
		"Verify the whole setup:       xentz-agent doctor --check-server")

	fmt.Println()
	fmt.Printf("Backups are scheduled %s.\n", describeSchedulePhrase(dailyAt))
	fmt.Println("Next steps:")
	for _, s := range steps {
		fmt.Println("  " + s)
	}
	fmt.Printf("\nInstalled in %s mode. Uninstall with:  xentz-agent uninstall --mode %s\n", mode, mode)
}

// storeInstallPassword persists the restic password and records where it went.
func storeInstallPassword(password, passwordFile string, cfg *config.Config) error {
	if err := config.StoreResticPassword(password); err != nil {
		log.Printf("warning: store restic password failed: %v", err)
		if passwordFile != "" {
			if _, err := WritePasswordFile(passwordFile, password); err != nil {
				return fmt.Errorf("write password file: %w", err)
			}
			cfg.Restic.PasswordFile = passwordFile
			return nil
		}
		return fmt.Errorf("restic password could not be stored (secretstore unavailable)")
	}
	cfg.Restic.PasswordFile = ""
	return nil
}

func ResetEnrollment(mode, cfgFile string) error {
	effectiveMode := strings.TrimSpace(mode)
	if effectiveMode == "" {
		effectiveMode = string(paths.ResolveMode(""))
	}
	cfgDir, err := paths.ConfigDir(effectiveMode)
	if err != nil {
		return fmt.Errorf("resolve config dir: %w", err)
	}
	var errs []string
	if err := os.Remove(cfgFile); err != nil && !errors.Is(err, os.ErrNotExist) {
		errs = append(errs, fmt.Sprintf("remove config: %v", err))
	}
	if err := os.Remove(filepath.Join(cfgDir, "user_id")); err != nil && !errors.Is(err, os.ErrNotExist) {
		errs = append(errs, fmt.Sprintf("remove user_id: %v", err))
	}
	if err := identity.Delete(effectiveMode); err != nil {
		errs = append(errs, fmt.Sprintf("remove identity: %v", err))
	}
	if err := config.DeleteDeviceAPIKeysForMode(effectiveMode); err != nil {
		errs = append(errs, fmt.Sprintf("remove device api key: %v", err))
	}
	if len(errs) > 0 {
		return errors.New(strings.Join(errs, "; "))
	}
	return nil
}

func WritePasswordFile(path, password string) (string, error) {
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
