WhatsApp Doppel for Windows
===========================

Install (no administrator rights needed):
  1. Extract this zip: right-click it > Extract All > Extract.
  2. In the extracted folder, double-click install.cmd.
     If Windows says "Windows protected your PC", click "More info", then "Run anyway"
     (the app is free but not code-signed, so Windows doesn't know it yet).
  3. Open "WhatsApp Doppel" from the Start menu or your Desktop. Your browser opens the
     app; scan the QR code with WhatsApp on your phone
     (Settings > Linked devices > Link a device).

What goes where:
  %LOCALAPPDATA%\Programs\WhatsappDoppel\   the app
  %APPDATA%\WhatsappDoppel\                 your data: settings, personas, WhatsApp link, logs
  Shortcuts in the Start menu and on the Desktop; an entry in Settings > Apps.

Good to know:
  - Notifications show up as coming from "Windows PowerShell".
  - For free, private replies install Ollama from https://ollama.com/download
    and run in a terminal: ollama pull llama3.1:8b
  - In a terminal: "%LOCALAPPDATA%\Programs\WhatsappDoppel\WhatsappDoppel.exe" status | more
    (also: quit, version)

Uninstall: Settings > Apps > WhatsApp Doppel > Uninstall, or double-click uninstall.cmd.
Your data is kept unless you choose to delete it.
