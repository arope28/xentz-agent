package logging

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const (
	maxEntriesPerBatch = 100
	minLogAgeForShipping = 1 * time.Hour
	maxRetries = 3
)

// ShipLogs ships log entries from rotated log files to the control plane
func (l *Logger) ShipLogs(serverURL, apiKey string) error {
	if serverURL == "" || apiKey == "" {
		return fmt.Errorf("server URL and API key required for log shipping")
	}

	logDir := filepath.Dir(l.writer.filePath)
	entries, err := os.ReadDir(logDir)
	if err != nil {
		return fmt.Errorf("read log directory: %w", err)
	}

	// Find rotated log files (not the active one)
	var rotatedFiles []os.DirEntry
	now := time.Now()
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		// Skip active log file
		if name == "agent.log" {
			continue
		}
		// Only process rotated JSON files (not compressed yet)
		if strings.HasPrefix(name, "agent.log.") && strings.HasSuffix(name, ".json") && !strings.HasSuffix(name, ".gz") {
			info, err := entry.Info()
			if err != nil {
				continue
			}
			// Only ship logs older than minLogAgeForShipping
			if now.Sub(info.ModTime()) >= minLogAgeForShipping {
				rotatedFiles = append(rotatedFiles, entry)
			}
		}
	}

	// Sort by modification time (oldest first)
	sort.Slice(rotatedFiles, func(i, j int) bool {
		infoI, _ := rotatedFiles[i].Info()
		infoJ, _ := rotatedFiles[j].Info()
		return infoI.ModTime().Before(infoJ.ModTime())
	})

	// Read and ship logs from each file
	for _, entry := range rotatedFiles {
		filePath := filepath.Join(logDir, entry.Name())
		if err := l.shipLogFile(filePath, serverURL, apiKey); err != nil {
			// Log error but continue with other files
			fmt.Fprintf(os.Stderr, "warning: failed to ship logs from %s: %v\n", entry.Name(), err)
		}
	}

	return nil
}

// shipLogFile ships logs from a specific rotated log file
func (l *Logger) shipLogFile(filePath, serverURL, apiKey string) error {
	// Read log entries from file
	logEntries, err := readLogEntries(filePath, maxEntriesPerBatch)
	if err != nil {
		return fmt.Errorf("read log entries: %w", err)
	}

	if len(logEntries) == 0 {
		return nil // No entries to ship
	}

	// Ship in batches
	for i := 0; i < len(logEntries); i += maxEntriesPerBatch {
		end := i + maxEntriesPerBatch
		if end > len(logEntries) {
			end = len(logEntries)
		}
		batch := logEntries[i:end]

		// Try to ship with retries
		if err := l.shipBatch(batch, serverURL, apiKey); err != nil {
			return fmt.Errorf("ship batch: %w", err)
		}

		// Mark as shipped (remove from file or mark in metadata)
		// For simplicity, we'll delete the file after successful shipping
		// In a more sophisticated implementation, we could track shipped entries
	}

	// After all batches are shipped, delete the file
	// This is a simple approach - in production, you might want to track shipped entries
	// and only delete after all entries are confirmed shipped
	return os.Remove(filePath)
}

// shipBatch ships a batch of log entries to the control plane
func (l *Logger) shipBatch(entries []LogEntry, serverURL, apiKey string) error {
	requestBody := map[string]interface{}{
		"logs": entries,
	}

	jsonData, err := json.Marshal(requestBody)
	if err != nil {
		return fmt.Errorf("marshal request: %w", err)
	}

	url := strings.TrimSuffix(serverURL, "/") + "/admin/v1/logs"
	req, err := http.NewRequest("POST", url, bytes.NewReader(jsonData))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)

	// Retry with exponential backoff
	var lastErr error
	for attempt := 0; attempt < maxRetries; attempt++ {
		if attempt > 0 {
			// Exponential backoff: 1s, 2s, 4s
			backoff := time.Duration(1<<uint(attempt)) * time.Second
			time.Sleep(backoff)
		}

		client := &http.Client{
			Timeout: 30 * time.Second,
		}

		resp, err := client.Do(req)
		if err != nil {
			lastErr = fmt.Errorf("http request: %w", err)
			continue
		}

		// Read response body
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()

		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			return nil // Success
		}

		lastErr = fmt.Errorf("http error %d: %s", resp.StatusCode, string(body))
	}

	return fmt.Errorf("failed after %d retries: %w", maxRetries, lastErr)
}

// readLogEntries reads log entries from a log file
func readLogEntries(filePath string, maxEntries int) ([]LogEntry, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, fmt.Errorf("open file: %w", err)
	}
	defer file.Close()

	var entries []LogEntry
	decoder := json.NewDecoder(file)

	for decoder.More() && len(entries) < maxEntries {
		var entry LogEntry
		if err := decoder.Decode(&entry); err != nil {
			if err == io.EOF {
				break
			}
			// Skip malformed entries
			continue
		}
		entries = append(entries, entry)
	}

	return entries, nil
}
