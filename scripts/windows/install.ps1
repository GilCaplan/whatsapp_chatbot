<#
.SYNOPSIS
  Installs WhatsApp Doppel for the current user (no administrator rights needed).

.DESCRIPTION
  Copies WhatsappDoppel.exe to %LOCALAPPDATA%\Programs\WhatsappDoppel, adds Start menu and
  Desktop shortcuts and an entry in Settings > Apps (so it can be uninstalled from there).
  Safe to run again: it stops a running copy and updates it in place. Your data
  (%APPDATA%\WhatsappDoppel) is never touched.

  Double-click install.cmd, or run:
    powershell -NoProfile -ExecutionPolicy Bypass -File .\install.ps1 [-Launch | -NoLaunch] [-NoDesktopShortcut]

.PARAMETER Source
  The WhatsappDoppel.exe to install (default: the one next to this script).
#>
[CmdletBinding()]
param(
  [string]$Source,
  [switch]$Launch,
  [switch]$NoLaunch,
  [switch]$NoDesktopShortcut,
  [switch]$Pause
)
Set-StrictMode -Version 2.0
$ErrorActionPreference = 'Stop'

$here = Split-Path -Parent $MyInvocation.MyCommand.Path
$appName = 'WhatsApp Doppel'
$dest = Join-Path $env:LOCALAPPDATA 'Programs\WhatsappDoppel'
$exe = Join-Path $dest 'WhatsappDoppel.exe'
$uninstallKey = 'HKCU:\Software\Microsoft\Windows\CurrentVersion\Uninstall\WhatsappDoppel'

function Step([string]$msg) { Write-Host "==> $msg" -ForegroundColor Cyan }
function Test-Interactive {
  try { return ($Host.Name -eq 'ConsoleHost') -and -not [Console]::IsInputRedirected } catch { return $false }
}
function Stop-Doppel {
  if (-not (Test-Path -LiteralPath $exe)) { return }
  Step 'Stopping the running copy (if any)'
  try {
    Start-Process -FilePath $exe -ArgumentList 'quit' -WindowStyle Hidden -Wait -ErrorAction Stop
  } catch { }
  # Wait for every process started from the installed exe to end.
  for ($i = 0; $i -lt 50; $i++) {
    $running = @(Get-Process -Name 'WhatsappDoppel' -ErrorAction SilentlyContinue |
      Where-Object { $_.Path -and ($_.Path -eq $exe) })
    if ($running.Count -eq 0) { return }
    Start-Sleep -Milliseconds 200
  }
  Write-Warning 'WhatsApp Doppel did not stop by itself; ending it.'
  $running | Stop-Process -Force -ErrorAction SilentlyContinue
  Start-Sleep -Milliseconds 500
}

try {
  if (-not $Source) { $Source = Join-Path $here 'WhatsappDoppel.exe' }
  if (-not (Test-Path -LiteralPath $Source)) {
    throw "WhatsappDoppel.exe was not found next to install.ps1 ($Source). Extract the whole zip first (right-click it > Extract All), then run install.cmd from the extracted folder."
  }
  $Source = (Resolve-Path -LiteralPath $Source).Path

  Stop-Doppel

  Step "Installing to $dest"
  New-Item -ItemType Directory -Force -Path $dest | Out-Null
  if ($Source -ne $exe) {
    $copied = $false
    for ($i = 0; $i -lt 20 -and -not $copied; $i++) {
      try { Copy-Item -LiteralPath $Source -Destination $exe -Force; $copied = $true }
      catch { Start-Sleep -Milliseconds 250 }
    }
    if (-not $copied) { Copy-Item -LiteralPath $Source -Destination $exe -Force }
  }
  # Downloaded files carry a "from the internet" mark; clear it on the installed copy.
  Unblock-File -LiteralPath $exe -ErrorAction SilentlyContinue
  foreach ($f in 'uninstall.ps1', 'uninstall.cmd') {
    $src = Join-Path $here $f
    if ((Test-Path -LiteralPath $src) -and ((Split-Path -Parent $src) -ne $dest)) {
      Copy-Item -LiteralPath $src -Destination (Join-Path $dest $f) -Force
      Unblock-File -LiteralPath (Join-Path $dest $f) -ErrorAction SilentlyContinue
    }
  }

  Step 'Adding Start menu and Desktop shortcuts'
  $shell = New-Object -ComObject WScript.Shell
  $links = @(Join-Path ([Environment]::GetFolderPath('Programs')) "$appName.lnk")
  if (-not $NoDesktopShortcut) { $links += Join-Path ([Environment]::GetFolderPath('Desktop')) "$appName.lnk" }
  foreach ($link in $links) {
    $s = $shell.CreateShortcut($link)
    $s.TargetPath = $exe
    $s.WorkingDirectory = $dest
    $s.IconLocation = "$exe,0"
    $s.Description = 'AI personas that reply in your WhatsApp chats'
    $s.Save()
  }

  Step 'Registering in Settings > Apps'
  $version = ''
  try {
    $old = $ErrorActionPreference; $ErrorActionPreference = 'Continue'
    $version = ((& $exe version 2>$null) | Out-String).Trim() -replace '^whatsapp-doppel\s+', ''
  } catch { } finally { $ErrorActionPreference = $old }
  $uninstallCmd = 'powershell.exe -NoProfile -ExecutionPolicy Bypass -File "' + (Join-Path $dest 'uninstall.ps1') + '"'
  New-Item -Path $uninstallKey -Force | Out-Null
  $props = @{
    DisplayName     = $appName
    DisplayIcon     = "$exe,0"
    DisplayVersion  = $version
    Publisher       = 'WhatsApp Doppel'
    InstallLocation = $dest
    UninstallString = $uninstallCmd
    QuietUninstallString = "$uninstallCmd -KeepData"
  }
  foreach ($k in $props.Keys) { New-ItemProperty -Path $uninstallKey -Name $k -Value $props[$k] -PropertyType String -Force | Out-Null }
  foreach ($k in 'NoModify', 'NoRepair') { New-ItemProperty -Path $uninstallKey -Name $k -Value 1 -PropertyType DWord -Force | Out-Null }
  $kb = [int][Math]::Ceiling((Get-Item -LiteralPath $exe).Length / 1KB)
  New-ItemProperty -Path $uninstallKey -Name 'EstimatedSize' -Value $kb -PropertyType DWord -Force | Out-Null

  Write-Host ''
  Write-Host "Installed. Open $appName from the Start menu or your Desktop." -ForegroundColor Green

  $start = $false
  if ($Launch) { $start = $true }
  elseif (-not $NoLaunch -and (Test-Interactive)) {
    $answer = Read-Host "Open $appName now? [Y/n]"
    $start = -not ($answer -match '^[Nn]')
  }
  if ($start) { Start-Process -FilePath $exe -WorkingDirectory $dest }
  $code = 0
} catch {
  Write-Host ''
  Write-Host "Installation failed: $($_.Exception.Message)" -ForegroundColor Red
  $code = 1
}
if ($Pause -and (Test-Interactive)) { Read-Host 'Press Enter to close' | Out-Null }
exit $code
