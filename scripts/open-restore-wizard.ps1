param(
  [ValidateSet("user", "system")]
  [string]$Mode = "user",
  [string]$Addr = "127.0.0.1:9800"
)

$ErrorActionPreference = "Stop"

function Fail([string]$Message) {
  Write-Error $Message
  exit 1
}

if (-not (Get-Command xentz-agent -ErrorAction SilentlyContinue)) {
  Fail "xentz-agent not found in PATH"
}

# Strict loopback host:port parsing
$m = [regex]::Match($Addr, '^(localhost|127\.0\.0\.1):([0-9]{1,5})$')
$m6 = [regex]::Match($Addr, '^\[(::1)\]:([0-9]{1,5})$')
if ($m.Success) {
  $hostName = $m.Groups[1].Value
  $port = [int]$m.Groups[2].Value
} elseif ($m6.Success) {
  $hostName = "::1"
  $port = [int]$m6.Groups[2].Value
} else {
  Fail "invalid --addr format: $Addr (expected localhost:9800, 127.0.0.1:9800, or [::1]:9800)"
}
if ($port -lt 1 -or $port -gt 65535) {
  Fail "invalid port in --addr: $port"
}

if ($Mode -eq "system") {
  $isAdmin = ([Security.Principal.WindowsPrincipal][Security.Principal.WindowsIdentity]::GetCurrent()).
    IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)
  if (-not $isAdmin) {
    Fail "--mode system requires Administrator PowerShell"
  }
}

$baseUrl = if ($hostName -eq "::1") { "http://[::1]:$port" } else { "http://$hostName`:$port" }

# Refuse to reuse existing listeners to avoid token leakage.
$tcp = New-Object System.Net.Sockets.TcpClient
try {
  $iar = $tcp.BeginConnect($hostName, $port, $null, $null)
  $connected = $iar.AsyncWaitHandle.WaitOne(700)
  if ($connected -and $tcp.Connected) {
    Fail "address already in use ($Addr). Refusing tokenized launch on existing listener."
  }
} catch {
  # no active listener is fine
} finally {
  $tcp.Close()
}

$logFile = Join-Path $env:TEMP ("xentz-agent-local-ui-" + [guid]::NewGuid().ToString("N") + ".log")
Start-Process -FilePath "xentz-agent" -ArgumentList @("local-ui", "--addr", $Addr) -WindowStyle Hidden -RedirectStandardOutput $logFile -RedirectStandardError $logFile | Out-Null

$configDir = if ($Mode -eq "system") {
  Join-Path $env:ProgramData "XentzAgent\config"
} else {
  Join-Path $env:APPDATA "XentzAgent\config"
}
$tokenFile = Join-Path $configDir "local-ui.token"

for ($i = 0; $i -lt 40; $i++) {
  $ready = $false
  try {
    $resp = Invoke-WebRequest -Uri "$baseUrl/" -UseBasicParsing -TimeoutSec 1
    if ($resp.Content -like "*xentz-agent Local UI*") {
      $ready = $true
    }
  } catch {}
  if ($ready -and (Test-Path $tokenFile) -and ((Get-Item $tokenFile).Length -gt 0)) {
    break
  }
  Start-Sleep -Milliseconds 250
}

if (-not (Test-Path $tokenFile)) {
  Fail "token file not found: $tokenFile"
}
$token = (Get-Content -Path $tokenFile -Raw).Trim()
if ([string]::IsNullOrWhiteSpace($token)) {
  Fail "token file is empty: $tokenFile"
}

$restoreUrl = "$baseUrl/restore?token=$token"
Start-Process $restoreUrl | Out-Null
Write-Host "Opened Restore Wizard at $baseUrl/restore"
