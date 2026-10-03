package cli

import (
	"errors"
	"fmt"
	"net/http"
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

// severity classifies a diagnostic finding.
type severity int

const (
	severityOK severity = iota
	severityWarning
	severityProblem
)

// findings accumulates diagnostics so doctor can both print a readable report
// and report health through its exit code.
type findings struct {
	problems int
	warnings int
	seen     map[string]bool
}

func newFindings() *findings {
	return &findings{seen: map[string]bool{}}
}

// record notes a finding, skipping exact duplicates so repeated checks do not
// inflate the counts.
func (f *findings) record(level severity, format string, args ...interface{}) {
	msg := fmt.Sprintf(format, args...)
	key := fmt.Sprintf("%d|%s", level, msg)
	if f.seen[key] {
		return
	}
	f.seen[key] = true

	switch level {
	case severityProblem:
		f.problems++
		fmt.Printf("✗  %s\n", msg)
	case severityWarning:
		f.warnings++
		fmt.Printf("⚠  %s\n", msg)
	default:
		fmt.Printf("✓  %s\n", msg)
	}
}

func (f *findings) healthy() bool { return f.problems == 0 }

func RunDoctor(args []string) error {
	fs := newFlagSet("doctor")
	mode := fs.String("mode", "user", "Inspect mode: user or system")
	configPath := fs.String("config", "", "Config path override")
	checkServer := fs.Bool("check-server", false, "Attempt GET /control/v1/config and print HTTP status")
	strict := fs.Bool("strict", false, "Exit non-zero when a problem is found (for scripts and onboarding checks)")
	if help, err := parseFlags(fs, args); err != nil {
		return err
	} else if help {
		return nil
	}
	cfgFile, err := resolveConfigPathWithMode(*configPath, *mode)
	if err != nil {
		return fmt.Errorf("resolve config path: %w", err)
	}

	found := newFindings()
	if err := doctorCommand(*mode, cfgFile, *checkServer, found); err != nil {
		return fmt.Errorf("doctor failed: %w", err)
	}

	fmt.Println()
	switch {
	case found.healthy() && found.warnings == 0:
		fmt.Println("Everything checks out.")
	case found.healthy():
		fmt.Printf("No blocking problems (%d warning(s)).\n", found.warnings)
	default:
		fmt.Printf("Found %d problem(s)", found.problems)
		if found.warnings > 0 {
			fmt.Printf(" and %d warning(s)", found.warnings)
		}
		fmt.Println(". Backups may not run until these are fixed.")
	}
	if !found.healthy() {
		fmt.Println("Run `xentz-agent status` for a quick summary, or `xentz-agent diagnostics --out <file>` to collect a support bundle.")
	}

	// Only --strict changes the exit code, so existing scripts that treat any
	// doctor run as informational keep working.
	if *strict && !found.healthy() {
		return failWithCode(fmt.Errorf("doctor found %d problem(s)", found.problems), 1)
	}
	return nil
}

func doctorCommand(mode, cfgFile string, checkServer bool, found *findings) error {
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
	enrolled := false
	if err != nil {
		found.record(severityProblem, "Configuration is missing or unreadable (%v)", err)
		found.record(severityWarning, "Not installed yet? Run `xentz-agent install --token <token> --server <url>`")
	} else {
		enrolled = enroll.IsEnrolled(cfg.TenantID, cfg.DeviceID)
		fmt.Printf("Enrolled in config: %v\n", enrolled)
		fmt.Printf("Tenant ID:   %s\n", strings.TrimSpace(cfg.TenantID))
		fmt.Printf("Device ID:   %s\n", strings.TrimSpace(cfg.DeviceID))
		fmt.Printf("Server URL:  %s\n", strings.TrimSpace(cfg.ServerURL))
		if strings.TrimSpace(cfg.DeviceAPIKey) != "" {
			fmt.Printf("Config has device_api_key field: yes (legacy/fallback)\n")
		} else {
			fmt.Printf("Config has device_api_key field: no\n")
		}

		if !enrolled && strings.TrimSpace(cfg.Restic.Repository) == "" {
			found.record(severityProblem, "Device is neither enrolled nor configured with a repository, so no backups can run")
		}
		if len(cfg.Include) == 0 {
			found.record(severityWarning, "No include paths configured, so a backup would copy nothing. Add one with `xentz-agent config --add-include <path>`")
		}
		if cfg.Enabled != nil && !*cfg.Enabled {
			found.record(severityProblem, "Server has disabled this device (kill switch). Backups and restores are blocked until it is re-enabled")
		}
	}

	keyName := "device_api_key (" + strings.ToLower(strings.TrimSpace(effectiveMode)) + ")"
	apiKey, err := config.GetDeviceAPIKeyForModeReadOnly(config.Config{Mode: effectiveMode}, effectiveMode)
	switch {
	case err == nil:
		fmt.Printf("Secret store %s: present (len=%d)\n", keyName, len(strings.TrimSpace(string(apiKey))))
	case errors.Is(err, secretstore.ErrNotFound):
		fmt.Printf("Secret store %s: missing\n", keyName)
		// Only fatal when a server identity exists, since that is what needs the key.
		if enrolled {
			found.record(severityProblem, "No device API key in the secret store, so config cannot be fetched from the server. Re-enroll with `xentz-agent recover`")
		}
	default:
		fmt.Printf("Secret store %s: error (%v)\n", keyName, err)
		found.record(severityProblem, "Secret store could not be read: %v", err)
	}

	if _, err := exec.LookPath("restic"); err != nil {
		found.record(severityProblem, "restic is not in PATH, so backups and restores cannot run")
		if hint := resticInstallHint(); hint != "" {
			found.record(severityWarning, "Install restic with: %s", hint)
		}
	} else {
		found.record(severityOK, "restic is installed and in PATH")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		found.record(severityWarning, "Cannot resolve the home directory, so restore destinations cannot be checked (%v)", err)
	} else {
		restoreTestDir := filepath.Join(home, "Desktop", "xentz-restore-check")
		if err := os.MkdirAll(restoreTestDir, 0o700); err != nil {
			found.record(severityWarning, "Restore destination %s is not writable (%v)", restoreTestDir, err)
		} else {
			f, err := os.CreateTemp(restoreTestDir, ".write-test-*")
			if err != nil {
				found.record(severityWarning, "Restore destination %s is not writable (%v)", restoreTestDir, err)
			} else {
				_ = f.Close()
				_ = os.Remove(f.Name())
				found.record(severityOK, "Restore destination is writable (%s)", restoreTestDir)
			}
		}
	}
	if runtime.GOOS == "darwin" {
		for _, inc := range cfg.Include {
			if strings.Contains(inc, "/Documents") || strings.Contains(inc, "/Desktop") {
				found.record(severityWarning, "Include paths contain Documents/Desktop. If backup or restore reports 'operation not permitted', see docs/MACOS_FULL_DISK_ACCESS_CHECKLIST.md")
				break
			}
		}
	}
	if !checkServer {
		return nil
	}
	fmt.Println()
	serverURL := strings.TrimSpace(cfg.ServerURL)
	if serverURL == "" {
		if id, idErr := identity.Load(effectiveMode); idErr == nil {
			serverURL = strings.TrimSpace(id.ServerURL)
		}
	}
	if serverURL == "" {
		found.record(severityWarning, "Server check skipped: no server URL in config or identity")
		return nil
	}
	key := strings.TrimSpace(apiKey)
	if key == "" {
		key = strings.TrimSpace(cfg.DeviceAPIKey)
	}
	if key == "" {
		found.record(severityWarning, "Server check skipped: no device API key in the secret store or config")
		return nil
	}
	client, err := controlapi.New(serverURL, key, 10*time.Second)
	if err != nil {
		found.record(severityWarning, "Server check skipped: %v", err)
		return nil
	}
	status, err := client.GetStatus("/control/v1/config")
	if err != nil {
		var statusErr *controlapi.StatusError
		if errors.As(err, &statusErr) {
			fmt.Printf("Server check status: %d\n", statusErr.StatusCode)
			switch {
			case statusErr.AuthFailure():
				found.record(severityProblem, "Server rejected this device's credentials (HTTP %d). The API key is invalid or revoked - re-enroll with `xentz-agent recover`", statusErr.StatusCode)
			case statusErr.StatusCode == http.StatusTooManyRequests:
				found.record(severityWarning, "Server is rate limiting this device (HTTP 429)")
			case statusErr.StatusCode >= 500:
				found.record(severityWarning, "Server returned an error (HTTP %d); this is a control-plane problem, not this device", statusErr.StatusCode)
			default:
				found.record(severityProblem, "Server returned HTTP %d for the config request", statusErr.StatusCode)
			}
			return nil
		}
		// A network failure is not a device fault; the agent can still run on
		// cached config, so warn rather than fail.
		found.record(severityWarning, "Could not reach the control plane (%v). Backups will use the cached configuration until it is reachable again", err)
		return nil
	}
	if status >= 200 && status < 300 {
		found.record(severityOK, "Control plane accepted this device (HTTP %d)", status)
	}
	fmt.Printf("Server check status: %d\n", status)
	return nil
}
