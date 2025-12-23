# GitHub Release Cheatsheet

Quick reference for building executables and creating GitHub releases.

## Prerequisites

1. **Install Go** (if not already installed):
   ```bash
   # macOS
   brew install go
   
   # Linux
   sudo apt install golang-go  # or your package manager
   ```

2. **Install GitHub CLI** (`gh`):
   ```bash
   # macOS
   brew install gh
   
   # Linux
   # Follow instructions at: https://cli.github.com/manual/installation
   ```

3. **Authenticate with GitHub**:
   ```bash
   gh auth login
   ```

## Building Executables

### Quick Build (All Platforms)

**On macOS/Linux:**
```bash
chmod +x build.sh
./build.sh
```

**On Windows:**
```cmd
build.bat
```

This creates all executables in the `dist/` directory:
- `xentz-agent-darwin-amd64` (macOS Intel)
- `xentz-agent-darwin-arm64` (macOS Apple Silicon)
- `xentz-agent-darwin-universal` (macOS Universal - both architectures)
- `xentz-agent-windows-amd64.exe` (Windows 64-bit)
- `xentz-agent-windows-arm64.exe` (Windows ARM)
- `xentz-agent-linux-amd64` (Linux 64-bit)
- `xentz-agent-linux-arm64` (Linux ARM64)
- `xentz-agent-linux-armv7` (Linux ARMv7 - Raspberry Pi)

### Build Specific Platform

```bash
# macOS Intel
GOOS=darwin GOARCH=amd64 go build -ldflags="-s -w" -o xentz-agent-darwin-amd64 ./cmd/xentz-agent

# macOS Apple Silicon
GOOS=darwin GOARCH=arm64 go build -ldflags="-s -w" -o xentz-agent-darwin-arm64 ./cmd/xentz-agent

# Windows 64-bit
GOOS=windows GOARCH=amd64 go build -ldflags="-s -w" -o xentz-agent-windows-amd64.exe ./cmd/xentz-agent

# Linux 64-bit
GOOS=linux GOARCH=amd64 go build -ldflags="-s -w" -o xentz-agent-linux-amd64 ./cmd/xentz-agent
```

## Creating a GitHub Release

### Step 1: Update Version (Optional but Recommended)

Update version in code or use git tags. The build script uses date as version by default.

### Step 2: Build All Executables

```bash
./build.sh
```

Verify all files are in `dist/`:
```bash
ls -lh dist/
```

### Step 3: Create Release with GitHub CLI

**Create a new release with all assets:**
```bash
# Create release with tag and upload all binaries + installers
gh release create v1.0.0 \
  dist/xentz-agent-* \
  install.sh \
  install.ps1 \
  --title "v1.0.0" \
  --notes "Release notes here"
```

**Or create release first, then upload assets:**
```bash
# Create release without assets
gh release create v1.0.0 \
  --title "v1.0.0" \
  --notes "Release notes here"

# Upload all binaries
gh release upload v1.0.0 dist/xentz-agent-*

# Upload installers
gh release upload v1.0.0 install.sh install.ps1
```

### Step 4: Verify Release

1. Check GitHub: https://github.com/arope28/xentz-agent/releases
2. Test download URLs:
   ```bash
   # Test latest release download
   curl -I https://github.com/arope28/xentz-agent/releases/latest/download/xentz-agent-darwin-universal
   ```

## Complete Release Workflow

Here's a complete workflow from start to finish:

```bash
# 1. Ensure you're on main/master branch and up to date
git checkout main
git pull origin main

# 2. Build all executables
./build.sh

# 3. Verify build output
ls -lh dist/

# 4. Commit any changes (if needed)
git add .
git commit -m "Prepare release v1.0.0"
git push origin main

# 5. Create and tag the release
git tag -a v1.0.0 -m "Release v1.0.0"
git push origin v1.0.0

# 6. Create GitHub release with all assets
gh release create v1.0.0 \
  dist/xentz-agent-* \
  install.sh \
  install.ps1 \
  --title "v1.0.0" \
  --notes "## What's New
- Feature 1
- Feature 2
- Bug fixes

## Installation
\`\`\`bash
curl -fsSL https://github.com/arope28/xentz-agent/releases/latest/download/install.sh | bash
\`\`\`"
```

## Updating an Existing Release

If you need to add files to an existing release:

```bash
# Upload additional files
gh release upload v1.0.0 dist/xentz-agent-linux-armv7

# Or replace all assets (delete and recreate)
gh release delete v1.0.0 --yes
gh release create v1.0.0 dist/xentz-agent-* install.sh install.ps1 --title "v1.0.0" --notes "..."
```

## Release Checklist

Before creating a release, verify:

- [ ] All tests pass (if you have tests)
- [ ] Code is committed and pushed
- [ ] Version number is updated (if using semantic versioning)
- [ ] All executables build successfully
- [ ] Installer scripts are up to date
- [ ] Release notes are prepared
- [ ] Documentation is updated (README.md, INSTALL.md)

## Common Commands Reference

```bash
# List all releases
gh release list

# View a specific release
gh release view v1.0.0

# Download a release asset
gh release download v1.0.0 --pattern "*.exe"

# Delete a release (use with caution)
gh release delete v1.0.0 --yes

# Edit release notes
gh release edit v1.0.0 --notes "Updated notes"
```

## Troubleshooting

### "Release.tag_name already exists"

The tag already exists. Options:
1. Use a new version number: `v1.0.1`
2. Delete the existing tag and release:
   ```bash
   gh release delete v1.0.0 --yes
   git tag -d v1.0.0
   git push origin :refs/tags/v1.0.0
   ```

### "asset under the same name already exists"

The release already has files with those names. Options:
1. Delete and recreate the release
2. Use different filenames
3. Upload to a new version

### Build fails on macOS for universal binary

If `lipo` is not found:
- Universal binary creation is skipped (not critical)
- Individual architecture binaries are still created
- Users can use architecture-specific binaries

### "gh: command not found"

Install GitHub CLI:
```bash
# macOS
brew install gh

# Then authenticate
gh auth login
```

## Best Practices

1. **Use semantic versioning**: `v1.0.0`, `v1.0.1`, `v1.1.0`, etc.
2. **Always include installers**: `install.sh` and `install.ps1` should be in every release
3. **Write release notes**: Document what changed, new features, bug fixes
4. **Test before releasing**: Download and test at least one binary from the release
5. **Tag releases**: Use git tags to mark releases in your repository
6. **Keep releases**: Don't delete old releases unless necessary (users may depend on them)

## Quick Reference

```bash
# Build everything
./build.sh

# Create release (all-in-one)
gh release create v1.0.0 dist/xentz-agent-* install.sh install.ps1 --title "v1.0.0" --notes "..."

# View releases
gh release list

# Download latest installer
curl -fsSL https://github.com/arope28/xentz-agent/releases/latest/download/install.sh | bash
```

