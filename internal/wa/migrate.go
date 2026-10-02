package wa

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
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
