package cli

import (
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"

	"xentz-agent/internal/config"
	"xentz-agent/internal/install"
	"xentz-agent/internal/paths"
)

func RunUpgrade(args []string) error {
	fs := flag.NewFlagSet("upgrade", flag.ExitOnError)
	mode := fs.String("mode", "user", "Upgrade mode: user or system")
	newBinary := fs.String("binary", "", "Path to new xentz-agent binary")
	configPath := fs.String("config", "", "Config path override")
	if err := fs.Parse(args); err != nil {
		return fmt.Errorf("parse flags: %w", err)
	}
	if *newBinary == "" {
		return fmt.Errorf("--binary is required for upgrade")
	}
	cfgFile, err := resolveConfigPathWithMode(*configPath, *mode)
	if err != nil {
		return fmt.Errorf("resolve config path: %w", err)
	}
	if err := replaceBinary(*newBinary); err != nil {
		return fmt.Errorf("upgrade failed: %w", err)
	}
	if err := install.InstallWithMode(cfgFile, *mode); err != nil {
		return fmt.Errorf("restart failed: %w", err)
	}
	log.Println("upgrade complete ✅")
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
