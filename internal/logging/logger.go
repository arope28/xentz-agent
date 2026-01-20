package logging

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"xentz-agent/internal/paths"
)

// LogLevel represents the log level
type LogLevel string

const (
	LevelDebug LogLevel = "debug"
	LevelInfo  LogLevel = "info"
	LevelWarn  LogLevel = "warn"
	LevelError LogLevel = "error"
)

// LogEntry represents a single log entry
type LogEntry struct {
	Timestamp string                 `json:"timestamp"`
	Level     string                 `json:"level"`
	Message   string                 `json:"message"`
	TenantID  string                 `json:"tenant_id,omitempty"`
	DeviceID  string                 `json:"device_id,omitempty"`
	Component string                 `json:"component,omitempty"`
	Error     string                 `json:"error,omitempty"`
	Fields    map[string]interface{} `json:"fields,omitempty"`
}

// Logger provides structured JSON logging
type Logger struct {
	tenantID  string
	deviceID  string
	writer    *rotatingWriter
	mu        sync.Mutex
	minLevel  LogLevel
	component string
}

// NewLogger creates a new logger instance
func NewLogger(tenantID, deviceID string) (*Logger, error) {
	logDir, err := paths.LogDir("")
	if err != nil {
		return nil, fmt.Errorf("resolve log dir: %w", err)
	}
	if err := os.MkdirAll(logDir, 0o700); err != nil {
		return nil, fmt.Errorf("create log directory: %w", err)
	}

	logPath := filepath.Join(logDir, "agent.log")
	writer, err := newRotatingWriter(logPath)
	if err != nil {
		return nil, fmt.Errorf("create rotating writer: %w", err)
	}

	return &Logger{
		tenantID: tenantID,
		deviceID: deviceID,
		writer:   writer,
		minLevel: LevelInfo, // Default to info level
	}, nil
}

// SetComponent sets the component name for subsequent log entries
func (l *Logger) SetComponent(component string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.component = component
}

// SetLevel sets the minimum log level
func (l *Logger) SetLevel(level LogLevel) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.minLevel = level
}

// shouldLog checks if a log level should be logged
func (l *Logger) shouldLog(level LogLevel) bool {
	levels := map[LogLevel]int{
		LevelDebug: 0,
		LevelInfo:  1,
		LevelWarn:  2,
		LevelError: 3,
	}
	return levels[level] >= levels[l.minLevel]
}

// log writes a log entry
func (l *Logger) log(level LogLevel, message string, err error, fields map[string]interface{}) {
	if !l.shouldLog(level) {
		return
	}

	entry := LogEntry{
		Timestamp: time.Now().UTC().Format(time.RFC3339Nano),
		Level:     string(level),
		Message:   message,
		TenantID:  l.tenantID,
		DeviceID:  l.deviceID,
		Component: l.component,
		Fields:    fields,
	}

	if err != nil {
		entry.Error = err.Error()
	}

	// Sanitize fields to remove sensitive data
	if entry.Fields != nil {
		entry.Fields = sanitizeFields(entry.Fields)
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	jsonData, jsonErr := json.Marshal(entry)
	if jsonErr != nil {
		// Fallback to simple text log if JSON marshaling fails
		fmt.Fprintf(os.Stderr, "log marshal error: %v\n", jsonErr)
		return
	}

	if writeErr := l.writer.Write(jsonData); writeErr != nil {
		fmt.Fprintf(os.Stderr, "log write error: %v\n", writeErr)
	}
}

// Info logs an info message
func (l *Logger) Info(message string, fields map[string]interface{}) {
	l.log(LevelInfo, message, nil, fields)
}

// Error logs an error message
func (l *Logger) Error(message string, err error, fields map[string]interface{}) {
	l.log(LevelError, message, err, fields)
}

// Warn logs a warning message
func (l *Logger) Warn(message string, fields map[string]interface{}) {
	l.log(LevelWarn, message, nil, fields)
}

// Debug logs a debug message
func (l *Logger) Debug(message string, fields map[string]interface{}) {
	l.log(LevelDebug, message, nil, fields)
}

// Close closes the logger and flushes any pending writes
func (l *Logger) Close() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.writer.Close()
}

// sanitizeFields removes sensitive data from log fields
func sanitizeFields(fields map[string]interface{}) map[string]interface{} {
	sanitized := make(map[string]interface{})
	sensitiveKeys := map[string]bool{
		"password":       true,
		"api_key":        true,
		"device_api_key": true,
		"token":          true,
		"install_token":  true,
		"secret":         true,
	}

	for k, v := range fields {
		if sensitiveKeys[k] {
			sanitized[k] = "[REDACTED]"
		} else {
			sanitized[k] = v
		}
	}
	return sanitized
}
