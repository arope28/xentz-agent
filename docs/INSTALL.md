# Installation Guide

## Automatic Installation (Recommended)

The installers automatically detect your operating system and CPU architecture, then download and install the correct binary.

### macOS / Linux

Run this one-liner:

```bash
curl -fsSL https://github.com/arope28/xentz-agent/releases/latest/download/install.sh | bash
```

Or download and run manually:

```bash
# Download the installer
curl -fsSL -o install.sh https://github.com/arope28/xentz-agent/releases/latest/download/install.sh

# Make it executable
chmod +x install.sh

# Run it
./install.sh
```

### Windows

**PowerShell (Recommended):**

```powershell
irm https://github.com/arope28/xentz-agent/releases/latest/download/install.ps1 | iex
```

Or download and run manually:

```powershell
# Download the installer
Invoke-WebRequest -Uri https://github.com/arope28/xentz-agent/releases/latest/download/install.ps1 -OutFile install.ps1

# Run it
.\install.ps1
```

**Note:** If you get an execution policy error, run:
```powershell
Set-ExecutionPolicy -ExecutionPolicy RemoteSigned -Scope CurrentUser
```

### How It Works

1. **Detects your platform:**
   - Operating system (macOS, Linux, Windows)
   - CPU architecture (amd64, arm64, armv7)

2. **Downloads the correct binary:**
   - For macOS: Prefers universal binary (works on both Intel and Apple Silicon)
   - For Windows: Downloads amd64 or arm64 based on your system
   - For Linux: Downloads amd64, arm64, or armv7 based on your system

3. **Installs to a standard location:**
   - macOS: `/usr/local/bin/xentz-agent` (requires sudo during installation)
   - Linux: `~/.local/bin/xentz-agent` (user-specific)
   - Windows: `%LOCALAPPDATA%\xentz-agent\xentz-agent.exe` (user-specific)

4. **Provides next steps:**
   - Instructions to add to PATH (if needed)
   - Commands to install restic (if not already installed)
   - Example install command with token-based enrollment

## Manual Installation

If you prefer to download manually:

1. Go to the [Releases page](https://github.com/arope28/xentz-agent/releases)
2. Download the binary for your platform:
   - **macOS Intel**: `xentz-agent-darwin-amd64`
   - **macOS Apple Silicon**: `xentz-agent-darwin-arm64` or `xentz-agent-darwin-universal`
   - **Windows 64-bit**: `xentz-agent-windows-amd64.exe`
   - **Windows ARM**: `xentz-agent-windows-arm64.exe`
   - **Linux 64-bit**: `xentz-agent-linux-amd64`
   - **Linux ARM64**: `xentz-agent-linux-arm64`
   - **Linux ARMv7** (Raspberry Pi): `xentz-agent-linux-armv7`

3. Make it executable (macOS/Linux):
   ```bash
   chmod +x xentz-agent-*
   ```

4. Move to a directory in your PATH, or run directly:
   
   **Token-based enrollment (recommended):**
   ```bash
   ./xentz-agent-* install --token <install-token> \
     --server https://control-plane.example.com \
     --daily-at 02:00 \
     --include "/path/to/backup"
   ```
   
   **Legacy mode (direct repository):**
   ```bash
   ./xentz-agent-* install --repo rest:https://your-repo.com/backup \
     --password "your-password" \
     --daily-at 02:00 \
     --include "/path/to/backup"
   ```

## Custom Installation URL

You can override the base URL by setting an environment variable:

**macOS/Linux:**
```bash
XENTZ_AGENT_BASE_URL=https://your-custom-domain.com/releases ./install.sh
```

**Windows:**
```powershell
$env:XENTZ_AGENT_BASE_URL="https://your-custom-domain.com/releases"
.\install.ps1
```

## Troubleshooting

### "Command not found" after installation

The installer adds the binary to different locations based on your OS. If the directory isn't in your PATH:

**macOS:**
The binary is installed to `/usr/local/bin`, which is typically already in PATH. If not, add to `~/.zshrc` or `~/.bash_profile`:
```bash
export PATH="/usr/local/bin:$PATH"
```

**Linux:**
The binary is installed to `~/.local/bin`. Add to `~/.bashrc`, `~/.zshrc`, or `~/.profile`:
```bash
export PATH="$HOME/.local/bin:$PATH"
```

**Windows:**
Add to your user PATH via System Settings, or run:
```powershell
[Environment]::SetEnvironmentVariable('Path', "$env:Path;$env:LOCALAPPDATA\xentz-agent", 'User')
```

### Download fails

- Check your internet connection
- Verify the release exists at the GitHub releases page
- Try downloading manually from the releases page

### Permission denied (macOS/Linux)

**macOS:**
The binary should already be executable after installation. If you get permission errors, you may need to run the installer with sudo (it will prompt you).

**Linux:**
Make sure the binary is executable:
```bash
chmod +x ~/.local/bin/xentz-agent
```

### Windows execution policy error

Run PowerShell as Administrator and execute:
```powershell
Set-ExecutionPolicy -ExecutionPolicy RemoteSigned -Scope CurrentUser
```

## Configuration

### Token-Based Enrollment (Recommended)

After installation, configure the agent using token-based enrollment:

```bash
xentz-agent install --token <install-token> \
  --server https://control-plane.example.com \
  --daily-at 02:00 \
  --include "/Users/yourname/Documents" \
  --include "/Users/yourname/Pictures"
```

The agent will:
1. Enroll with the control plane using the install token
2. Receive server-assigned identifiers (tenant_id, device_id, device_api_key)
3. Store the device_api_key for future authentication
4. Set up scheduled backups

### Server-Driven Configuration

Once enrolled, the agent automatically:
- Fetches configuration from the control plane on every backup/retention run
- Uses cached configuration if the server is unreachable
- Applies the latest settings (include paths, schedule, retention policy, etc.)

You can update configuration on the control plane, and it will be applied on the next backup run automatically.

### Legacy Mode (Direct Repository)

If you prefer to configure the repository directly without a control plane:

```bash
xentz-agent install --repo rest:https://your-repo.com/backup \
  --password "your-password" \
  --daily-at 02:00 \
  --include "/path/to/backup"
```

In legacy mode, configuration is stored locally and not fetched from a server.

