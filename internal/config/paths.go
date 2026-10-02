package config

import (
	"os"
	"path/filepath"
)

// Paths resolves every on-disk location from a single data directory.
// Never use relative paths elsewhere: the .app launches with cwd "/".
type Paths struct {
	Dir string
}

// DefaultDataDir is ~/Library/Application Support/WhatsappDoppel unless
// DOPPEL_DATA_DIR is set.
func DefaultDataDir() string {
	if d := os.Getenv("DOPPEL_DATA_DIR"); d != "" {
		if abs, err := filepath.Abs(d); err == nil {
			return abs
		}
		return d
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(os.TempDir(), "WhatsappDoppel")
	}
	return filepath.Join(home, "Library", "Application Support", "WhatsappDoppel")
}

func NewPaths(dir string) (Paths, error) {
	if dir == "" {
		dir = DefaultDataDir()
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		return Paths{}, err
	}
	p := Paths{Dir: abs}
	for _, d := range []string{p.Dir, p.AvatarsDir(), p.AvatarCacheDir(), p.HistoryDir(), p.LogsDir()} {
		if err := os.MkdirAll(d, 0o700); err != nil {
			return Paths{}, err
		}
	}
	return p, nil
}

func (p Paths) ConfigFile() string     { return filepath.Join(p.Dir, "config.json") }
func (p Paths) SecretsFile() string    { return filepath.Join(p.Dir, "secrets.json") }
func (p Paths) PersonasFile() string   { return filepath.Join(p.Dir, "personas.json") }
func (p Paths) ChatsFile() string      { return filepath.Join(p.Dir, "chats.json") }
func (p Paths) ApprovalsFile() string  { return filepath.Join(p.Dir, "approvals.json") }
func (p Paths) RuntimeFile() string    { return filepath.Join(p.Dir, "runtime.json") }
func (p Paths) WhatsAppDB() string     { return filepath.Join(p.Dir, "whatsapp.db") }
func (p Paths) AvatarsDir() string     { return filepath.Join(p.Dir, "avatars") }
func (p Paths) CacheDir() string       { return filepath.Join(p.Dir, "cache") }
func (p Paths) AvatarCacheDir() string { return filepath.Join(p.Dir, "cache", "avatars") }
func (p Paths) RecentChatsFile() string {
	return filepath.Join(p.Dir, "cache", "recent.json")
}
func (p Paths) HistoryDir() string   { return filepath.Join(p.Dir, "history") }
func (p Paths) LogsDir() string      { return filepath.Join(p.Dir, "logs") }
func (p Paths) ServerLog() string    { return filepath.Join(p.Dir, "logs", "server.log") }
func (p Paths) InstanceFile() string { return filepath.Join(p.Dir, "instance.json") }
func (p Paths) LockFile() string     { return filepath.Join(p.Dir, "server.lock") }

// Wave 3 files (created lazily by their store helpers).
func (p Paths) MemoriesDir() string     { return filepath.Join(p.Dir, "memories") }
func (p Paths) RecapsFile() string      { return filepath.Join(p.Dir, "recaps.json") }
func (p Paths) MissionsFile() string    { return filepath.Join(p.Dir, "missions.json") }
func (p Paths) SelfSamplesFile() string { return filepath.Join(p.Dir, "cache", "self-samples.jsonl") }
