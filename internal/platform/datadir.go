package platform

import (
	"os"
	"path/filepath"
)

// DefaultDataDir is the per-user data folder:
//
//	macOS:   ~/Library/Application Support/WhatsappDoppel
//	Linux:   $XDG_CONFIG_HOME/WhatsappDoppel (default ~/.config/WhatsappDoppel)
//	Windows: %APPDATA%\WhatsappDoppel
//
// It does not create the folder and ignores DOPPEL_DATA_DIR (see config).
func DefaultDataDir() (string, error) {
	base, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(base, AppDirName), nil
}
