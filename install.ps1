# Firefly Framework CLI Installer for Windows
# Usage:
#   irm https://raw.githubusercontent.com/fireflyframework/fireflyframework-cli/main/install.ps1 | iex
#   $env:FLYWORK_VERSION = "v26.02.02"; .\install.ps1

$ErrorActionPreference = "Stop"

$Repo = "fireflyframework/fireflyframework-cli"
$Binary = "flywork"

# ── Banner ───────────────────────────────────────────────────────────────────

Write-Host ""
Write-Host "  _____.__                _____.__" -ForegroundColor Cyan
Write-Host "_/ ____\__|______   _____/ ____\  | ___.__. " -ForegroundColor Cyan
Write-Host "\   __\|  \_  __ \_/ __ \   __\|  |<   |  |" -ForegroundColor Cyan
Write-Host " |  |  |  ||  | \/\  ___/|  |  |  |_\___  |" -ForegroundColor Cyan
Write-Host " |__|  |__||__|    \___  >__|  |____/ ____|" -ForegroundColor Cyan
Write-Host "                       \/           \/" -ForegroundColor Cyan
Write-Host "  _____                                                 __" -ForegroundColor Cyan
Write-Host "_/ ____\___________    _____   ______  _  _____________|  | __" -ForegroundColor Cyan
Write-Host "\   __\\_  __ \__  \  /     \_/ __ \ \/ \/ /  _ \_  __ \  |/ /" -ForegroundColor Cyan
Write-Host " |  |   |  | \// __ \|  Y Y  \  ___/\     (  <_> )  | \/    <" -ForegroundColor Cyan
Write-Host " |__|   |__|  (____  /__|_|  /\___  >\/\_/ \____/|__|  |__|_ \" -ForegroundColor Cyan
Write-Host "                   \/      \/     \/                        \/" -ForegroundColor Cyan
Write-Host ""
Write-Host "  Firefly Framework CLI Installer" -ForegroundColor Cyan
Write-Host ""

function Write-Info($msg)    { Write-Host "  i $msg" -ForegroundColor Cyan }
function Write-Ok($msg)      { Write-Host "  ✓ $msg" -ForegroundColor Green }
function Write-Warn($msg)    { Write-Host "  ! $msg" -ForegroundColor Yellow }
function Write-Err($msg)     { Write-Host "  ✗ $msg" -ForegroundColor Red; exit 1 }

# ── Detect architecture ──────────────────────────────────────────────────────

$Arch = if ([System.Runtime.InteropServices.RuntimeInformation]::OSArchitecture -eq "Arm64") { "arm64" } else { "amd64" }
Write-Info "Detected platform: windows/$Arch"

# ── Resolve version ──────────────────────────────────────────────────────────

$Version = $env:FLYWORK_VERSION
if (-not $Version) {
    Write-Info "Fetching latest release..."
    try {
        $release = Invoke-RestMethod -Uri "https://api.github.com/repos/$Repo/releases/latest" -Headers @{ "User-Agent" = "flywork-installer" }
        $Version = $release.tag_name
    } catch {
        Write-Err "Could not determine latest version: $_"
    }
}
# Ensure version starts with 'v' (matches Makefile archive naming)
if ($Version -notmatch "^v") { $Version = "v$Version" }
Write-Info "Version: $Version"

# ── Resolve install directory ────────────────────────────────────────────────

$InstallDir = $env:FLYWORK_INSTALL_DIR
if (-not $InstallDir) {
    $InstallDir = Join-Path $env:LOCALAPPDATA "flywork\bin"
}

if (-not (Test-Path $InstallDir)) {
    New-Item -ItemType Directory -Path $InstallDir -Force | Out-Null
}
Write-Info "Install directory: $InstallDir"

# ── Download ─────────────────────────────────────────────────────────────────

$ArchiveName = "$Binary-$Version-windows-$Arch.zip"
$DownloadUrl = "https://github.com/$Repo/releases/download/$Version/$ArchiveName"
$TmpDir = Join-Path ([System.IO.Path]::GetTempPath()) "flywork-install-$(Get-Random)"
New-Item -ItemType Directory -Path $TmpDir -Force | Out-Null

try {
    Write-Info "Downloading $ArchiveName..."
    Invoke-WebRequest -Uri $DownloadUrl -OutFile (Join-Path $TmpDir $ArchiveName) -UseBasicParsing
} catch {
    Write-Err "Download failed — check that version $Version exists for windows/$Arch"
}

# ── Extract & install ────────────────────────────────────────────────────────

Write-Info "Extracting..."
Expand-Archive -Path (Join-Path $TmpDir $ArchiveName) -DestinationPath $TmpDir -Force

$ExtractedBin = Get-ChildItem -Path $TmpDir -Recurse -Filter "$Binary.exe" | Select-Object -First 1
if (-not $ExtractedBin) {
    Write-Err "Binary not found in archive"
}

Copy-Item -Path $ExtractedBin.FullName -Destination (Join-Path $InstallDir "$Binary.exe") -Force
Write-Ok "Installed $Binary.exe to $InstallDir"

# ── Update PATH ──────────────────────────────────────────────────────────────

$UserPath = [System.Environment]::GetEnvironmentVariable("Path", "User")
if ($UserPath -notlike "*$InstallDir*") {
    $NewPath = "$InstallDir;$UserPath"
    [System.Environment]::SetEnvironmentVariable("Path", $NewPath, "User")
    $env:Path = "$InstallDir;$env:Path"
    Write-Warn "Added $InstallDir to user PATH — restart your terminal for changes to take effect"
}

# ── Cleanup ──────────────────────────────────────────────────────────────────

Remove-Item -Recurse -Force $TmpDir -ErrorAction SilentlyContinue

# ── Verify ───────────────────────────────────────────────────────────────────

Write-Host ""
try {
    & (Join-Path $InstallDir "$Binary.exe") version
    Write-Ok "Installation complete!"
} catch {
    Write-Warn "Installed, but could not verify. Restart your terminal and run: flywork version"
}

Write-Host ""
Write-Info "Get started: flywork setup"
