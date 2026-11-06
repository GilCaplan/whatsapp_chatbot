package store

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"

	"whatsappdoppel/internal/core/config"
)

// minNameWord is the shortest word of a person's name worth looking for.
const minNameWord = 3

// KnownNames lists the names in this computer's chats, for the privacy check:
// each person's full name and each word of it, and each group's name as a
// whole, since a group's single words are ordinary words. Missing or unreadable
// files just mean fewer names.
func KnownNames(p config.Paths) []string {
	seen := map[string]bool{}
	person := func(name string) {
		name = strings.TrimSpace(name)
		if utf8.RuneCountInString(name) >= minNameWord {
			seen[name] = true
		}
		for _, w := range strings.Fields(name) {
			w = strings.TrimFunc(w, func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) })
			if first, _ := utf8.DecodeRuneInString(w); utf8.RuneCountInString(w) >= minNameWord && unicode.IsLetter(first) && !unicode.IsLower(first) {
				seen[w] = true
			}
		}
	}
	named := func(kind, name string) {
		if kind != "group" {
			person(name)
		} else if name = strings.TrimSpace(name); name != "" {
			seen[name] = true
		}
	}

	var chats []struct {
		Kind   string `json:"kind"`
		Name   string `json:"name"`
		People struct {
			People []struct {
				Name string `json:"name"`
			} `json:"people"`
		} `json:"people"`
	}
	if b, err := os.ReadFile(p.ChatsFile()); err == nil && json.Unmarshal(b, &chats) == nil {
		for _, c := range chats {
			named(c.Kind, c.Name)
			for _, m := range c.People.People {
				person(m.Name)
			}
		}
	}
	var recent []struct {
		Kind string `json:"kind"`
		Name string `json:"name"`
	}
	if b, err := os.ReadFile(p.RecentChatsFile()); err == nil && json.Unmarshal(b, &recent) == nil {
		for _, c := range recent {
			named(c.Kind, c.Name)
		}
	}
	files, _ := filepath.Glob(filepath.Join(p.HistoryDir(), "*.jsonl"))
	for _, f := range files {
		historyNames(f, person)
	}

	out := make([]string, 0, len(seen))
	for n := range seen {
		out = append(out, n)
	}
	slices.Sort(out)
	return out
}

func historyNames(path string, person func(string)) {
	f, err := os.Open(path)
	if err != nil {
		return
	}
	defer f.Close()
	lines := bufio.NewScanner(f)
	lines.Buffer(make([]byte, 0, 64<<10), 4<<20)
	for lines.Scan() {
		var m struct {
			Name string `json:"name"`
		}
		if json.Unmarshal(lines.Bytes(), &m) == nil && m.Name != "" {
			person(m.Name)
		}
	}
}
