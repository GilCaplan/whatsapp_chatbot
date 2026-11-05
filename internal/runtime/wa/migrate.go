package wa

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

// migrateLegacy copies (never moves) the first existing legacy session DB to
// dst when dst does not exist yet. It returns the source path when a copy was
// made. A non-nil error means a candidate was found but deliberately not imported
// (e.g. the old bot still has it open); the caller surfaces it as lastError.
func migrateLegacy(dst string, candidates []string) (imported string, err error) {
	if _, err := os.Stat(dst); err == nil {
		return "", nil
	} else if !errors.Is(err, fs.ErrNotExist) {
		return "", fmt.Errorf("check %s: %w", dst, err)
	}
	for _, src := range candidates {
		if src == "" {
			continue
		}
		abs, err := filepath.Abs(src)
		if err != nil {
			continue
		}
		st, err := os.Stat(abs)
		if err != nil || !st.Mode().IsRegular() || st.Size() == 0 {
			continue
		}
		if same, _ := sameFile(abs, dst); same {
			return "", nil
		}
		for _, suffix := range []string{"-wal", "-journal"} {
			if _, err := os.Stat(abs + suffix); err == nil {
				return "", fmt.Errorf("didn't import your existing WhatsApp session from %s because %s exists — the old bot may still be running. Stop it, then restart the app",
					filepath.Base(abs), filepath.Base(abs+suffix))
			}
		}
		if err := copyFile(abs, dst); err != nil {
			return "", fmt.Errorf("couldn't import WhatsApp session from %s: %w", filepath.Base(abs), err)
		}
		return abs, nil
	}
	return "", nil
}

// backupSession keeps one copy of the session beside it, replaced the first
// time a version of the app runs: a whatsmeow upgrade migrates whatsapp.db one
// way, and the copy holds the same login, so there must never be more than this
// one. The mark lists every version seen, so going back to an earlier one keeps
// the copy from before the migration instead of replacing it with the migrated file.
func backupSession(dbPath, version string) error {
	removeUnfinishedCopies(dbPath)
	mark := dbPath + ".bak.version"
	got, _ := os.ReadFile(mark)
	seen := strings.Fields(string(got))
	if slices.Contains(seen, version) {
		return nil
	}
	if st, err := os.Stat(dbPath); err != nil || st.Size() == 0 {
		return nil // nothing linked yet
	}
	for _, suffix := range []string{"-wal", "-journal"} {
		if _, err := os.Stat(dbPath + suffix); err == nil {
			return nil // a crash left it mid-write: the copy wouldn't be whole
		}
	}
	if err := copyFile(dbPath, dbPath+".bak"); err != nil {
		return fmt.Errorf("back up WhatsApp session: %w", err)
	}
	return os.WriteFile(mark, []byte(strings.Join(append(seen, version), "\n")), 0o600)
}

// removeSessionBackup forgets the copy: after a log out, nothing should keep
// the login that was just unlinked.
func removeSessionBackup(dbPath string) error {
	removeUnfinishedCopies(dbPath)
	var errs []error
	for _, p := range []string{dbPath + ".bak", dbPath + ".bak.version"} {
		if err := os.Remove(p); err != nil && !errors.Is(err, fs.ErrNotExist) {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

// removeUnfinishedCopies clears what a crash during copyFile leaves behind,
// which is a login too.
func removeUnfinishedCopies(dbPath string) {
	dir, prefix := filepath.Dir(dbPath), filepath.Base(dbPath)+".bak.import-"
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), prefix) {
			_ = os.Remove(filepath.Join(dir, e.Name()))
		}
	}
}

func sameFile(a, b string) (bool, error) {
	sa, err := os.Stat(a)
	if err != nil {
		return false, err
	}
	sb, err := os.Stat(b)
	if err != nil {
		return false, err
	}
	return os.SameFile(sa, sb), nil
}

// copyFile copies src to dst atomically (temp file + rename) with mode 0600.
func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	if err := os.MkdirAll(filepath.Dir(dst), 0o700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(dst), filepath.Base(dst)+".import-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name()) // no-op after a successful rename
	if _, err := io.Copy(tmp, in); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Chmod(0o600); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), dst)
}
