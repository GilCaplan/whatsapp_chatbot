<#
.SYNOPSIS
  Removes WhatsApp Doppel (what install.ps1 created).

.DESCRIPTION
  Stops the app, removes %LOCALAPPDATA%\Programs\WhatsappDoppel, the Start menu and Desktop
  shortcuts and the Settings > Apps entry. Your data (%APPDATA%\WhatsappDoppel, or
  $env:DOPPEL_DATA_DIR) is kept unless you pass -Purge or answer "y" when asked.

    powershell -NoProfile -ExecutionPolicy Bypass -File .\uninstall.ps1 [-Purge | -KeepData]
#>
[CmdletBinding()]
param(
  [switch]$Purge,
  [switch]$KeepData,
  [switch]$Pause
)
Set-StrictMode -Version 2.0
$ErrorActionPreference = 'Stop'

$appName = 'WhatsApp Doppel'
$dest = Join-Path $env:LOCALAPPDATA 'Programs\WhatsappDoppel'
$exe = Join-Path $dest 'WhatsappDoppel.exe'
$uninstallKey = 'HKCU:\Software\Microsoft\Windows\CurrentVersion\Uninstall\WhatsappDoppel'
$data = if ($env:DOPPEL_DATA_DIR) { $env:DOPPEL_DATA_DIR } else { Join-Path $env:APPDATA 'WhatsappDoppel' }

function Step([string]$msg) { Write-Host "==> $msg" -ForegroundColor Cyan }
function Test-Interactive {
  try { return ($Host.Name -eq 'ConsoleHost') -and -not [Console]::IsInputRedirected } catch { return $false }
}

$code = 0
try {
  # This script may run from the folder it deletes.
  Set-Location -LiteralPath $env:TEMP

  if (Test-Path -LiteralPath $exe) {
    Step 'Stopping the server'
    try { Start-Process -FilePath $exe -ArgumentList 'quit' -WindowStyle Hidden -Wait -ErrorAction Stop } catch { }
    for ($i = 0; $i -lt 50; $i++) {
      $running = @(Get-Process -Name 'WhatsappDoppel' -ErrorAction SilentlyContinue |
        Where-Object { $_.Path -and ($_.Path -eq $exe) })
      if ($running.Count -eq 0) { break }
      if ($i -eq 49) { $running | Stop-Process -Force -ErrorAction SilentlyContinue; Start-Sleep -Milliseconds 500 }
      Start-Sleep -Milliseconds 200
    }
  }

  foreach ($link in @(
      (Join-Path ([Environment]::GetFolderPath('Programs')) "$appName.lnk"),
      (Join-Path ([Environment]::GetFolderPath('Desktop')) "$appName.lnk"))) {
    if (Test-Path -LiteralPath $link) { Step "Removing $link"; Remove-Item -LiteralPath $link -Force }
  }
  if (Test-Path -LiteralPath $uninstallKey) { Step 'Removing the Settings > Apps entry'; Remove-Item -LiteralPath $uninstallKey -Recurse -Force }
  if (Test-Path -LiteralPath $dest) { Step "Removing $dest"; Remove-Item -LiteralPath $dest -Recurse -Force }

  if (Test-Path -LiteralPath $data) {
    $delete = [bool]$Purge
    if (-not $Purge -and -not $KeepData -and (Test-Interactive)) {
      $answer = Read-Host "Also delete your data in `"$data`" (personas, settings, WhatsApp link)? [y/N]"
      $delete = $answer -match '^[Yy]$'
    }
    if ($delete) { Step "Deleting $data"; Remove-Item -LiteralPath $data -Recurse -Force }
    else { Write-Host "Kept your data in: $data" }
  }
  Write-Host 'Uninstalled.' -ForegroundColor Green
} catch {
  Write-Host "Uninstall failed: $($_.Exception.Message)" -ForegroundColor Red
  $code = 1
}
if ($Pause -and (Test-Interactive)) { Read-Host 'Press Enter to close' | Out-Null }
exit $code
