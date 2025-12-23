package report

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"xentz-agent/internal/validation"
)

const (
	maxErrorLength    = 4096 // Maximum error message length in bytes
	maxPendingReports = 20
)

// Report represents a backup or retention run report
type Report struct {
	DeviceID       string `json:"device_id"`
	Job            string `json:"job"`         // "backup" or "retention"
	StartedAt      string `json:"started_at"`  // RFC3339 UTC
	FinishedAt     string `json:"finished_at"` // RFC3339 UTC
	Status         string `json:"status"`      // "success" or "failure"
	DurationMS     int64  `json:"duration_ms"`
	FilesTotal     int64  `json:"files_total,omitempty"`
	BytesTotal     int64  `json:"bytes_total,omitempty"`
	DataAddedBytes int64  `json:"data_added_bytes,omitempty"`
	SnapshotID     string `json:"snapshot_id,omitempty"`
	Error          string `json:"error,omitempty"` // Truncated to 4096 bytes
}

// getSpoolDir returns the spool directory path
func getSpoolDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".xentz-agent", "spool"), nil
}

// truncateError truncates error message to maxErrorLength bytes
func truncateError(errMsg string) string {
	if len(errMsg) <= maxErrorLength {
		return errMsg
	}
	// Truncate to maxErrorLength, but try to avoid cutting in the middle of a UTF-8 character
	truncated := errMsg[:maxErrorLength]
	// Find last valid UTF-8 character boundary
	for i := len(truncated) - 1; i >= 0; i-- {
		if (truncated[i] & 0xC0) != 0x80 {
			return truncated[:i+1]
		}
	}
	return truncated
}

// SendReport sends a report to the server
func SendReport(serverURL, deviceAPIKey string, report Report) error {
	if serverURL == "" {
		return fmt.Errorf("server URL is required")
	}
	if deviceAPIKey == "" {
		return fmt.Errorf("device API key is required")
	}

	// Validate server URL to prevent SSRF
	if err := validation.ValidateServerURL(serverURL); err != nil {
		return fmt.Errorf("invalid server URL: %w", err)
	}

	// Truncate error message if present
	if report.Error != "" {
		report.Error = truncateError(report.Error)
	}

	jsonData, err := json.Marshal(report)
	if err != nil {
		return fmt.Errorf("marshal report: %w", err)
	}

	// Make POST request to /v1/report
	url := fmt.Sprintf("%s/v1/report", serverURL)
	req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", deviceAPIKey))

	// Set timeout
	client := &http.Client{
		Timeout: 30 * time.Second,
	}

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("report request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		var errMsg bytes.Buffer
		// Limit error message to prevent information leakage
		io.CopyN(&errMsg, resp.Body, 512) // Limit to 512 bytes
		errStr := strings.TrimSpace(errMsg.String())
		// Remove newlines and limit length
		errStr = strings.ReplaceAll(errStr, "\n", " ")
		errStr = strings.ReplaceAll(errStr, "\r", " ")
		if len(errStr) > 256 {
			errStr = errStr[:256] + "..."
		}
		return fmt.Errorf("report failed (status %d): %s", resp.StatusCode, errStr)
	}

	return nil
}

// checkSpoolSize checks if spool directory is within size limits
func checkSpoolSize() error {
	spoolDir, err := getSpoolDir()
	if err != nil {
		return err
	}
	var totalSize int64
	entries, err := os.ReadDir(spoolDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // Directory doesn't exist yet, that's fine
		}
		return err
	}
	for _, entry := range entries {
		if info, err := entry.Info(); err == nil {
			totalSize += info.Size()
		}
	}
	const maxSpoolSize = 100 * 1024 * 1024 // 100MB
	if totalSize > maxSpoolSize {
		return fmt.Errorf("spool directory too large: %d bytes (max %d bytes)", totalSize, maxSpoolSize)
	}
	return nil
}

// SpoolReport writes a report to the spool directory
func SpoolReport(report Report) error {
	spoolDir, err := getSpoolDir()
	if err != nil {
		return fmt.Errorf("get spool dir: %w", err)
	}

	if err := os.MkdirAll(spoolDir, 0o700); err != nil {
		return fmt.Errorf("create spool dir: %w", err)
	}

	// Check spool size before writing
	if err := checkSpoolSize(); err != nil {
		return fmt.Errorf("spool size check failed: %w", err)
	}

	// Truncate error message if present
	if report.Error != "" {
		report.Error = truncateError(report.Error)
	}

	// Sanitize job and status to prevent path injection
	sanitize := func(s string) string {
		// Remove any path separators and dangerous characters
		s = strings.ReplaceAll(s, "/", "_")
		s = strings.ReplaceAll(s, "\\", "_")
		s = strings.ReplaceAll(s, "..", "_")
		s = strings.ReplaceAll(s, string(filepath.Separator), "_")
		// Remove any other potentially dangerous characters
		s = strings.ReplaceAll(s, "\x00", "_")
		// Limit length
		if len(s) > 50 {
			s = s[:50]
		}
		return s
	}

	// Generate filename: {unix_timestamp}-{job}-{status}.json
	timestamp := time.Now().Unix()
	filename := fmt.Sprintf("%d-%s-%s.json", timestamp, sanitize(report.Job), sanitize(report.Status))
	targetPath := filepath.Join(spoolDir, filename)

	jsonData, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal report: %w", err)
	}

	if err := os.WriteFile(targetPath, jsonData, 0o600); err != nil {
		return fmt.Errorf("write spool file: %w", err)
	}

	return nil
}

// SendReportWithSpool attempts to send report immediately, spools if it fails
func SendReportWithSpool(serverURL, deviceAPIKey string, report Report) error {
	// Try to send immediately
	err := SendReport(serverURL, deviceAPIKey, report)
	if err == nil {
		return nil
	}

	// Send failed, spool it
	log.Printf("warning: failed to send report to server: %v", err)
	if spoolErr := SpoolReport(report); spoolErr != nil {
		log.Printf("error: failed to spool report: %v", spoolErr)
		return fmt.Errorf("send failed and spool failed: send=%v, spool=%v", err, spoolErr)
	}

	log.Printf("Report spooled for retry: %s/%s", report.Job, report.Status)
	return err // Return original send error (non-blocking)
}

// LoadPendingReports loads pending reports from spool directory
func LoadPendingReports(maxCount int) ([]Report, []string, error) {
	spoolDir, err := getSpoolDir()
	if err != nil {
		return nil, nil, fmt.Errorf("get spool dir: %w", err)
	}

	entries, err := os.ReadDir(spoolDir)
	if err != nil {
		if os.IsNotExist(err) {
			return []Report{}, []string{}, nil
		}
		return nil, nil, fmt.Errorf("read spool dir: %w", err)
	}

	// Filter JSON files and sort by filename (oldest first)
	var files []string
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".json") {
			files = append(files, entry.Name())
		}
	}

	// Sort by filename (which includes timestamp, so oldest first)
	sort.Strings(files)

	// Limit to maxCount
	if len(files) > maxCount {
		files = files[:maxCount]
	}

	// Load reports
	var reports []Report
	var filenames []string
	for _, filename := range files {
		targetPath := filepath.Join(spoolDir, filename)
		data, err := os.ReadFile(targetPath)
		if err != nil {
			log.Printf("warning: failed to read spooled report %s: %v", filename, err)
			continue
		}

		var report Report
		if err := json.Unmarshal(data, &report); err != nil {
			log.Printf("warning: failed to parse spooled report %s: %v", filename, err)
			continue
		}

		reports = append(reports, report)
		filenames = append(filenames, filename)
	}

	return reports, filenames, nil
}

// DeleteSpooledReport deletes a spooled report file
func DeleteSpooledReport(filename string) error {
	// Validate filename - must be simple filename, no path separators
	if strings.Contains(filename, "/") || strings.Contains(filename, "\\") ||
		strings.Contains(filename, "..") || filepath.IsAbs(filename) {
		return fmt.Errorf("invalid filename: %s", filename)
	}
	// Ensure it's a JSON file
	if !strings.HasSuffix(filename, ".json") {
		return fmt.Errorf("invalid filename: must be .json file")
	}

	spoolDir, err := getSpoolDir()
	if err != nil {
		return fmt.Errorf("get spool dir: %w", err)
	}

	targetPath := filepath.Join(spoolDir, filename)
	// Double-check the resolved path is within spoolDir
	resolved, err := filepath.EvalSymlinks(targetPath)
	if err == nil {
		spoolResolved, err2 := filepath.EvalSymlinks(spoolDir)
		if err2 == nil {
			spoolResolvedWithSep := spoolResolved + string(filepath.Separator)
			if !strings.HasPrefix(resolved, spoolResolvedWithSep) && resolved != spoolResolved {
				return fmt.Errorf("path traversal detected")
			}
		}
	}

	if err := os.Remove(targetPath); err != nil {
		return fmt.Errorf("delete spool file: %w", err)
	}

	return nil
}

// CleanupOldReports removes reports older than maxAge
func CleanupOldReports(maxAge time.Duration) error {
	spoolDir, err := getSpoolDir()
	if err != nil {
		return fmt.Errorf("get spool dir: %w", err)
	}

	entries, err := os.ReadDir(spoolDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("read spool dir: %w", err)
	}

	cutoff := time.Now().Add(-maxAge)
	deleted := 0

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}

		// Extract timestamp from filename: {timestamp}-{job}-{status}.json
		parts := strings.Split(entry.Name(), "-")
		if len(parts) < 2 {
			continue
		}

		timestamp, err := strconv.ParseInt(parts[0], 10, 64)
		if err != nil {
			log.Printf("warning: invalid timestamp in spool file %s: %v", entry.Name(), err)
			continue
		}

		fileTime := time.Unix(timestamp, 0)
		if fileTime.Before(cutoff) {
			targetPath := filepath.Join(spoolDir, entry.Name())
			if err := os.Remove(targetPath); err != nil {
				log.Printf("warning: failed to delete old report %s: %v", entry.Name(), err)
			} else {
				deleted++
			}
		}
	}

	if deleted > 0 {
		log.Printf("Cleaned up %d old reports (older than %v)", deleted, maxAge)
	}

	return nil
}

// SendPendingReports sends pending reports from spool directory
func SendPendingReports(serverURL, deviceAPIKey string, maxCount int) error {
	if serverURL == "" || deviceAPIKey == "" {
		// Can't send reports without server URL or API key
		return nil
	}

	reports, filenames, err := LoadPendingReports(maxCount)
	if err != nil {
		return fmt.Errorf("load pending reports: %w", err)
	}

	if len(reports) == 0 {
		return nil
	}

	log.Printf("Sending %d pending report(s)...", len(reports))

	successCount := 0
	for i, report := range reports {
		// Rate limit: wait 100ms between reports to avoid flooding server
		if i > 0 {
			time.Sleep(100 * time.Millisecond)
		}

		err := SendReport(serverURL, deviceAPIKey, report)
		if err != nil {
			log.Printf("warning: failed to send pending report %s/%s: %v", report.Job, report.Status, err)
			// Continue with next report
			continue
		}

		// Successfully sent, delete from spool
		if err := DeleteSpooledReport(filenames[i]); err != nil {
			log.Printf("warning: failed to delete spooled report %s: %v", filenames[i], err)
		} else {
			successCount++
		}
	}

	if successCount > 0 {
		log.Printf("Successfully sent %d pending report(s)", successCount)
	}

	return nil
}
