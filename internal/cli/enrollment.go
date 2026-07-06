package cli

import (
	"errors"
	"fmt"
	"strings"

	"xentz-agent/internal/config"
	"xentz-agent/internal/identity"
	"xentz-agent/internal/paths"
	"xentz-agent/internal/secretstore"
)

func LoadEnrollment(cfgFile, mode string) (config.Config, string, error) {
	var cfg config.Config
	if c, err := config.Read(cfgFile); err == nil {
		cfg = c
	}

	if strings.TrimSpace(cfg.DeviceID) != "" && strings.TrimSpace(cfg.ServerURL) != "" {
		effectiveMode := mode
		if strings.TrimSpace(effectiveMode) == "" {
			effectiveMode = cfg.Mode
		}
		if _, err := identity.Load(effectiveMode); err != nil {
			_ = identity.Save(effectiveMode, identity.Identity{
				ServerURL:   strings.TrimSpace(cfg.ServerURL),
				TenantID:    strings.TrimSpace(cfg.TenantID),
				DeviceID:    strings.TrimSpace(cfg.DeviceID),
				PrincipalID: strings.TrimSpace(cfg.DeviceID),
				Mode:        strings.TrimSpace(cfg.Mode),
			})
		}
	}

	if strings.TrimSpace(cfg.ServerURL) == "" || strings.TrimSpace(cfg.DeviceID) == "" || strings.TrimSpace(cfg.TenantID) == "" {
		if id, err := identity.Load(mode); err == nil {
			if strings.TrimSpace(cfg.ServerURL) == "" {
				cfg.ServerURL = strings.TrimSpace(id.ServerURL)
			}
			if strings.TrimSpace(cfg.TenantID) == "" {
				cfg.TenantID = strings.TrimSpace(id.TenantID)
			}
			if strings.TrimSpace(cfg.DeviceID) == "" {
				cfg.DeviceID = strings.TrimSpace(id.DeviceID)
			}
			if strings.TrimSpace(cfg.Mode) == "" && strings.TrimSpace(id.Mode) != "" {
				cfg.Mode = strings.TrimSpace(id.Mode)
			}
		}
	}

	effectiveMode := strings.TrimSpace(mode)
	if effectiveMode == "" {
		effectiveMode = strings.TrimSpace(cfg.Mode)
	}
	apiKey, err := config.GetDeviceAPIKeyForMode(cfg, effectiveMode)
	if err != nil {
		if errors.Is(err, secretstore.ErrNotFound) {
			return cfg, "", nil
		}
		return cfg, "", err
	}
	return cfg, strings.TrimSpace(apiKey), nil
}

func ModeMismatchError(command string, cfg config.Config) error {
	enrollmentMode := strings.ToLower(strings.TrimSpace(cfg.Mode))
	if enrollmentMode == "" {
		return nil
	}
	runtimeMode := strings.ToLower(string(paths.ResolveMode("")))
	if enrollmentMode == runtimeMode {
		return nil
	}
	switch {
	case enrollmentMode == "system" && runtimeMode == "user":
		return fmt.Errorf("%s: enrollment is system mode, but command is running in user mode. Re-run with sudo (or re-enroll with --mode user)", command)
	case enrollmentMode == "user" && runtimeMode == "system":
		return fmt.Errorf("%s: enrollment is user mode, but command is running in system mode (likely sudo). Re-run without sudo (or re-enroll with --mode system)", command)
	default:
		return fmt.Errorf("%s: enrollment mode is %q but runtime mode is %q", command, enrollmentMode, runtimeMode)
	}
}
