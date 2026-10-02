@echo off
rem Double-click to install WhatsApp Doppel for this Windows user (no admin needed).
setlocal
if not exist "%~dp0install.ps1" (
  echo install.ps1 is missing. Extract the whole zip first: right-click it, Extract All,
  echo then double-click install.cmd in the extracted folder.
  pause
  exit /b 1
)
powershell.exe -NoProfile -ExecutionPolicy Bypass -File "%~dp0install.ps1" -Pause %*
exit /b %ERRORLEVEL%
