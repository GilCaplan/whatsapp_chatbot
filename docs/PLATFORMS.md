# Platform notes

<!-- Moved word for word from CLAUDE.md; CLAUDE.md points here. -->

macOS app bundle and Windows build details.

- The .app is `LSUIElement` (no Dock icon): the launcher exits after opening the browser and
  the server keeps running detached; failures surface as a native alert (`platform.Alert`).
- Windows release exe is GUI subsystem (`-H=windowsgui`, no console on double-click);
  `platform.AttachParentConsole` lets `status`/`quit`/`version` print in cmd/PowerShell (PowerShell
  only waits when the output is piped: `& exe status | Out-Host`). Dev builds (`go build`) are console exes.
- Windows icon/version resources: `rsrc_windows_*.syso` generated at dist/install time by
  `go run github.com/tc-hib/go-winres@v0.3.3` (not in go.mod), deleted after the build, gitignored.
- `.gitattributes` forces LF (CRLF for `.cmd`/`.ps1`); keep `.ps1` files ASCII (Windows PowerShell 5.1
  reads BOM-less files as ANSI). Never put `:` or other Windows-invalid characters in file names.
