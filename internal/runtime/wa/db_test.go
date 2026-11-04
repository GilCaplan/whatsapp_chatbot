package wa

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"testing"

	"go.mau.fi/whatsmeow/store/sqlstore"
)

// TestSessionDSNOpens opens a fresh session store through the pure-Go driver
// in a folder whose name needs escaping, and checks the per-connection pragmas
// whatsmeow depends on.
func TestSessionDSNOpens(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "odd %name #1", "App Support")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "whatsapp.db")
	ctx := context.Background()

	c, err := sqlstore.New(ctx, sqlDriver, sessionDSN(path), nil)
	if err != nil {
		t.Fatalf("sqlstore.New: %v", err)
	}
	dev, err := c.GetFirstDevice(ctx)
	if err != nil || dev == nil {
		t.Fatalf("GetFirstDevice: %v, %v", dev, err)
	}
	if err := c.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("database not created at the literal path: %v", err)
	}
	for _, side := range []string{"-wal", "-shm"} {
		if _, err := os.Stat(path + side); err == nil {
			t.Errorf("unexpected %s sidecar: journal mode must stay the default", side)
		}
	}

	db, err := sql.Open(sqlDriver, sessionDSN(path))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var fk, busy int
	if err := db.QueryRow("PRAGMA foreign_keys").Scan(&fk); err != nil || fk != 1 {
		t.Errorf("foreign_keys = %d, %v; want 1", fk, err)
	}
	if err := db.QueryRow("PRAGMA busy_timeout").Scan(&busy); err != nil || busy != 5000 {
		t.Errorf("busy_timeout = %d, %v; want 5000", busy, err)
	}
	var version int
	if err := db.QueryRow("SELECT version FROM whatsmeow_version").Scan(&version); err != nil || version == 0 {
		t.Errorf("whatsmeow_version = %d, %v", version, err)
	}
}
