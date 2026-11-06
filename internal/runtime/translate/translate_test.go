package translate

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"whatsappdoppel/internal/logic/lang"
)

func writeZip(t *testing.T, files map[string]string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "pack.zip")
	f, err := os.Create(p)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	w := zip.NewWriter(f)
	for name, body := range files {
		e, err := w.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := e.Write([]byte(body)); err != nil {
			t.Fatal(err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestUnpackKeepsWhatItNeeds(t *testing.T) {
	dest := filepath.Join(t.TempDir(), "en_ru")
	err := unpack(writeZip(t, map[string]string{
		"translate-en_ru-1_9/model/model.bin":       "m",
		"translate-en_ru-1_9/sentencepiece.model":   "s",
		"translate-en_ru-1_9/README.md":             "r",
		"translate-en_ru-1_9/stanza/en/tokenize.pt": "x",
	}), dest)
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range []string{"model/model.bin", "sentencepiece.model"} {
		if !exists(filepath.Join(dest, f)) {
			t.Errorf("%s is missing", f)
		}
	}
	for _, f := range []string{"README.md", "stanza"} {
		if exists(filepath.Join(dest, f)) {
			t.Errorf("%s should be left out", f)
		}
	}
}

func TestUnpackRefusesPathsOutsideThePack(t *testing.T) {
	root := t.TempDir()
	err := unpack(writeZip(t, map[string]string{"top/model/../../../evil": "x"}), filepath.Join(root, "a", "b"))
	if err == nil || !strings.Contains(err.Error(), "bad path") {
		t.Fatalf("err = %v", err)
	}
}

func TestNotReadyUntilInstalled(t *testing.T) {
	s := New(t.TempDir())
	if _, err := s.Translate(context.Background(), "hello", "ru", nil); !errors.Is(err, ErrNotReady) {
		t.Fatalf("err = %v", err)
	}
	if st := s.Status(); st.Installed || len(st.Packs) != 0 {
		t.Fatalf("status = %+v", st)
	}
}

func TestAddOnFromOlderPinsIsNotInstalled(t *testing.T) {
	s := New(t.TempDir())
	if err := os.MkdirAll(s.dir, 0o700); err != nil {
		t.Fatal(err)
	}
	ready := filepath.Join(s.dir, "ready")
	for _, old := range []string{"", "an older stamp"} {
		if err := os.WriteFile(ready, []byte(old), 0o600); err != nil {
			t.Fatal(err)
		}
		if s.Status().Installed {
			t.Fatalf("ready file %q counted as installed", old)
		}
		if _, err := s.Translate(context.Background(), "hello", "ru", nil); !errors.Is(err, ErrNotReady) {
			t.Fatalf("err = %v", err)
		}
	}
	if err := os.WriteFile(ready, []byte(addonStamp()), 0o600); err != nil {
		t.Fatal(err)
	}
	if !s.Status().Installed {
		t.Fatal("the current pins didn't count as installed")
	}
}

func TestEveryLanguageHasAPack(t *testing.T) {
	if len(packs) != len(lang.All)-1 {
		t.Errorf("%d packs for %d languages besides English", len(packs), len(lang.All)-1)
	}
	for _, l := range lang.All {
		if l.Code == "en" {
			continue
		}
		p, ok := packs[l.Code]
		if !ok {
			t.Errorf("%s has no pinned pack", l.Name)
			continue
		}
		if !strings.HasPrefix(p.url, "https://") || p.size <= 0 {
			t.Errorf("%s: bad pin %+v", l.Name, p)
		}
		if b, err := hex.DecodeString(p.sha256); err != nil || len(b) != sha256.Size {
			t.Errorf("%s: bad checksum %q", l.Name, p.sha256)
		}
	}
}

func pin(url string, body []byte) pack {
	sum := sha256.Sum256(body)
	return pack{url: url, sha256: hex.EncodeToString(sum[:]), size: int64(len(body))}
}

func TestDownloadChecksThePin(t *testing.T) {
	body := []byte("the pack")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write(body) }))
	defer srv.Close()
	var out bytes.Buffer
	if err := download(context.Background(), pin(srv.URL, body), &out); err != nil || out.String() != "the pack" {
		t.Fatalf("good pack: %q, %v", out.String(), err)
	}
	if err := download(context.Background(), pin(srv.URL, []byte("another pack")), io.Discard); err == nil {
		t.Fatal("a different file passed")
	}
	shorter := pin(srv.URL, body[:3])
	shorter.sha256 = pin(srv.URL, body).sha256
	if err := download(context.Background(), shorter, io.Discard); err == nil {
		t.Fatal("a longer file than pinned passed")
	}
}

func TestBadDownloadLeavesTheOldPackAlone(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		w.Write([]byte("not what was pinned"))
	}))
	defer srv.Close()
	s := New(t.TempDir())
	s.packs = map[string]pack{"ru": pin(srv.URL, []byte("the real pack"))}
	if err := os.MkdirAll(s.packDir("ru"), 0o700); err != nil {
		t.Fatal(err)
	}
	old := filepath.Join(s.packDir("ru"), "sentencepiece.model")
	if err := os.WriteFile(old, []byte("old"), 0o600); err != nil {
		t.Fatal(err)
	}

	s.EnsurePack("ru")
	waitForFetches(t, s)
	if !strings.Contains(s.Status().Error, "checksum") {
		t.Fatalf("error = %q", s.Status().Error)
	}
	if !exists(old) || s.packReady("ru") {
		t.Fatal("a bad download touched the pack or marked it ready")
	}
	s.EnsurePack("ru") // the next reply mustn't download it all again
	waitForFetches(t, s)
	if hits.Load() != 1 {
		t.Fatalf("downloaded %d times", hits.Load())
	}
}

func TestPackFromBeforeThePinsIsDownloadedAgain(t *testing.T) {
	body := zipBytes(t, map[string]string{"translate-en_ru/model/model.bin": "m", "translate-en_ru/sentencepiece.model": "s"})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write(body) }))
	defer srv.Close()
	s := New(t.TempDir())
	s.packs = map[string]pack{"ru": pin(srv.URL, body)}
	if err := os.MkdirAll(s.packDir("ru"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(s.packDir("ru"), "ready"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if got := s.Status().Packs; len(got) != 0 {
		t.Fatalf("old pack counted as ready: %v", got)
	}
	s.EnsurePack("ru")
	waitForFetches(t, s)
	if got := s.Status().Packs; !slices.Equal(got, []string{"ru"}) {
		t.Fatalf("packs = %v, error %q", got, s.Status().Error)
	}
}

func TestALaterRetryClearsThePackError(t *testing.T) {
	body := zipBytes(t, map[string]string{"translate-en_ru/sentencepiece.model": "s"})
	var fail atomic.Bool
	fail.Store(true)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if fail.Load() {
			w.WriteHeader(http.StatusBadGateway)
			return
		}
		w.Write(body)
	}))
	defer srv.Close()
	s := New(t.TempDir())
	if err := os.MkdirAll(s.dir, 0o700); err != nil {
		t.Fatal(err)
	}
	s.packs = map[string]pack{"ru": pin(srv.URL, body)}
	s.EnsurePack("ru")
	waitForFetches(t, s)
	if s.Status().Error == "" {
		t.Fatal("no error after a failed download")
	}
	fail.Store(false)
	s.failedAt["ru"] = time.Now().Add(-2 * packRetry)
	s.EnsurePack("ru")
	waitForFetches(t, s)
	if st := s.Status(); st.Error != "" || !slices.Equal(st.Packs, []string{"ru"}) {
		t.Fatalf("status = %+v", st)
	}
}

func TestDownloadsTakeTurns(t *testing.T) {
	var now, peak atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := now.Add(1)
		defer now.Add(-1)
		for p := peak.Load(); n > p && !peak.CompareAndSwap(p, n); p = peak.Load() {
		}
		time.Sleep(20 * time.Millisecond)
	}))
	defer srv.Close()
	s := New(t.TempDir())
	if err := os.MkdirAll(s.dir, 0o700); err != nil {
		t.Fatal(err)
	}
	s.packs = map[string]pack{}
	for _, c := range []string{"a", "b", "c", "d", "e", "f", "g", "h"} {
		s.packs[c] = pin(srv.URL, []byte(c))
	}
	for c := range s.packs {
		s.EnsurePack(c)
	}
	waitForFetches(t, s)
	if p := peak.Load(); p == 0 || p > packDownloads {
		t.Fatalf("%d downloads at once", p)
	}
}

func TestEnsurePackIgnoresLanguagesWithoutAPack(t *testing.T) {
	s := New(t.TempDir())
	s.EnsurePack("en")
	s.EnsurePack("xx")
	if st := s.Status(); len(st.Fetching) != 0 || st.Error != "" {
		t.Fatalf("status = %+v", st)
	}
}

func zipBytes(t *testing.T, files map[string]string) []byte {
	t.Helper()
	b, err := os.ReadFile(writeZip(t, files))
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func waitForFetches(t *testing.T, s *Service) {
	t.Helper()
	for range 500 {
		if len(s.Status().Fetching) == 0 {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("download never finished")
}

func TestDependenciesArePinned(t *testing.T) {
	deps := pipPackages()
	if len(deps) != 5 {
		t.Fatalf("deps = %v", deps)
	}
	for _, d := range deps {
		if !strings.Contains(d, "==") && !strings.Contains(d, "#sha256=") {
			t.Errorf("%s isn't pinned", d)
		}
	}
}
