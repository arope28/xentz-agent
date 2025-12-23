# Security Review - xentz-agent

## Critical Issues

### 1. Path Traversal Vulnerability in DeleteSpooledReport
**Location**: `internal/report/report.go:219-230`

**Issue**: The `filename` parameter is not validated, allowing path traversal attacks:
```go
func DeleteSpooledReport(filename string) error {
    spoolDir, err := getSpoolDir()
    filepath := filepath.Join(spoolDir, filename)  // Vulnerable to "../" attacks
    if err := os.Remove(filepath); err != nil {
```

**Risk**: An attacker could delete arbitrary files by providing filenames like `../../../.ssh/id_rsa`

**Fix**: Validate filename and ensure it stays within spool directory:
```go
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
    filepath := filepath.Join(spoolDir, filename)
    // Double-check the resolved path is within spoolDir
    resolved, err := filepath.EvalSymlinks(filepath)
    if err == nil {
        spoolResolved, _ := filepath.EvalSymlinks(spoolDir)
        if !strings.HasPrefix(resolved, spoolResolved+string(filepath.Separator)) {
            return fmt.Errorf("path traversal detected")
        }
    }
    if err := os.Remove(filepath); err != nil {
        return fmt.Errorf("delete spool file: %w", err)
    }
    return nil
}
```

### 2. Filename Injection in SpoolReport
**Location**: `internal/report/report.go:127-130`

**Issue**: User-controlled data (job, status) is used in filename without sanitization:
```go
filename := fmt.Sprintf("%d-%s-%s.json", timestamp, report.Job, report.Status)
```

**Risk**: If job or status contains path separators or special characters, could create files outside intended directory.

**Fix**: Sanitize job and status values:
```go
// Sanitize job and status to prevent path injection
sanitize := func(s string) string {
    // Remove any path separators and dangerous characters
    s = strings.ReplaceAll(s, "/", "_")
    s = strings.ReplaceAll(s, "\\", "_")
    s = strings.ReplaceAll(s, "..", "_")
    s = strings.ReplaceAll(s, string(filepath.Separator), "_")
    // Limit length
    if len(s) > 50 {
        s = s[:50]
    }
    return s
}
filename := fmt.Sprintf("%d-%s-%s.json", timestamp, sanitize(report.Job), sanitize(report.Status))
```

### 3. Command Injection Risk in Backup Include Paths
**Location**: `internal/backup/backup.go:40-45`

**Issue**: User-provided paths from config are passed directly to `exec.Command`:
```go
args := []string{"backup", "--json"}
for _, ex := range cfg.Exclude {
    args = append(args, "--exclude", ex)
}
args = append(args, cfg.Include...)
cmd := exec.CommandContext(ctx, "restic", args...)
```

**Risk**: While restic should handle this safely, if paths contain unexpected characters, could cause issues. More importantly, if config comes from untrusted server, malicious paths could be injected.

**Mitigation**: This is actually safe because:
- `exec.Command` properly escapes arguments
- Restic validates paths
- Config comes from trusted server (authenticated with device API key)

**Recommendation**: Add validation to ensure paths are reasonable (no null bytes, reasonable length):
```go
func validatePath(path string) error {
    if len(path) == 0 || len(path) > 4096 {
        return fmt.Errorf("path length invalid")
    }
    if strings.Contains(path, "\x00") {
        return fmt.Errorf("path contains null byte")
    }
    return nil
}
```

### 4. SSRF Risk in Server URL Validation
**Location**: `internal/config/fetch.go:22`, `internal/report/report.go:83`, `internal/enroll/enroll.go:103`

**Issue**: Server URLs are not validated, could allow SSRF attacks:
```go
url := fmt.Sprintf("%s/v1/config", serverURL)
```

**Risk**: If serverURL is manipulated (e.g., `http://localhost:22/` or `file:///etc/passwd`), could access internal services.

**Mitigation**: Server URL comes from:
1. User input during install (trusted)
2. Stored in config file (protected by file permissions)

**Fix**: Validate server URLs:
```go
func validateServerURL(url string) error {
    parsed, err := url.Parse(url)
    if err != nil {
        return fmt.Errorf("invalid URL: %w", err)
    }
    // Only allow http/https
    if parsed.Scheme != "http" && parsed.Scheme != "https" {
        return fmt.Errorf("only http/https schemes allowed")
    }
    // Block localhost/internal IPs (optional, depends on use case)
    host := parsed.Hostname()
    if host == "localhost" || host == "127.0.0.1" || strings.HasPrefix(host, "192.168.") {
        return fmt.Errorf("localhost/internal IPs not allowed")
    }
    return nil
}
```

## High Priority Issues

### 5. Sensitive Data in Error Messages
**Location**: Multiple locations

**Issue**: Error messages might leak sensitive information:
- API keys in error messages
- Server URLs in logs
- File paths that reveal user structure

**Examples**:
- `internal/config/fetch.go:44`: Error message mentions "device API key" but doesn't leak the key itself (good)
- `internal/report/report.go:153`: Logs error but doesn't include API key (good)
- `cmd/xentz-agent/main.go:148`: Logs repository path which might be sensitive

**Fix**: Ensure sensitive data is never logged:
```go
// Good: Don't log API keys
log.Printf("warning: failed to send report to server: %v", err)

// Bad: Would leak API key
log.Printf("warning: failed with API key %s: %v", deviceAPIKey, err)
```

**Status**: Currently safe - API keys are not logged directly.

### 6. File Permissions - Some Files Too Permissive
**Location**: `internal/install/windows.go:64`, `internal/install/linux.go:87`, `internal/install/macos_launchd.go:54`

**Issue**: Some files use `0o644` instead of `0o600`:
```go
os.WriteFile(batchFile, []byte(batchContent), 0o644)  // Should be 0o600
```

**Risk**: Other users on the system could read these files, potentially exposing:
- Config paths
- Executable paths
- Log paths

**Fix**: Use `0o600` for all user-specific files:
```go
os.WriteFile(batchFile, []byte(batchContent), 0o600)
```

**Note**: Systemd service files might need `0o644` for systemd to read them, but user-specific files should be `0o600`.

### 7. Error Message Information Leakage
**Location**: `internal/config/fetch.go:50`, `internal/report/report.go:105`

**Issue**: Error messages include full server response which might leak sensitive information:
```go
var errMsg bytes.Buffer
errMsg.ReadFrom(resp.Body)
return fmt.Errorf("config fetch failed (status %d): %s", resp.StatusCode, errMsg.String())
```

**Risk**: Server error messages might contain:
- Internal paths
- Database errors
- Stack traces
- Other sensitive debugging info

**Fix**: Limit error message length and sanitize:
```go
var errMsg bytes.Buffer
io.CopyN(&errMsg, resp.Body, 512) // Limit to 512 bytes
errStr := strings.TrimSpace(errMsg.String())
// Remove potential sensitive patterns
errStr = strings.ReplaceAll(errStr, "\n", " ")
if len(errStr) > 256 {
    errStr = errStr[:256] + "..."
}
return fmt.Errorf("config fetch failed (status %d): %s", resp.StatusCode, errStr)
```

## Medium Priority Issues

### 8. No Rate Limiting on Report Retries
**Location**: `internal/report/report.go:286-326`

**Issue**: `SendPendingReports` sends up to 20 reports without rate limiting.

**Risk**: If server is down and then comes back, could flood server with requests.

**Fix**: Add rate limiting:
```go
for i, report := range reports {
    if i > 0 {
        time.Sleep(100 * time.Millisecond) // Rate limit
    }
    err := SendReport(serverURL, deviceAPIKey, report)
    // ...
}
```

### 9. No Input Validation on Config Fields
**Location**: `internal/config/fetch.go:59-65`

**Issue**: Config validation only checks for empty fields, not malicious values:
```go
if len(cfg.Include) == 0 {
    return Config{}, fmt.Errorf("server config missing required field: include")
}
```

**Risk**: Server could send malicious config with:
- Extremely long paths
- Paths with null bytes
- Too many include/exclude paths

**Fix**: Add validation:
```go
if len(cfg.Include) == 0 {
    return Config{}, fmt.Errorf("server config missing required field: include")
}
if len(cfg.Include) > 1000 {
    return Config{}, fmt.Errorf("too many include paths (max 1000)")
}
for _, path := range cfg.Include {
    if err := validatePath(path); err != nil {
        return Config{}, fmt.Errorf("invalid include path: %w", err)
    }
}
```

### 10. Spool Directory Exhaustion
**Location**: `internal/report/report.go:111-142`

**Issue**: No limit on spool directory size - could fill disk if reports fail continuously.

**Risk**: Disk space exhaustion could crash system or prevent backups.

**Fix**: Add size limit check:
```go
func checkSpoolSize() error {
    spoolDir, err := getSpoolDir()
    if err != nil {
        return err
    }
    var totalSize int64
    entries, _ := os.ReadDir(spoolDir)
    for _, entry := range entries {
        if info, err := entry.Info(); err == nil {
            totalSize += info.Size()
        }
    }
    const maxSpoolSize = 100 * 1024 * 1024 // 100MB
    if totalSize > maxSpoolSize {
        return fmt.Errorf("spool directory too large: %d bytes (max %d)", totalSize, maxSpoolSize)
    }
    return nil
}
```

## Low Priority / Best Practices

### 11. Use filepath.Clean for All Path Operations
**Location**: Multiple locations

**Recommendation**: Always use `filepath.Clean()` before using paths:
```go
filepath := filepath.Clean(filepath.Join(spoolDir, filename))
```

### 12. Add Context Timeout to HTTP Requests
**Location**: `internal/config/fetch.go:31`, `internal/report/report.go:92`

**Status**: Already implemented - timeouts are set (30 seconds).

### 13. Validate JSON Structure More Strictly
**Location**: `internal/backup/backup.go:135-210`

**Recommendation**: Add stricter validation for restic JSON parsing to prevent crashes on malformed JSON.

### 14. Use Secure Random for Temporary Files
**Location**: N/A (not currently used, but if added)

**Recommendation**: If temporary files are needed, use `os.CreateTemp()` instead of predictable names.

## Summary of Fixes Applied

✅ **FIXED**: Path traversal in `DeleteSpooledReport` - Added filename validation and path traversal checks
✅ **FIXED**: Filename injection in `SpoolReport` - Added sanitization function for job/status values
✅ **FIXED**: SSRF protection - Added server URL validation (blocks localhost, allows private IPs for internal servers)
✅ **FIXED**: Error message sanitization - Limited error messages to 512 bytes, sanitized output
✅ **FIXED**: Rate limiting - Added 100ms delay between pending report sends
✅ **FIXED**: Input validation - Added validation for config paths (length, null bytes, max counts)
✅ **FIXED**: Spool directory size limits - Added 100MB size check before writing reports

## Remaining Recommendations

1. **HIGH**: Fix file permissions (0o644 → 0o600 where appropriate) - Installer files may need 0o644 for systemd/launchd, but user-specific files should be 0o600
2. **LOW**: Consider adding TLS certificate pinning for production use
3. **LOW**: Consider adding request signing for additional security

## Security Best Practices Already Implemented

✅ API keys never logged directly
✅ Sensitive files use 0o600 permissions (config, password files)
✅ HTTP timeouts implemented
✅ Error messages truncated (4096 bytes for reports)
✅ File operations use filepath.Join (prevents some path issues)
✅ Command execution uses exec.Command (properly escapes arguments)
✅ Bearer token authentication
✅ Non-blocking error handling (reports don't fail backups)

