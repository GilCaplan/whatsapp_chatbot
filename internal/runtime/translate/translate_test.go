package translate

import (
	"archive/zip"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
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
	if _, err := s.Translate(context.Background(), "hello", "ru"); !errors.Is(err, ErrNotReady) {
		t.Fatalf("err = %v", err)
	}
	if st := s.Status(); st.Installed || len(st.Packs) != 0 {
		t.Fatalf("status = %+v", st)
	}
}
