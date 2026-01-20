package diagnostics

import (
	"archive/zip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"xentz-agent/internal/config"
	"xentz-agent/internal/paths"
	"xentz-agent/internal/report"
	"xentz-agent/internal/state"
)

type Summary struct {
	Timestamp      string `json:"timestamp"`
	Version        string `json:"version"`
	OS             string `json:"os"`
	Arch           string `json:"arch"`
	ConfigRevision int    `json:"config_revision,omitempty"`
	ConfigChecksum string `json:"config_checksum,omitempty"`
	SpoolCount     int    `json:"spool_count"`
	SpoolBytes     int64  `json:"spool_bytes"`
	LastRunStatus  string `json:"last_run_status,omitempty"`
	LastRunError   string `json:"last_run_error,omitempty"`
	Revoked        bool   `json:"revoked,omitempty"`
}

func CreateBundle(outPath string) error {
	if outPath == "" {
		return fmt.Errorf("output path required")
	}
	if err := os.MkdirAll(filepath.Dir(outPath), 0o700); err != nil {
		return fmt.Errorf("create output dir: %w", err)
	}
	outFile, err := os.Create(outPath)
	if err != nil {
		return fmt.Errorf("create bundle: %w", err)
	}
	defer outFile.Close()

	zipWriter := zip.NewWriter(outFile)
	defer zipWriter.Close()

	summary, logExcerpt := buildSummary()
	if err := writeJSON(zipWriter, "summary.json", summary); err != nil {
		return err
	}
	if logExcerpt != "" {
		if err := writeText(zipWriter, "logs.txt", logExcerpt); err != nil {
			return err
		}
	}

	return nil
}

func buildSummary() (Summary, string) {
	var summary Summary
	summary.Timestamp = time.Now().UTC().Format(time.RFC3339)
	summary.Version = runtime.Version()
	summary.OS = runtime.GOOS
	summary.Arch = runtime.GOARCH

	cfgPath, err := config.ResolvePath("")
	if err == nil {
		if checksum, err := checksumFile(cfgPath); err == nil {
			summary.ConfigChecksum = checksum
		}
		if cfg, err := config.Read(cfgPath); err == nil {
			summary.ConfigRevision = cfg.ConfigRevision
		}
	}

	if cachedCfg, err := config.ReadCached(); err == nil && summary.ConfigRevision == 0 {
		summary.ConfigRevision = cachedCfg.ConfigRevision
	}

	spoolCount, spoolBytes, err := report.SpoolStats()
	if err == nil {
		summary.SpoolCount = spoolCount
		summary.SpoolBytes = spoolBytes
	}

	if st, err := state.New(); err == nil {
		if last, ok, _ := st.LoadLastRun(); ok {
			summary.LastRunStatus = last.Status
			summary.LastRunError = last.Error
		}
		if agentState, ok, _ := st.LoadAgentState(); ok {
			summary.Revoked = agentState.Revoked
		}
	}

	logs := tailLogs()
	return summary, logs
}

func checksumFile(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func tailLogs() string {
	logDir, err := paths.LogDir("")
	if err != nil {
		return ""
	}
	logPath := filepath.Join(logDir, "agent.log")
	data, err := os.ReadFile(logPath)
	if err != nil {
		return ""
	}
	const maxBytes = 2000 * 200 // rough limit for last 2000 lines
	if len(data) > maxBytes {
		data = data[len(data)-maxBytes:]
	}
	return string(data)
}

func writeJSON(zipWriter *zip.Writer, name string, v interface{}) error {
	w, err := zipWriter.Create(name)
	if err != nil {
		return err
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

func writeText(zipWriter *zip.Writer, name, content string) error {
	w, err := zipWriter.Create(name)
	if err != nil {
		return err
	}
	_, err = w.Write([]byte(content))
	return err
}
