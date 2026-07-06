package cli

import (
	"flag"
	"fmt"
	"log"
	"strings"

	"xentz-agent/internal/config"
	"xentz-agent/internal/enroll"
	"xentz-agent/internal/identity"
)

func RunRecover(args []string) error {
	fs := flag.NewFlagSet("recover", flag.ExitOnError)
	server := fs.String("server", "", "Control plane base URL (required)")
	mode := fs.String("mode", "user", "Mode: user or system (controls where identity is stored)")
	configPath := fs.String("config", "", "Config path override")
	recoveryToken := fs.String("recovery-token", "", "Portal-minted recovery token (one-time, short-lived)")
	principalIDFlag := fs.String("principal-id", "", "Stable principal ID (optional if identity.json exists)")
	displayName := fs.String("display-name", "", "Human-friendly name (optional, defaults to current username)")

	if err := fs.Parse(args); err != nil {
		return fmt.Errorf("parse flags: %w", err)
	}
	if *server == "" {
		return fmt.Errorf("--server is required")
	}

	cfgFile, err := resolveConfigPathWithMode(*configPath, *mode)
	if err != nil {
		return fmt.Errorf("resolve config path: %w", err)
	}

	principalID := strings.TrimSpace(*principalIDFlag)
	if principalID == "" {
		if id, err := identity.Load(*mode); err == nil {
			principalID = strings.TrimSpace(id.PrincipalID)
		}
	}
	if principalID == "" {
		return fmt.Errorf("principal id not found; provide --principal-id or restore identity.json")
	}

	if strings.TrimSpace(*displayName) == "" {
		if u, err := enroll.GetUserID(); err == nil {
			*displayName = u
		}
	}

	log.Println("Recovering enrollment with control plane...")
	res, err := enroll.Recover(*server, *recoveryToken, principalID, *displayName)
	if err != nil {
		return fmt.Errorf("recover failed: %w", err)
	}

	if err := config.StoreDeviceAPIKeyForMode(res.DeviceAPIKey, *mode); err != nil {
		return fmt.Errorf("store device api key: %w", err)
	}
	if err := config.StoreResticPassword(res.Password); err != nil {
		return fmt.Errorf("store restic password: %w", err)
	}

	fetchedCfg, err := config.FetchAndCache(*server, res.DeviceAPIKey)
	if err != nil {
		log.Printf("warning: failed to fetch config after recovery: %v", err)
	}

	cfg := fetchedCfg
	cfg.ServerURL = *server
	cfg.TenantID = res.TenantID
	cfg.DeviceID = res.DeviceID
	cfg.DeviceAPIKey = ""
	cfg.Mode = *mode
	if cfg.UserID == "" {
		if u, err := enroll.GetUserID(); err == nil {
			cfg.UserID = u
		}
	}
	_ = config.EnsurePasswordFile(cfg, *server, res.DeviceAPIKey)

	if err := config.Write(cfgFile, cfg); err != nil {
		return fmt.Errorf("write config: %w", err)
	}

	_ = identity.Save(*mode, identity.Identity{
		ServerURL:   *server,
		TenantID:    res.TenantID,
		DeviceID:    res.DeviceID,
		PrincipalID: principalID,
		Mode:        *mode,
	})

	log.Println("Recovery successful:")
	log.Printf("  Tenant ID: %s", res.TenantID)
	log.Printf("  Device ID: %s", res.DeviceID)
	log.Printf("  Repo:      %s", res.RepoPath)
	return nil
}
