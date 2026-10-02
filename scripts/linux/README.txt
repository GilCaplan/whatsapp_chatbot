WhatsApp Doppel for Linux
=========================

Install (no sudo needed):
  1. Open a terminal in this folder (most file managers: right-click > "Open in Terminal").
  2. Run:  ./install.sh
     (Or right-click install.sh > "Run as a Program", where your file manager offers it.)
  3. Open "WhatsApp Doppel" from your app menu or your Desktop. Your browser opens
     the app; scan the QR code with WhatsApp on your phone
     (Settings > Linked devices > Link a device).

What goes where:
  ~/.local/bin/whatsapp-doppel     the app (a single program)
  ~/.config/WhatsappDoppel/        your data: settings, personas, WhatsApp link, logs
  App menu entry + icon in ~/.local/share, a shortcut on your Desktop.

Good to know:
  - Notifications need notify-send (Ubuntu/Debian: sudo apt install libnotify-bin).
  - For free, private replies install Ollama (curl -fsSL https://ollama.com/install.sh | sh)
    and run: ollama pull llama3.1:8b
  - In a terminal: whatsapp-doppel status | quit | version
  - On Windows with WSL, use the Windows download instead.

Uninstall:  ~/.local/share/whatsapp-doppel/uninstall.sh   (or ./uninstall.sh from this folder)
            It asks before deleting your data; --keep-data / --purge skip the question.
