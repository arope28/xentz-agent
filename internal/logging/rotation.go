package logging

import (
	"compress/gzip"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

const (
	defaultMaxSizeBytes = 10 * 1024 * 1024 // 10MB
	defaultMaxAge       = 7 * 24 * time.Hour
	maxRotatedFiles     = 10
	compressAfterAge    = 24 * time.Hour
)

type rotatingWriter struct {
	filePath string
	file     *os.File
	size     int64
	lastRotateCheck time.Time
	mu       sync.Mutex
}

func newRotatingWriter(filePath string) (*rotatingWriter, error) {
	file, err := os.OpenFile(filePath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return nil, fmt.Errorf("open log file: %w", err)
	}

	info, err := file.Stat()
	if err != nil {
		file.Close()
		return nil, fmt.Errorf("stat log file: %w", err)
	}

	return &rotatingWriter{
		filePath:        filePath,
		file:            file,
		size:            info.Size(),
		lastRotateCheck: time.Now(),
	}, nil
}

func (w *rotatingWriter) Write(data []byte) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	// Check if rotation is needed
	if err := w.checkAndRotate(); err != nil {
		return fmt.Errorf("rotation check: %w", err)
	}

	// Write data with newline
	data = append(data, '\n')
	n, err := w.file.Write(data)
	if err != nil {
		return fmt.Errorf("write log: %w", err)
	}

	w.size += int64(n)
	return nil
}

func (w *rotatingWriter) checkAndRotate() error {
	now := time.Now()
	shouldCheckAge := now.Sub(w.lastRotateCheck) >= time.Hour

	// Check size-based rotation
	if w.size >= defaultMaxSizeBytes {
		return w.rotate("size")
	}

	// Check age-based rotation (once per hour)
	if shouldCheckAge {
		w.lastRotateCheck = now
		info, err := w.file.Stat()
		if err != nil {
			return err
		}
		if now.Sub(info.ModTime()) >= defaultMaxAge {
			return w.rotate("age")
		}
	}

	return nil
}

func (w *rotatingWriter) rotate(reason string) error {
	// Close current file
	if err := w.file.Close(); err != nil {
		return fmt.Errorf("close log file: %w", err)
	}

	// Generate rotated filename with timestamp
	timestamp := time.Now().UTC().Format("2006-01-02T15-04-05")
	rotatedPath := fmt.Sprintf("%s.%s.json", w.filePath, timestamp)

	// Rename current file to rotated name
	if err := os.Rename(w.filePath, rotatedPath); err != nil {
		return fmt.Errorf("rename log file: %w", err)
	}

	// Open new log file
	file, err := os.OpenFile(w.filePath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return fmt.Errorf("open new log file: %w", err)
	}

	w.file = file
	w.size = 0

	// Compress old rotated files and cleanup
	go func() {
		w.compressOldLogs()
		w.cleanupOldLogs()
	}()

	return nil
}

func (w *rotatingWriter) compressOldLogs() {
	logDir := filepath.Dir(w.filePath)
	entries, err := os.ReadDir(logDir)
	if err != nil {
		return
	}

	now := time.Now()
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		name := entry.Name()
		// Skip already compressed files
		if strings.HasSuffix(name, ".gz") {
			continue
		}

		// Only compress files matching our rotated log pattern
		if !strings.HasPrefix(name, "agent.log.") || !strings.HasSuffix(name, ".json") {
			continue
		}

		// Extract timestamp from filename
		// Format: agent.log.2024-01-15T12-00-00.json
		parts := strings.Split(name, ".")
		if len(parts) < 4 {
			continue
		}

		timestampStr := parts[2] + "T" + parts[3]
		timestamp, err := time.Parse("2006-01-02T15-04-05", timestampStr)
		if err != nil {
			continue
		}

		// Compress if older than compressAfterAge
		if now.Sub(timestamp) >= compressAfterAge {
			filePath := filepath.Join(logDir, name)
			w.compressFile(filePath)
		}
	}
}

func (w *rotatingWriter) compressFile(filePath string) {
	// Check if already compressed
	if strings.HasSuffix(filePath, ".gz") {
		return
	}

	// Read file
	data, err := os.ReadFile(filePath)
	if err != nil {
		return
	}

	// Create compressed file
	compressedPath := filePath + ".gz"
	compressedFile, err := os.Create(compressedPath)
	if err != nil {
		return
	}
	defer compressedFile.Close()

	// Write compressed data
	gzWriter := gzip.NewWriter(compressedFile)
	if _, err := gzWriter.Write(data); err != nil {
		gzWriter.Close()
		return
	}
	if err := gzWriter.Close(); err != nil {
		return
	}

	// Remove original file
	os.Remove(filePath)
}

func (w *rotatingWriter) cleanupOldLogs() {
	logDir := filepath.Dir(w.filePath)
	entries, err := os.ReadDir(logDir)
	if err != nil {
		return
	}

	// Find all rotated log files
	var rotatedFiles []os.DirEntry
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if strings.HasPrefix(name, "agent.log.") && (strings.HasSuffix(name, ".json") || strings.HasSuffix(name, ".json.gz")) {
			rotatedFiles = append(rotatedFiles, entry)
		}
	}

	// Sort by modification time (oldest first)
	sort.Slice(rotatedFiles, func(i, j int) bool {
		infoI, _ := rotatedFiles[i].Info()
		infoJ, _ := rotatedFiles[j].Info()
		return infoI.ModTime().Before(infoJ.ModTime())
	})

	// Delete oldest files if we exceed maxRotatedFiles
	if len(rotatedFiles) > maxRotatedFiles {
		for i := 0; i < len(rotatedFiles)-maxRotatedFiles; i++ {
			filePath := filepath.Join(logDir, rotatedFiles[i].Name())
			os.Remove(filePath)
		}
	}
}

func (w *rotatingWriter) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.file.Close()
}
