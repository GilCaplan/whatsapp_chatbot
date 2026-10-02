// Package mission holds the ready-made goals ("missions") a user can give a
// persona, and the achievements earned by completing them. A mission is just
// a goal text filled in from a template; pursuing it is the goal engine's job
// (internal/goals, engine/goal.go). Leaf package: imports only model + stdlib.
package mission

import (
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strings"
	"unicode/utf8"

	"whatsappdoppel/internal/model"
)

// Categories group templates in the picker.
const (
	CatWords   = "words"   // wordplay
	CatPlans   = "plans"   // making plans
	CatCurious = "curious" // finding things out
	CatVibes   = "vibes"   // good vibes
	CatShare   = "share"   // getting them to share something
)

// CategoryLabels are the picker headings, in display order.
var CategoryLabels = []struct{ ID, Label string }{
	{CatWords, "Wordplay"},
	{CatVibes, "Good vibes"},
	{CatCurious, "Curious"},
	{CatShare, "Show and tell"},
	{CatPlans, "Make plans"},
}

// Detectors (MissionTemplate.Detect) — how a mission can be reached besides
// the planner's evidence check.
const (
	DetectSayWord = "say_word"    // goals.ParseSayWord (exact word check)
	DetectImage   = "media:image" // they sent a photo
	DetectAudio   = "media:audio" // they sent a voice note
)

// HowMedia is GoalStatus.How when a media mission was reached by the detector.
const HowMedia = "media"

// Blank limits.
const (
	MaxBlank = 60
	MaxGoal  = 500
)

var name = model.Blank{Key: "name", Label: "Who", Placeholder: "e.g. Dana"}

var templates = []model.MissionTemplate{
	{ID: "say-word", Category: CatWords, Difficulty: 2, Badge: "word", Detect: DetectSayWord,
		Title: "Get {name} to say “{word}”", Blurb: "The classic. The persona steers the chat until the word comes up — without ever asking for it.",
		Goal:   `Get {name} to say the word "{word}"`,
		Blanks: []model.Blank{name, {Key: "word", Label: "The word", Placeholder: "e.g. pineapple"}}},
	{ID: "make-laugh", Category: CatVibes, Difficulty: 1, Badge: "laugh",
		Title: "Make {name} laugh out loud", Blurb: "A real haha, lol or a string of laughing replies counts.",
		Goal:   "Make {name} laugh out loud (a real haha, lol or laughing reply counts)",
		Blanks: []model.Blank{name}},
	{ID: "compliment-back", Category: CatVibes, Difficulty: 2, Badge: "heart",
		Title: "Get a compliment from {name}", Blurb: "Be good company until they say something genuinely nice.",
		Goal:   "Get {name} to pay you a genuine compliment",
		Blanks: []model.Blank{name}},
	{ID: "cheer-up", Category: CatVibes, Difficulty: 2, Badge: "sun",
		Title: "Cheer {name} up", Blurb: "Leave them in a noticeably better mood than when the chat started.",
		Goal:   "Cheer {name} up and leave them in a noticeably better mood than when the chat started",
		Blanks: []model.Blank{name}},
	{ID: "nickname", Category: CatWords, Difficulty: 2, Badge: "tag",
		Title: "Give {name} a nickname that sticks", Blurb: "Invent a playful nickname and get them to accept it.",
		Goal:   "Give {name} a playful nickname and get them to accept it (they use it themselves or reply warmly to it)",
		Blanks: []model.Blank{name}},
	{ID: "inside-joke", Category: CatWords, Difficulty: 3, Badge: "spark",
		Title: "Start an inside joke with {name}", Blurb: "Something you both bring up again later in the chat.",
		Goal:   "Start an inside joke with {name}: something funny you both bring up again later in the chat",
		Blanks: []model.Blank{name}},
	{ID: "find-out", Category: CatCurious, Difficulty: 2, Badge: "magnifier",
		Title: "Find out {thing} without asking", Blurb: "Detective mode: get the answer naturally, never with a direct question.",
		Goal:   "Find out {thing} from {name} without asking about it directly",
		Blanks: []model.Blank{name, {Key: "thing", Label: "What to find out", Placeholder: "e.g. their favourite food"}}},
	{ID: "their-day", Category: CatCurious, Difficulty: 1, Badge: "chat",
		Title: "Get {name} to tell you about their day", Blurb: "Not just fine — what they did and how it felt.",
		Goal:   "Get {name} to tell you about their day in some detail: what they did and how it felt",
		Blanks: []model.Blank{name}},
	{ID: "recommendation", Category: CatCurious, Difficulty: 1, Badge: "star",
		Title: "Get {name} to recommend {thing}", Blurb: "A show, a book, a place to eat — and a follow-up question about it.",
		Goal:   "Get {name} to recommend you {thing}",
		Blanks: []model.Blank{name, {Key: "thing", Label: "Something to recommend", Placeholder: "e.g. a show to watch"}}},
	{ID: "get-photo", Category: CatShare, Difficulty: 2, Badge: "camera", Detect: DetectImage,
		Title: "Get {name} to send you a photo", Blurb: "Their lunch, their pet, the view — any photo counts.",
		Goal:   "Get {name} to send you a photo (of anything: their food, their pet, where they are)",
		Blanks: []model.Blank{name}},
	{ID: "voice-note", Category: CatShare, Difficulty: 3, Badge: "mic", Detect: DetectAudio,
		Title: "Get {name} to send a voice note", Blurb: "Make them want to say it out loud.",
		Goal:   "Get {name} to send you a voice message",
		Blanks: []model.Blank{name}},
	{ID: "share-song", Category: CatShare, Difficulty: 1, Badge: "music",
		Title: "Get {name} to share a song", Blurb: "A song they love, or one that fits the moment.",
		Goal:   "Get {name} to share a song they love with you",
		Blanks: []model.Blank{name}},
	{ID: "plan-meetup", Category: CatPlans, Difficulty: 3, Badge: "calendar",
		Title: "Make real plans to meet {name}", Blurb: "Not a vague some time — a day and a place you both agree on.",
		Goal:   "Make concrete plans to meet {name} this week: agree on a day and a place",
		Blanks: []model.Blank{name}},
	{ID: "movie-night", Category: CatPlans, Difficulty: 2, Badge: "film",
		Title: "Plan a movie night with {name}", Blurb: "Pick a film together and agree when to watch it.",
		Goal:   "Plan a movie night with {name}: pick a film together and agree on when to watch it",
		Blanks: []model.Blank{name}},
	{ID: "agree-to", Category: CatPlans, Difficulty: 2, Badge: "handshake",
		Title: "Get {name} to agree to {plan}", Blurb: "Talk them into something fun, the friendly way.",
		Goal:   "Get {name} to agree to {plan}",
		Blanks: []model.Blank{name, {Key: "plan", Label: "The idea", Placeholder: "e.g. trying sushi on Friday"}}},
}

// Templates returns every mission template (deep copy), in picker order.
func Templates() []model.MissionTemplate {
	out := make([]model.MissionTemplate, len(templates))
	for i, t := range templates {
		t.Blanks = slices.Clone(t.Blanks)
		out[i] = t
	}
	return out
}

// Template returns a template by id.
func Template(id string) (model.MissionTemplate, bool) {
	for _, t := range templates {
		if t.ID == id {
			t.Blanks = slices.Clone(t.Blanks)
			return t, true
		}
	}
	return model.MissionTemplate{}, false
}

// Detector returns a template's detector ("" for none or an unknown id).
func Detector(id string) string {
	t, _ := Template(id)
	return t.Detect
}

// MediaDetected reports whether an incoming message of the given media kind
// ("image", "audio", …) reaches a mission template.
func MediaDetected(templateID, media string) bool {
	d := Detector(templateID)
	return media != "" && strings.HasPrefix(d, "media:") && d == "media:"+media
}

// ErrBlank is returned by Fill for a missing or too long blank.
var ErrBlank = errors.New("mission: blank")

var placeholderRe = regexp.MustCompile(`\{([a-z]+)\}`)

// Fill renders a template's goal with the blanks (trimmed, whitespace
// collapsed, quotes stripped). Every blank is required.
func Fill(t model.MissionTemplate, blanks map[string]string) (string, error) {
	vals := map[string]string{}
	for _, b := range t.Blanks {
		v := strings.Join(strings.Fields(blanks[b.Key]), " ")
		v = strings.Trim(v, `"'“”‘’`)
		if v == "" {
			return "", fmt.Errorf("%w: %s is missing", ErrBlank, strings.ToLower(b.Label))
		}
		if utf8.RuneCountInString(v) > MaxBlank {
			return "", fmt.Errorf("%w: %s is longer than %d characters", ErrBlank, strings.ToLower(b.Label), MaxBlank)
		}
		vals[b.Key] = v
	}
	return placeholderRe.ReplaceAllStringFunc(t.Goal, func(m string) string {
		if v, ok := vals[m[1:len(m)-1]]; ok {
			return v
		}
		return m
	}), nil
}

// Title renders a template's title with the blanks (empty blanks keep their
// placeholder's label, e.g. "Get Dana to say “…”").
func Title(t model.MissionTemplate, blanks map[string]string) string {
	return placeholderRe.ReplaceAllStringFunc(t.Title, func(m string) string {
		if v := strings.TrimSpace(blanks[m[1:len(m)-1]]); v != "" {
			return v
		}
		return "…"
	})
}
