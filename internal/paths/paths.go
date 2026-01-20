package paths

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

type Mode string

const (
	ModeUser   Mode = "user"
	ModeSystem Mode = "system"
)

func ResolveMode(mode string) Mode {
	if mode == "" {
		mode = strings.TrimSpace(os.Getenv("XENTZ_MODE"))
		if mode == "" {
			mode = strings.TrimSpace(os.Getenv("XENTZ_AGENT_MODE"))
		}
	}
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case "system":
		return ModeSystem
	case "user":
		return ModeUser
	default:
		if isElevated() {
			return ModeSystem
		}
		return ModeUser
	}
}

func ConfigDir(mode string) (string, error) {
	return resolveDir(ResolveMode(mode), dirConfig)
}

func StateDir(mode string) (string, error) {
	return resolveDir(ResolveMode(mode), dirState)
}

func LogDir(mode string) (string, error) {
	return resolveDir(ResolveMode(mode), dirLogs)
}

func EnsureDirs(mode string) error {
	cfgDir, err := ConfigDir(mode)
	if err != nil {
		return err
	}
	stateDir, err := StateDir(mode)
	if err != nil {
		return err
	}
	logDir, err := LogDir(mode)
	if err != nil {
		return err
	}
	for _, dir := range []string{cfgDir, stateDir, logDir} {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return err
		}
	}
	return nil
}

type dirKind int

const (
	dirConfig dirKind = iota
	dirState
	dirLogs
)

func resolveDir(mode Mode, kind dirKind) (string, error) {
	switch runtime.GOOS {
	case "darwin":
		return resolveDarwin(mode, kind)
	case "windows":
		return resolveWindows(mode, kind)
	default:
		return resolveLinux(mode, kind)
	}
}

func resolveLinux(mode Mode, kind dirKind) (string, error) {
	if mode == ModeSystem {
		switch kind {
		case dirConfig:
			return "/etc/xentz-agent", nil
		case dirState:
			return "/var/lib/xentz-agent", nil
		case dirLogs:
			return "/var/log/xentz-agent", nil
		}
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	switch kind {
	case dirConfig:
		return filepath.Join(home, ".config", "xentz-agent"), nil
	case dirState:
		return filepath.Join(home, ".local", "share", "xentz-agent"), nil
	case dirLogs:
		return filepath.Join(home, ".local", "state", "xentz-agent", "logs"), nil
	}
	return "", errors.New("unknown dir kind")
}

func resolveDarwin(mode Mode, kind dirKind) (string, error) {
	if mode == ModeSystem {
		switch kind {
		case dirConfig:
			return "/Library/Application Support/XentzAgent/config", nil
		case dirState:
			return "/Library/Application Support/XentzAgent/state", nil
		case dirLogs:
			return "/Library/Logs/XentzAgent", nil
		}
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	base := filepath.Join(home, "Library", "Application Support", "XentzAgent")
	switch kind {
	case dirConfig:
		return filepath.Join(base, "config"), nil
	case dirState:
		return filepath.Join(base, "state"), nil
	case dirLogs:
		return filepath.Join(home, "Library", "Logs", "XentzAgent"), nil
	}
	return "", errors.New("unknown dir kind")
}

func resolveWindows(mode Mode, kind dirKind) (string, error) {
	if mode == ModeSystem {
		programData := os.Getenv("ProgramData")
		if programData == "" {
			return "", errors.New("ProgramData env var not set")
		}
		base := filepath.Join(programData, "XentzAgent")
		switch kind {
		case dirConfig:
			return filepath.Join(base, "config"), nil
		case dirState:
			return filepath.Join(base, "state"), nil
		case dirLogs:
			return filepath.Join(base, "logs"), nil
		}
	}

	appData := os.Getenv("APPDATA")
	localAppData := os.Getenv("LOCALAPPDATA")
	if appData == "" || localAppData == "" {
		return "", errors.New("APPDATA/LOCALAPPDATA env var not set")
	}
	switch kind {
	case dirConfig:
		return filepath.Join(appData, "XentzAgent", "config"), nil
	case dirState:
		return filepath.Join(localAppData, "XentzAgent", "state"), nil
	case dirLogs:
		return filepath.Join(localAppData, "XentzAgent", "logs"), nil
	}
	return "", errors.New("unknown dir kind")
}
