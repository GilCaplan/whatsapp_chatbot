package behavior

import (
	"fmt"
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"

	"whatsappdoppel/internal/model"
)

// Limits of a chat's people config.
const (
	MaxPeople      = 2048
	MaxNotesRunes  = 300
	MaxPersonName  = 64
	SourcePerson   = "person" // respondSource: set for this person
	SourcePeopleBy = "mode"   // respondSource: follows the chat's mode
)

// PeopleModes are the valid PeopleConfig.Mode values ("" = auto).
var PeopleModes = []string{model.PeopleAuto, model.PeopleEveryone, model.PeopleSelected}

// EffectivePeopleMode resolves a group's mode to "everyone" or "selected".
// Automatic answers everyone in groups with at most threshold members
// (threshold 0 = never) and only picked people in bigger ones; an unknown
// size (memberCount <= 0) counts as small so nothing goes quiet by surprise.
func EffectivePeopleMode(pc model.PeopleConfig, memberCount, threshold int) string {
	switch pc.Mode {
	case model.PeopleEveryone, model.PeopleSelected:
		return pc.Mode
	}
	if memberCount <= 0 || (threshold > 0 && memberCount <= threshold) {
		return model.PeopleEveryone
	}
	return model.PeopleSelected
}

// UserOf returns the user part of a JID ("972501234567:12@s.whatsapp.net" → "972501234567").
func UserOf(jid string) string {
	u, _, _ := strings.Cut(jid, "@")
	u, _, _ = strings.Cut(u, ":")
	return strings.TrimSpace(u)
}

// FindPerson returns the index of the prefs whose JID's user part equals the
// user part of any of ids (phone or lid forms of the same person), or -1.
func FindPerson(people []model.PersonPrefs, ids ...string) int {
	for _, id := range ids {
		u := UserOf(id)
		if u == "" {
			continue
		}
		if i := slices.IndexFunc(people, func(p model.PersonPrefs) bool { return UserOf(p.JID) == u }); i >= 0 {
			return i
		}
	}
	return -1
}

// Answerable reports whether the persona answers a group member, and why:
// "person" (set for them) or "mode" (the chat's effective mode). A priority
// ("always reply") person is always answerable.
func Answerable(pc model.PeopleConfig, memberCount, threshold int, ids ...string) (bool, string) {
	if i := FindPerson(pc.People, ids...); i >= 0 {
		switch p := pc.People[i]; {
		case p.Priority:
			return true, SourcePerson
		case p.Respond != nil:
			return *p.Respond, SourcePerson
		}
	}
	return EffectivePeopleMode(pc, memberCount, threshold) == model.PeopleEveryone, SourcePeopleBy
}

// ValidatePeople checks a people config (mode, JIDs, lengths).
func ValidatePeople(pc model.PeopleConfig) error {
	if pc.Mode != "" && !slices.Contains(PeopleModes, pc.Mode) {
		return fmt.Errorf("people.mode must be one of %s", strings.Join(PeopleModes, ", "))
	}
	if len(pc.People) > MaxPeople {
		return fmt.Errorf("people can list at most %d people", MaxPeople)
	}
	seen := map[string]bool{}
	for _, p := range pc.People {
		u := UserOf(p.JID)
		if u == "" || !strings.Contains(p.JID, "@") {
			return fmt.Errorf("people: %q is not a WhatsApp address", p.JID)
		}
		if seen[u] {
			return fmt.Errorf("people lists %s twice", p.JID)
		}
		seen[u] = true
		if utf8.RuneCountInString(p.Notes) > MaxNotesRunes {
			return fmt.Errorf("people: notes for %s are longer than %d characters", personLabel(p), MaxNotesRunes)
		}
		if utf8.RuneCountInString(p.Name) > MaxPersonName {
			return fmt.Errorf("people: name %q is longer than %d characters", p.Name, MaxPersonName)
		}
	}
	return nil
}

func personLabel(p model.PersonPrefs) string {
	if p.Name != "" {
		return p.Name
	}
	return p.JID
}

// NormalizePeople trims text, drops entries that carry no preference and
// stores "" for the automatic mode.
func NormalizePeople(pc model.PeopleConfig) model.PeopleConfig {
	if pc.Mode == model.PeopleAuto {
		pc.Mode = ""
	}
	out := make([]model.PersonPrefs, 0, len(pc.People))
	for _, p := range pc.People {
		p.JID = strings.TrimSpace(p.JID)
		p.Name = strings.TrimSpace(p.Name)
		p.Notes = strings.TrimSpace(p.Notes)
		if p.Respond == nil && !p.Priority && p.Notes == "" {
			continue
		}
		out = append(out, p)
	}
	pc.People = out
	return pc
}

// MatchWord returns the first of words that text contains as a whole word or
// phrase (case-insensitive; the characters around the match must not be
// letters or digits), or "".
func MatchWord(text string, words []string) string {
	lower := strings.ToLower(text)
	for _, w := range words {
		lw := strings.ToLower(strings.TrimSpace(w))
		if lw == "" {
			continue
		}
		for from := 0; from < len(lower); {
			i := strings.Index(lower[from:], lw)
			if i < 0 {
				break
			}
			i += from
			end := i + len(lw)
			before, _ := utf8.DecodeLastRuneInString(lower[:i])
			after, _ := utf8.DecodeRuneInString(lower[end:])
			if (i == 0 || !wordRune(before)) && (end == len(lower) || !wordRune(after)) {
				return w
			}
			_, size := utf8.DecodeRuneInString(lower[i:])
			from = i + size
		}
	}
	return ""
}

func wordRune(r rune) bool { return unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' }
