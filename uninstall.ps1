# Firefly Framework CLI Uninstaller for Windows
# Usage: .\uninstall.ps1

$ErrorActionPreference = "Stop"

$Binary = "flywork"

function Write-Info($msg)  { Write-Host "  i $msg" -ForegroundColor Cyan }
function Write-Ok($msg)    { Write-Host "  ✓ $msg" -ForegroundColor Green }
function Write-Warn($msg)  { Write-Host "  ! $msg" -ForegroundColor Yellow }

Write-Host ""
Write-Info "Firefly Framework CLI Uninstaller"
Write-Host ""

# ── Remove binary ────────────────────────────────────────────────────────────

$InstallDir = Join-Path $env:LOCALAPPDATA "flywork\bin"
$BinPath = Join-Path $InstallDir "$Binary.exe"

if (Test-Path $BinPath) {
    Remove-Item -Force $BinPath
    Write-Ok "Removed $BinPath"
} else {
    Write-Warn "Binary not found at $BinPath"
}

# Also check /usr/local/bin equivalent on Windows
$AltBin = Get-Command $Binary -ErrorAction SilentlyContinue
if ($AltBin) {
    Remove-Item -Force $AltBin.Source -ErrorAction SilentlyContinue
    Write-Ok "Removed $($AltBin.Source)"
}

# ── Remove ~/.flywork directory ──────────────────────────────────────────────

$FlyworkHome = Join-Path $env:USERPROFILE ".flywork"

if (Test-Path $FlyworkHome) {
    $answer = Read-Host "  ? Remove $FlyworkHome and all data? [y/N]"
    if ($answer -eq "y" -or $answer -eq "Y") {
        Remove-Item -Recurse -Force $FlyworkHome
        Write-Ok "Removed $FlyworkHome"
    } else {
        Write-Info "Kept $FlyworkHome"
    }
}

# ── Clean PATH ───────────────────────────────────────────────────────────────

$UserPath = [System.Environment]::GetEnvironmentVariable("Path", "User")
if ($UserPath -like "*$InstallDir*") {
    $CleanPath = ($UserPath -split ";" | Where-Object { $_ -ne $InstallDir }) -join ";"
    [System.Environment]::SetEnvironmentVariable("Path", $CleanPath, "User")
    Write-Ok "Removed $InstallDir from user PATH"
}

# Remove empty install directory
if (Test-Path $InstallDir) {
    $items = Get-ChildItem $InstallDir -ErrorAction SilentlyContinue
    if (-not $items -or $items.Count -eq 0) {
        Remove-Item -Force $InstallDir
    }
}

Write-Host ""
Write-Ok "Flywork CLI uninstalled."
