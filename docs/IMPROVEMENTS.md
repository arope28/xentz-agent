# Security & Reliability Improvements

This document tracks important improvements and security considerations for future development.

## High Priority

### 1. Auto-Init Repository Safety ✅

**Status:** ✅ **IMPLEMENTED** - Gated behind `--auto-init` flag

**Implementation:**
- ✅ Added `--auto-init` flag to `backup` command (default: `false`)
- ✅ Renamed `ensureRepoInitialized()` to `checkOrInitRepo()` with `autoInit` parameter
- ✅ Without flag: Backup fails with clear error if repository doesn't exist
- ✅ With flag: Automatically initializes repository if missing
- ✅ Updated `backup.Run()` signature to accept `autoInit bool` parameter
- ✅ Updated usage documentation

**Current Behavior:**
- Default: Backup fails if repository doesn't exist (safe)
- With `--auto-init`: Automatically initializes repository if missing
- Clear error message guides users to use `--auto-init` or run `restic init` manually

**Code Changes:**
- `internal/backup/backup.go`: `checkOrInitRepo()` function with `autoInit` parameter
- `cmd/xentz-agent/main.go`: Added `--auto-init` flag to backup command
- Updated function signature: `backup.Run(ctx, cfg, autoInit bool)`

**Usage:**
```bash
# Safe default: fails if repo doesn't exist
xentz-agent backup

# Explicit opt-in: auto-initializes if missing
xentz-agent backup --auto-init
```

---

### 2. URL Validation: Private IP Handling ✅

**Status:** ✅ **IMPLEMENTED** - Added documentation and strict validation option

**Implementation:**
- ✅ Added comprehensive documentation explaining private IP allowance rationale
- ✅ Added `ValidateServerURLStrict()` function for strict SSRF protection
- ✅ Standard `ValidateServerURL()` allows private IPs (for enterprise/internal deployments)
- ✅ Strict mode blocks private IPs (for public-only control planes)

**Current Behavior:**
- **Standard mode** (`ValidateServerURL`):
  - Blocks: `localhost`, `127.0.0.1`, `::1`
  - Allows: Private RFC1918 IPs (10.0.0.0/8, 172.16.0.0/12, 192.168.0.0/16)
  - Allows: Other private/internal IPs
  - **Rationale:** Many enterprise deployments use internal control plane servers on private networks

- **Strict mode** (`ValidateServerURLStrict`):
  - Blocks: All private IPs, loopback, link-local addresses
  - Allows: Only public IPs and hostnames (DNS will resolve)
  - Use when you only want to allow public control plane servers

**Code Changes:**
- `internal/validation/url.go`: 
  - Enhanced documentation for `ValidateServerURL()`
  - Added `ValidateServerURLStrict()` function
  - Uses `net.IP.IsPrivate()` for reliable private IP detection

**Usage:**
```go
// Standard validation (allows private IPs)
err := validation.ValidateServerURL("http://10.0.0.1:8080") // ✅ Allowed

// Strict validation (blocks private IPs)
err := validation.ValidateServerURLStrict("http://10.0.0.1:8080") // ❌ Blocked
err := validation.ValidateServerURLStrict("https://control.example.com") // ✅ Allowed
```

**Future Enhancement:**
- Consider adding config option to choose validation mode per deployment

---

## Medium Priority

### 3. Report Spool Cleanup ✅

**Current Behavior:**
- `CleanupOldReports()` exists and removes reports older than 30 days
- Spool directory has 100MB size limit
- **Status:** ✅ Verified - Called in both backup and retention commands

**Verification:**
- ✅ Called in `backup` command: `cmd/xentz-agent/main.go:334`
- ✅ Called in `retention` command: `cmd/xentz-agent/main.go:430`
- ✅ Runs even when server is unreachable (local file operations)
- ✅ Runs after sending reports (prevents accumulation)

**Recommendations:**
1. ✅ **Complete** - Cleanup is called in both flows
2. ✅ **Complete** - Cleanup runs after sending reports
3. Consider adding metrics/logging for cleanup operations (optional enhancement)

**Implementation Notes:**
- Function: `internal/report/report.go:317`
- Called in: `cmd/xentz-agent/main.go:334` (backup) and `:430` (retention)
- Max age: 30 days (configurable via parameter)

---

### 4. Secret Storage Improvements 🔐

**Current Behavior:**
- Device API key and restic password are stored in OS-native secret stores when available ✅
- Linux uses libsecret if present, otherwise falls back to an on-disk secret store under `<CONFIG_DIR>/secrets` with `0600` perms ✅
- Config file stores non-sensitive settings only ✅

**Security Assessment:**
- **Day 1:** OS-native stores are used where available
- **Fallback:** File-based secrets are protected with strict permissions

**Implementation Notes:**
- Implemented in `internal/secretstore/*` with per-OS backends
- Legacy password files are migrated into secretstore on first use

---

## Low Priority

### 5. Distribution Binaries in Repository 📦

**Current Behavior:**
- `dist/` directory contains prebuilt binaries
- `.gitignore` already excludes `dist/` ✅

**Status:** ✅ Already handled correctly

**Verification:**
- `.gitignore` line 11: `dist/` is ignored
- Binaries should not be committed to repository
- Release artifacts should be uploaded to GitHub Releases

**Recommendations:**
1. ✅ Ensure `dist/` stays in `.gitignore`
2. ✅ Use GitHub Releases for distribution
3. ✅ Document build process in `GITHUB_RELEASE.md` (already exists)
4. Consider: Add pre-commit hook to prevent accidental commits of binaries

**Implementation Notes:**
- Current: `GITHUB_RELEASE.md` documents release process
- Build scripts: `build.sh`, `build.bat` output to `dist/`
- Installers download from GitHub Releases, not from repo

---

## Implementation Checklist

- [ ] Add `--auto-init` flag or restrict auto-init to install only
- [ ] Document private IP allowance rationale (or add strict mode)
- [ ] Verify `CleanupOldReports()` is called in backup and retention flows
- [ ] Add OS credential storage integration (future enhancement)
- [ ] Ensure `dist/` remains in `.gitignore` (already done ✅)

---

## Notes

- These improvements are **good-to-fix** items, not critical bugs
- Current implementation is secure for MVP use cases
- Prioritize based on deployment environment and threat model
- Consider user feedback before implementing breaking changes

