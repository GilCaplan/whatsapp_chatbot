// Package persona holds the built-in personas and persona validation.
package persona

import (
	"errors"
	"slices"
	"strings"

	"whatsappdoppel/internal/model"
	"whatsappdoppel/internal/world"
)

// Field limits (runes).
const (
	MaxName       = 60
	MaxTagline    = 140
	MaxField      = 4000
	MaxAdvanced   = 20000
	MaxShortField = 500
	MaxFavorites  = 8
)

var (
	EmojiUsages    = []string{"none", "rare", "some", "lots"}
	MessageLengths = []string{"short", "medium", "long"}
	LLMProviders   = []string{"ollama", "anthropic", "openai"}
)

// DefaultGoal is used when neither the chat nor the persona sets one.
const DefaultGoal = "Catch up and see how their week is going."

// DefaultFallbackReply is sent when the model keeps breaking character.
const DefaultFallbackReply = "lol what? anyway…"

var ErrNameRequired = errors.New("persona name is required")

// Normalize trims and clamps fields, fills defaults (message length "short",
// emoji usage "rare", generated avatar, goal, fallback reply) and validates.
func Normalize(p *model.Persona) error {
	p.Name = clamp(p.Name, MaxName)
	if p.Name == "" {
		return ErrNameRequired
	}
	p.Tagline = clamp(p.Tagline, MaxTagline)
	for _, f := range []*string{&p.Bio, &p.Personality, &p.Style, &p.Vocabulary, &p.Rules, &p.Language} {
		*f = clamp(*f, MaxField)
	}
	p.Goal = clamp(p.Goal, MaxShortField)
	p.DecisionHint = clamp(p.DecisionHint, MaxShortField)
	p.FallbackReply = clamp(p.FallbackReply, MaxShortField)
	p.AdvancedPrompt = clamp(p.AdvancedPrompt, MaxAdvanced)

	p.MessageLength = strings.ToLower(strings.TrimSpace(p.MessageLength))
	if !slices.Contains(MessageLengths, p.MessageLength) {
		p.MessageLength = "short"
	}
	p.Emoji.Usage = strings.ToLower(strings.TrimSpace(p.Emoji.Usage))
	if !slices.Contains(EmojiUsages, p.Emoji.Usage) {
		p.Emoji.Usage = "rare"
	}
	favs := make([]string, 0, len(p.Emoji.Favorites))
	for _, f := range p.Emoji.Favorites {
		if f = strings.TrimSpace(f); f != "" && !slices.Contains(favs, f) && len(favs) < MaxFavorites {
			favs = append(favs, clamp(f, 16))
		}
	}
	p.Emoji.Favorites = favs

	if p.Goal == "" {
		p.Goal = DefaultGoal
	}
	normalizeGoal(p)
	if p.FallbackReply == "" {
		p.FallbackReply = DefaultFallbackReply
	}
	normalizeAvatar(p)
	// Where the persona lives (wave 3): a bad zone or time is an error so the
	// editor can say so; everything else is cleaned up.
	if err := world.Validate(p.World); err != nil {
		return err
	}
	world.Normalize(&p.World)

	if p.LLM != nil {
		p.LLM.Provider = strings.ToLower(strings.TrimSpace(p.LLM.Provider))
		p.LLM.Model = strings.TrimSpace(p.LLM.Model)
		if p.LLM.Provider == "" || !slices.Contains(LLMProviders, p.LLM.Provider) {
			p.LLM = nil
		}
	}
	return nil
}

// normalizeGoal fills the goal-pursuit defaults: subtle style, plan ahead on,
// relax once the goal is reached.
func normalizeGoal(p *model.Persona) {
	p.GoalStyle = strings.ToLower(strings.TrimSpace(p.GoalStyle))
	if !slices.Contains(model.GoalStyles, p.GoalStyle) {
		p.GoalStyle = model.GoalStyleSubtle
	}
	p.GoalAfterReached = strings.ToLower(strings.TrimSpace(p.GoalAfterReached))
	if !slices.Contains(model.GoalAfterOptions, p.GoalAfterReached) {
		p.GoalAfterReached = model.GoalAfterRelax
	}
	if p.GoalPlanAhead == nil {
		on := true
		p.GoalPlanAhead = &on
	}
}

func normalizeAvatar(p *model.Persona) {
	a := &p.Avatar
	if a.Kind == "upload" {
		if a.Initials == "" {
			a.Initials = Initials(p.Name)
		}
		return
	}
	gen := GeneratedAvatar(p.Name)
	fresh := a.Kind == "" // no avatar chosen yet; "generated" with no glyph means initials
	a.Kind = "generated"
	if len(a.Gradient) != 2 || a.Gradient[0] == "" || a.Gradient[1] == "" {
		a.Gradient = gen.Gradient
	}
	// An unknown glyph (e.g. an old emoji avatar) falls back to the generated one.
	if (fresh && a.Glyph == "") || (a.Glyph != "" && !IsGlyph(a.Glyph)) {
		a.Glyph = gen.Glyph
	}
	a.Initials = clamp(a.Initials, 2)
	if a.Initials == "" {
		a.Initials = gen.Initials
	}
}

func clamp(s string, n int) string {
	s = strings.TrimSpace(s)
	r := []rune(s)
	if len(r) > n {
		return strings.TrimSpace(string(r[:n]))
	}
	return s
}
