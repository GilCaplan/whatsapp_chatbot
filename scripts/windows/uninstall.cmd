@echo off
rem Double-click to remove WhatsApp Doppel (asks before deleting your data).
setlocal
powershell.exe -NoProfile -ExecutionPolicy Bypass -File "%~dp0uninstall.ps1" -Pause %*
exit /b %ERRORLEVEL%
