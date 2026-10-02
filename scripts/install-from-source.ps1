<#
.SYNOPSIS
  One-command install of WhatsApp Doppel from a source checkout on Windows.

.DESCRIPTION
  Builds WhatsappDoppel.exe (no console window; with the app icon when go-winres can be
  fetched, otherwise without) and runs scripts\windows\install.ps1: the app goes to
  %LOCALAPPDATA%\Programs\WhatsappDoppel with Start menu and Desktop shortcuts. No admin
  rights needed. Needs Go (https://go.dev/dl/); it never installs system packages for you.

    powershell -NoProfile -ExecutionPolicy Bypass -File scripts\install-from-source.ps1 [-NoLaunch | -Launch]
#>
[CmdletBinding()]
param(
  [switch]$Launch,
  [switch]$NoLaunch,
  [switch]$NoDesktopShortcut
)
Set-StrictMode -Version 2.0
$ErrorActionPreference = 'Stop'

$root = Split-Path -Parent (Split-Path -Parent $MyInvocation.MyCommand.Path)
Set-Location -LiteralPath $root

function Step([string]$msg) { Write-Host "==> $msg" -ForegroundColor Cyan }

# Runs a native command; returns its exit code. Its stderr (Go prints progress
# there) must not turn into PowerShell errors on Windows PowerShell 5.1.
function Invoke-Native([string]$file, [string[]]$arguments) {
  $old = $ErrorActionPreference
  $ErrorActionPreference = 'Continue'
  try {
    & $file @arguments 2>&1 | ForEach-Object { Write-Host "$_" }
    return $LASTEXITCODE
  } finally { $ErrorActionPreference = $old }
}

function Show-GoHelp {
  Write-Host ''
  Write-Host 'Install Go, open a new PowerShell window and run this script again:'
  Write-Host '  - the Windows installer (.msi) from https://go.dev/dl/'
  Write-Host '  - or:  winget install --id GoLang.Go -e'
  Write-Host '  (any Go 1.21 or newer works: it downloads the exact version this project needs by itself)'
}

$go = Get-Command go -ErrorAction SilentlyContinue
if (-not $go) {
  Write-Host 'Go is not installed (or not on your PATH).' -ForegroundColor Red
  Show-GoHelp
  exit 1
}
$old = $ErrorActionPreference; $ErrorActionPreference = 'Continue'
$gov = (& go env GOVERSION 2>$null | Out-String).Trim()
$arch = (& go env GOARCH 2>$null | Out-String).Trim()
$version = (& git describe --tags --always --dirty 2>$null | Out-String).Trim()
$ErrorActionPreference = $old
if (-not ($gov -match '^go1\.(\d+)') -or [int]$Matches[1] -lt 21) {
  Write-Host "Go $gov is too old; Go 1.21 or newer is needed." -ForegroundColor Red
  Show-GoHelp
  exit 1
}
if (-not $version) { $version = 'dev' }
Step "Using $gov ($arch), version $version"

New-Item -ItemType Directory -Force -Path (Join-Path $root 'build') | Out-Null
$exeOut = Join-Path $root 'build\WhatsappDoppel.exe'
$syso = Join-Path $root "rsrc_windows_$arch.syso"
try {
  Step 'Rendering the icon'
  $icon = Join-Path $root 'build\icon.ico'
  if ((Invoke-Native 'go' @('run', './scripts/geniconn', '-ico', $icon)) -eq 0) {
    Step 'Adding icon and version info (go-winres)'
    $code = Invoke-Native 'go' @('run', 'github.com/tc-hib/go-winres@v0.3.3', 'simply', '--arch', $arch, '--out', 'rsrc',
      '--manifest', 'gui', '--icon', $icon, '--product-name', 'WhatsApp Doppel', '--file-description', 'WhatsApp Doppel',
      '--original-filename', 'WhatsappDoppel.exe', '--product-version', $version)
    if ($code -ne 0) { Write-Warning 'Could not add the icon (offline?). Building without it.' }
  } else {
    Write-Warning 'Could not render the icon. Building without it.'
  }

  Step 'Building WhatsappDoppel.exe (pure Go; the first build takes a minute or two)'
  $env:CGO_ENABLED = '0'
  $code = Invoke-Native 'go' @('build', '-trimpath', '-ldflags', "-s -w -H=windowsgui -X main.version=$version", '-o', $exeOut, '.')
  if ($code -ne 0) { throw "go build failed (exit $code)" }
} finally {
  Remove-Item -LiteralPath $syso -Force -ErrorAction SilentlyContinue
}

$installArgs = @{ Source = $exeOut }
if ($Launch) { $installArgs.Launch = $true }
if ($NoLaunch) { $installArgs.NoLaunch = $true }
if ($NoDesktopShortcut) { $installArgs.NoDesktopShortcut = $true }
& (Join-Path $root 'scripts\windows\install.ps1') @installArgs
exit $LASTEXITCODE
