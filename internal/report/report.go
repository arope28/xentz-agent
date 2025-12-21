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
)

const (
	maxErrorLength = 4096 // Maximum error message length in bytes
	maxPendingReports = 20
)

// Report represents a backup or retention run report
type Report struct {
	DeviceID      string `json:"device_id"`
	Job           string `json:"job"` // "backup" or "retention"
	StartedAt     string `json:"started_at"` // RFC3339 UTC
	FinishedAt    string `json:"finished_at"` // RFC3339 UTC
	Status        string `json:"status"` // "success" or "failure"
	DurationMS    int64  `json:"duration_ms"`
	FilesTotal    int64  `json:"files_total,omitempty"`
	BytesTotal    int64  `json:"bytes_total,omitempty"`
	DataAddedBytes int64 `json:"data_added_bytes,omitempty"`
	SnapshotID    string `json:"snapshot_id,omitempty"`
	Error         string `json:"error,omitempty"` // Truncated to 4096 bytes
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
		io.Copy(&errMsg, resp.Body)
		return fmt.Errorf("report failed (status %d): %s", resp.StatusCode, errMsg.String())
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

	// Truncate error message if present
	if report.Error != "" {
		report.Error = truncateError(report.Error)
	}

	// Generate filename: {unix_timestamp}-{job}-{status}.json
	timestamp := time.Now().Unix()
	filename := fmt.Sprintf("%d-%s-%s.json", timestamp, report.Job, report.Status)
	filepath := filepath.Join(spoolDir, filename)

	jsonData, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal report: %w", err)
	}

	if err := os.WriteFile(filepath, jsonData, 0o600); err != nil {
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
		filepath := filepath.Join(spoolDir, filename)
		data, err := os.ReadFile(filepath)
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
	spoolDir, err := getSpoolDir()
	if err != nil {
		return fmt.Errorf("get spool dir: %w", err)
	}

	filepath := filepath.Join(spoolDir, filename)
	if err := os.Remove(filepath); err != nil {
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
			filepath := filepath.Join(spoolDir, entry.Name())
			if err := os.Remove(filepath); err != nil {
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

