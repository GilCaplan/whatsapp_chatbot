package wa

import (
	"os"
	"path/filepath"
	"testing"
)

func read(t *testing.T, p string) string {
	t.Helper()
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestBackupSessionKeepsOneCopyPerVersion(t *testing.T) {
	db := filepath.Join(t.TempDir(), "whatsapp.db")
	write := func(s string) {
		if err := os.WriteFile(db, []byte(s), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	write("v1 session")
	if err := backupSession(db, "1.0"); err != nil {
		t.Fatal(err)
	}
	if got := read(t, db+".bak"); got != "v1 session" {
		t.Fatalf("backup = %q", got)
	}
	write("newer session")
	if err := backupSession(db, "1.0"); err != nil {
		t.Fatal(err)
	}
	if got := read(t, db+".bak"); got != "v1 session" {
		t.Fatalf("same version replaced the backup: %q", got)
	}
	if err := backupSession(db, "1.1"); err != nil {
		t.Fatal(err)
	}
	if got := read(t, db+".bak"); got != "newer session" {
		t.Fatalf("new version kept the old backup: %q", got)
	}
	st, err := os.Stat(db + ".bak")
	if err != nil {
		t.Fatal(err)
	}
	if os.PathSeparator == '/' && st.Mode().Perm() != 0o600 {
		t.Fatalf("backup mode: %v", st.Mode())
	}
	entries, _ := os.ReadDir(filepath.Dir(db))
	if len(entries) != 3 { // whatsapp.db, .bak, .bak.version
		t.Fatalf("%d files in the folder", len(entries))
	}
	if err := removeSessionBackup(db); err != nil {
		t.Fatal(err)
	}
	if entries, _ = os.ReadDir(filepath.Dir(db)); len(entries) != 1 {
		t.Fatalf("log out left %d files", len(entries))
	}
}

func TestBackupSessionSurvivesGoingBack(t *testing.T) {
	db := filepath.Join(t.TempDir(), "whatsapp.db")
	if err := os.WriteFile(db, []byte("before the migration"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := backupSession(db, "1.0"); err != nil {
		t.Fatal(err)
	}
	if err := backupSession(db, "1.1"); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(db, []byte("migrated"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := backupSession(db, "1.0"); err != nil {
		t.Fatal(err)
	}
	if got := read(t, db+".bak"); got != "before the migration" {
		t.Fatalf("going back replaced the backup: %q", got)
	}
}

func TestRemoveSessionBackupReportsWhatItCouldNotRemove(t *testing.T) {
	db := filepath.Join(t.TempDir(), "whatsapp.db")
	if err := removeSessionBackup(db); err != nil {
		t.Fatalf("nothing to remove: %v", err)
	}
	// A folder with something in it can't be removed like a file.
	if err := os.MkdirAll(filepath.Join(db+".bak", "stuck"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := removeSessionBackup(db); err == nil {
		t.Fatal("a backup that stayed was not reported")
	}
}

func TestBackupSessionSkips(t *testing.T) {
	dir := t.TempDir()
	db := filepath.Join(dir, "whatsapp.db")
	if err := backupSession(db, "1.0"); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(db, []byte("half written"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, sidecar := range []string{"-journal", "-wal"} {
		if err := os.WriteFile(db+sidecar, []byte("j"), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := backupSession(db, "1.0"); err != nil {
			t.Fatal(err)
		}
		if _, err := os.Stat(db + ".bak"); err == nil {
			t.Fatal("backed up a session that wasn't whole")
		}
		if err := os.Remove(db + sidecar); err != nil {
			t.Fatal(err)
		}
	}
}

func TestUnfinishedCopyIsRemoved(t *testing.T) {
	db := filepath.Join(t.TempDir(), "whatsapp.db")
	for _, p := range []string{db, db + ".bak.import-123"} {
		if err := os.WriteFile(p, []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if err := backupSession(db, "1.0"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(db + ".bak.import-123"); err == nil {
		t.Fatal("a copy left by a crash is still there")
	}
	if err := os.WriteFile(db+".bak.import-456", []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := removeSessionBackup(db); err != nil {
		t.Fatal(err)
	}
	if entries, _ := os.ReadDir(filepath.Dir(db)); len(entries) != 1 {
		t.Fatalf("log out left %d files", len(entries))
	}
}
