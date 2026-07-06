package cli

import (
	"errors"
	"flag"
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
	fs := flag.NewFlagSet("install", flag.ExitOnError)
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

	if err := fs.Parse(args); err != nil {
		return fmt.Errorf("parse flags: %w", err)
	}

	cfgFile, err := resolveConfigPathWithMode(*configPath, *mode)
	if err != nil {
		return fmt.Errorf("resolve config path: %w", err)
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
			} else if *password != "" {
				if err := storeInstallPassword(*password, *passwordFile, &cfg); err != nil {
					return err
				}
			} else {
				return fmt.Errorf("password required: either server must provide it or use --password flag")
			}
		}
	} else if *repo != "" {
		log.Println("Using legacy mode with direct repository URL")
		if *password == "" {
			return fmt.Errorf("--password is required when using --repo (legacy mode)")
		}
		if err := storeInstallPassword(*password, *passwordFile, &cfg); err != nil {
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
	return nil
}

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
