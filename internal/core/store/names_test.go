package store

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	"whatsappdoppel/internal/core/config"
)

func TestKnownNames(t *testing.T) {
	p, err := config.NewPaths(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	write := func(path, body string) {
		t.Helper()
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	write(p.ChatsFile(), `[{"kind":"group","name":"Chess Circle","people":{"people":[{"name":"Toma Ferro"}]}},{"kind":"dm","name":"Zuri"}]`)
	write(p.RecentChatsFile(), `[{"kind":"dm","name":"Wren🌸"},{"kind":"group","name":"Tide Pool"}]`)
	write(filepath.Join(p.HistoryDir(), "dm_1.jsonl"), `{"name":"תמרה גולן"}`+"\n"+`not json`+"\n"+`{"name":"the boss 300"}`+"\n"+`{"text":"no name"}`+"\n")

	got := KnownNames(p)
	for _, want := range []string{"Toma Ferro", "Toma", "Ferro", "Zuri", "Wren", "תמרה", "גולן", "Chess Circle", "Tide Pool", "the boss 300"} {
		if !slices.Contains(got, want) {
			t.Errorf("missing %q in %q", want, got)
		}
	}
	for _, not := range []string{"Chess", "Circle", "Tide", "the", "boss", "300"} {
		if slices.Contains(got, not) {
			t.Errorf("%q shouldn't be a name", not)
		}
	}
}

func TestKnownNamesWithNoData(t *testing.T) {
	p, err := config.NewPaths(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if got := KnownNames(p); len(got) != 0 {
		t.Fatalf("names = %q", got)
	}
}
