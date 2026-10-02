package persona

import (
	"strings"
	"testing"
	"unicode/utf8"

	"whatsappdoppel/internal/model"
)

func TestSeeds(t *testing.T) {
	s := Seeds()
	want := []string{"leo", "kyle", "luna", "brad", "chad"}
	if len(s) != len(want) {
		t.Fatalf("got %d seeds", len(s))
	}
	for i, id := range want {
		p := s[i]
		if p.ID != id || !p.BuiltIn {
			t.Errorf("seed %d: id=%q builtIn=%v", i, p.ID, p.BuiltIn)
		}
		if p.Name == "" || p.Bio == "" || p.Style == "" || p.Rules == "" || p.Goal == "" ||
			p.DecisionHint == "" || p.FallbackReply == "" || p.Tagline == "" {
			t.Errorf("seed %s has empty fields", id)
		}
		if len(p.Avatar.Gradient) != 2 || p.Avatar.Glyph == "" || p.Avatar.Initials == "" {
			t.Errorf("seed %s avatar incomplete: %+v", id, p.Avatar)
		}
		cp := p
		if err := Normalize(&cp); err != nil {
			t.Errorf("seed %s: %v", id, err)
		}
	}
	leo, _ := Seed("leo")
	if !strings.Contains(leo.Rules, "own it!") {
		t.Error("Leo should use the bot.go variant with \"own it\"")
	}
	if _, ok := Seed("nope"); ok {
		t.Error("unknown seed")
	}
	// Seeds must be independent copies.
	a, _ := Seed("kyle")
	a.Emoji.Favorites[0] = "x"
	b, _ := Seed("kyle")
	if b.Emoji.Favorites[0] == "x" {
		t.Error("Seed returned shared slice")
	}
}

func TestNormalize(t *testing.T) {
	var p model.Persona
	if err := Normalize(&p); err != ErrNameRequired {
		t.Fatalf("want ErrNameRequired, got %v", err)
	}
	p = model.Persona{
		Name:          "  Dana Levi  ",
		MessageLength: "HUGE",
		Emoji:         model.EmojiPrefs{Usage: "", Favorites: []string{"✨", "✨", " ", "🍸"}},
		Bio:           strings.Repeat("ש", MaxField+50),
		LLM:           &model.LLMChoice{Provider: "bogus"},
	}
	if err := Normalize(&p); err != nil {
		t.Fatal(err)
	}
	if p.Name != "Dana Levi" || p.MessageLength != "short" || p.Emoji.Usage != "rare" {
		t.Errorf("defaults: %+v", p)
	}
	if len(p.Emoji.Favorites) != 2 {
		t.Errorf("favorites dedupe: %v", p.Emoji.Favorites)
	}
	if utf8.RuneCountInString(p.Bio) != MaxField || !utf8.ValidString(p.Bio) {
		t.Errorf("bio clamp: %d runes", utf8.RuneCountInString(p.Bio))
	}
	if p.Avatar.Kind != "generated" || len(p.Avatar.Gradient) != 2 || p.Avatar.Initials != "DL" || p.Avatar.Glyph == "" {
		t.Errorf("avatar: %+v", p.Avatar)
	}
	if p.Goal != DefaultGoal || p.FallbackReply != DefaultFallbackReply {
		t.Error("goal/fallback defaults")
	}
	if p.LLM != nil {
		t.Error("invalid provider should clear llm choice")
	}
	if GeneratedAvatar("Dana").Gradient[0] != GeneratedAvatar("dana").Gradient[0] {
		t.Error("generated avatar should be stable")
	}
}
