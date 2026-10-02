package wa

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"whatsappdoppel/internal/config"
	"whatsappdoppel/internal/events"
	"whatsappdoppel/internal/model"
)

func TestMigrateLegacy(t *testing.T) {
	dir := t.TempDir()
	dst := filepath.Join(dir, "data", "whatsapp.db")
	src := filepath.Join(dir, "bot.db")
	if err := os.WriteFile(src, []byte("session"), 0o644); err != nil {
		t.Fatal(err)
	}

	got, err := migrateLegacy(dst, []string{"", filepath.Join(dir, "missing.db"), src})
	if err != nil || got != src {
		t.Fatalf("migrate = %q, %v", got, err)
	}
	if b, _ := os.ReadFile(dst); string(b) != "session" {
		t.Errorf("dst content = %q", b)
	}
	if st, _ := os.Stat(dst); st.Mode().Perm() != 0o600 {
		t.Errorf("dst mode = %v", st.Mode().Perm())
	}
	if _, err := os.Stat(src); err != nil {
		t.Error("source must be kept (copy, never move)")
	}
	// Existing destination: no-op.
	if got, err := migrateLegacy(dst, []string{src}); got != "" || err != nil {
		t.Errorf("second migrate = %q, %v", got, err)
	}

	// Sidecar present: refuse with a reason.
	dst2 := filepath.Join(dir, "other", "whatsapp.db")
	if err := os.WriteFile(src+"-wal", nil, 0o644); err != nil {
		t.Fatal(err)
	}
	got, err = migrateLegacy(dst2, []string{src})
	if got != "" || err == nil || !strings.Contains(err.Error(), "bot.db-wal") {
		t.Errorf("sidecar: %q, %v", got, err)
	}
	if _, err := os.Stat(dst2); err == nil {
		t.Error("dst must not be created when refusing")
	}
}

// TestNewOfflineManager opens a fresh store in a path with spaces (like
// "Application Support") and exercises everything that needs no network.
func TestNewOfflineManager(t *testing.T) {
	paths, err := config.NewPaths(filepath.Join(t.TempDir(), "App Support", "Doppel"))
	if err != nil {
		t.Fatal(err)
	}
	hub := events.NewHub()
	// A legacy candidate with a -journal sidecar must be refused, and the reason kept.
	legacy := filepath.Join(t.TempDir(), "bot.db")
	os.WriteFile(legacy, []byte("x"), 0o600)
	os.WriteFile(legacy+"-journal", nil, 0o600)

	m, err := New(Options{Paths: paths, Hub: hub, LegacyDBCandidates: []string{legacy}})
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close()
	if _, err := os.Stat(paths.WhatsAppDB()); err != nil {
		t.Fatalf("session db not created: %v", err)
	}
	st := m.Status()
	if st.State != model.WADisconnected || st.Me != nil || !strings.Contains(st.LastError, "old bot") {
		t.Errorf("status = %+v", st)
	}
	if m.OwnJIDs() != nil || m.CurrentQR() != nil {
		t.Error("unlinked manager should have no JIDs/QR")
	}
	ctx := context.Background()
	items, total, err := m.ListChats(ctx, model.ChatQuery{Tab: "recent"})
	if err != nil || total != 0 || len(items) != 0 {
		t.Errorf("recent while unlinked: %v %d %v", items, total, err)
	}
	if _, total, err := m.ListChats(ctx, model.ChatQuery{Tab: "contacts"}); err != nil || total != 0 {
		t.Errorf("contacts while unlinked: %d %v", total, err)
	}
	if _, total, err := m.ListChats(ctx, model.ChatQuery{Tab: "groups"}); err != nil || total != 0 {
		t.Errorf("groups while unlinked: %d %v", total, err)
	}
	it, err := m.ResolveChat(ctx, "+972 50 111 2222")
	if err != nil || it.Key != "dm:972501112222" || it.JID != "972501112222@s.whatsapp.net" || it.Name != "+972501112222" {
		t.Errorf("ResolveChat = %+v, %v", it, err)
	}
	if err := m.Send(ctx, "972501112222", "hi"); err != ErrNotConnected {
		t.Errorf("Send while unlinked = %v", err)
	}
	if data, _, err := m.Avatar(ctx, "972501112222"); data != nil || err != nil {
		t.Errorf("Avatar while unlinked = %v, %v", data, err)
	}
	if err := m.Logout(ctx); err != ErrNotLinked {
		t.Errorf("Logout while unlinked = %v", err)
	}
}

func TestRecentIndexPersistence(t *testing.T) {
	path := filepath.Join(t.TempDir(), "cache", "recent.json")
	r := newRecentIndex(path)
	r.upsert(recentEntry{Key: "dm:1", Kind: "dm", JID: "1@s.whatsapp.net", Name: "Saved", TS: 100, Archived: true}, true)
	r.upsert(recentEntry{Key: "dm:1", Kind: "dm", JID: "1@s.whatsapp.net", TS: 50}, false) // older live msg
	if e, _ := r.get("dm:1"); e.Name != "Saved" || e.TS != 100 || !e.Archived {
		t.Errorf("after older upsert: %+v", e)
	}
	r.upsert(recentEntry{Key: "dm:1", Kind: "dm", Name: "Push", TS: 200}, false) // newer live msg unarchives
	if e, _ := r.get("dm:1"); e.Name != "Push" || e.TS != 200 || e.Archived || e.JID == "" {
		t.Errorf("after newer upsert: %+v", e)
	}
	r.close() // flushes the pending debounced save
	r2 := newRecentIndex(path)
	if e, ok := r2.get("dm:1"); !ok || e.TS != 200 {
		t.Errorf("reloaded: %+v %v", e, ok)
	}
}

func TestAvatarCache(t *testing.T) {
	a := newAvatarCache(t.TempDir())
	if data, fresh, neg := a.lookup("x@s.whatsapp.net"); data != nil || fresh || neg {
		t.Fatal("empty cache should miss")
	}
	a.storeImage("x@s.whatsapp.net", []byte("\xff\xd8\xff\xe0jpeg"))
	data, fresh, neg := a.lookup("x@s.whatsapp.net")
	if data == nil || !fresh || neg || imageContentType(data) != "image/jpeg" {
		t.Errorf("positive: %v %v %v", data, fresh, neg)
	}
	img, _ := a.files("x@s.whatsapp.net")
	old := time.Now().Add(-avatarTTL - time.Minute)
	os.Chtimes(img, old, old)
	if data, fresh, _ := a.lookup("x@s.whatsapp.net"); data == nil || fresh {
		t.Error("expired entry should be stale but returned")
	}
	a.storeNegative("x@s.whatsapp.net")
	if _, _, neg := a.lookup("x@s.whatsapp.net"); !neg {
		t.Error("negative entry expected")
	}
	a.invalidate("x@s.whatsapp.net")
	if data, _, neg := a.lookup("x@s.whatsapp.net"); data != nil || neg {
		t.Error("invalidate should clear both")
	}
}

func TestQRPNG(t *testing.T) {
	png, err := qrPNG("2@abcdefghijklmnopqrstuvwxyz,0123456789,ABCDEFGHIJ,klmnop")
	if err != nil || len(png) < 100 || string(png[1:4]) != "PNG" {
		t.Fatalf("qrPNG: %v len=%d", err, len(png))
	}
}
