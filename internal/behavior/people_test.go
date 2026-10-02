package behavior

import (
	"slices"
	"strings"
	"testing"

	"whatsappdoppel/internal/model"
)

func bp(b bool) *bool { return &b }

func TestAnswerableResolution(t *testing.T) {
	josh := "972501234567@s.whatsapp.net"
	joshLID := "200000000000001@lid"
	cases := []struct {
		name    string
		pc      model.PeopleConfig
		members int
		ids     []string
		want    bool
		source  string
	}{
		{"auto small group answers everyone", model.PeopleConfig{}, 6, []string{josh}, true, SourcePeopleBy},
		{"auto at the threshold", model.PeopleConfig{Mode: "auto"}, 8, []string{josh}, true, SourcePeopleBy},
		{"auto big group answers only picked", model.PeopleConfig{}, 24, []string{josh}, false, SourcePeopleBy},
		{"unknown size counts as small", model.PeopleConfig{}, 0, []string{josh}, true, SourcePeopleBy},
		{"everyone mode", model.PeopleConfig{Mode: "everyone"}, 200, []string{josh}, true, SourcePeopleBy},
		{"selected mode", model.PeopleConfig{Mode: "selected"}, 3, []string{josh}, false, SourcePeopleBy},
		{"picked in a big group", model.PeopleConfig{People: []model.PersonPrefs{{JID: josh, Respond: bp(true)}}}, 24, []string{josh}, true, SourcePerson},
		{"muted in a small group", model.PeopleConfig{People: []model.PersonPrefs{{JID: josh, Respond: bp(false)}}}, 4, []string{josh}, false, SourcePerson},
		{"matched by lid", model.PeopleConfig{People: []model.PersonPrefs{{JID: joshLID, Respond: bp(true)}}}, 24, []string{josh, joshLID + ":3"}, true, SourcePerson},
		{"notes only follow the mode", model.PeopleConfig{People: []model.PersonPrefs{{JID: josh, Notes: "brother"}}}, 24, []string{josh}, false, SourcePeopleBy},
		{"priority is always answered", model.PeopleConfig{People: []model.PersonPrefs{{JID: josh, Priority: true}}}, 24, []string{josh}, true, SourcePerson},
	}
	for _, c := range cases {
		got, src := Answerable(c.pc, c.members, 8, c.ids...)
		if got != c.want || src != c.source {
			t.Errorf("%s: got %v/%s, want %v/%s", c.name, got, src, c.want, c.source)
		}
	}
	if EffectivePeopleMode(model.PeopleConfig{}, 3, 0) != model.PeopleSelected {
		t.Error("threshold 0 = never default to everyone")
	}
}

func TestValidateAndNormalizePeople(t *testing.T) {
	ok := model.PeopleConfig{Mode: "selected", People: []model.PersonPrefs{{JID: "1@s.whatsapp.net", Notes: "hi"}}}
	if err := ValidatePeople(ok); err != nil {
		t.Fatal(err)
	}
	for name, pc := range map[string]model.PeopleConfig{
		"mode":  {Mode: "some"},
		"jid":   {People: []model.PersonPrefs{{JID: "josh"}}},
		"dup":   {People: []model.PersonPrefs{{JID: "1@s.whatsapp.net"}, {JID: "1:2@s.whatsapp.net"}}},
		"notes": {People: []model.PersonPrefs{{JID: "1@lid", Notes: strings.Repeat("x", 301)}}},
	} {
		if ValidatePeople(pc) == nil {
			t.Errorf("%s: want an error", name)
		}
	}
	n := NormalizePeople(model.PeopleConfig{Mode: "auto", People: []model.PersonPrefs{
		{JID: " 1@lid ", Name: " Dana "}, {JID: "2@lid", Priority: true}, {JID: "3@lid", Notes: "  x "},
	}})
	if n.Mode != "" || len(n.People) != 2 || n.People[0].JID != "2@lid" || n.People[1].Notes != "x" {
		t.Errorf("normalize = %+v", n)
	}
}

func TestMatchWord(t *testing.T) {
	words := []string{"pizza", "game night", "C++"}
	for text, want := range map[string]string{
		"Who wants PIZZA?":          "pizza",
		"pizzas are great":          "",
		"is game  night on":         "",
		"Game night tomorrow!":      "game night",
		"learning c++ today":        "C++",
		"":                          "",
		"no match here, just pizz.": "",
	} {
		if got := MatchWord(text, words); got != want {
			t.Errorf("MatchWord(%q) = %q, want %q", text, got, want)
		}
	}
	if MatchWord("שלום חברים", []string{"חברים"}) != "חברים" {
		t.Error("unicode words should match")
	}
}

func TestNewBehaviourFieldsAndFill(t *testing.T) {
	np, ng := Default(KindDM), Default(KindGroup)
	if !np.AllowMentions || np.MentionMax != 1 || np.TagReplyPercent != 0 || np.MaxStreak != 0 || np.RespondToAllMaxMembers != 8 || !np.AnswerAnyoneWhoAddressesIt {
		t.Errorf("natural private: %+v", np)
	}
	if ng.TagReplyPercent != 15 || ng.MaxStreak != 6 || ng.TriggerWords == nil || len(ng.MuteWords) != 0 {
		t.Errorf("natural group: %+v", ng)
	}
	in, _ := Preset(PresetInstant)
	if in.Group.TagReplyPercent != 0 {
		t.Error("instant group should not tag replies")
	}

	// A v2 Instant profile lacking the new fields gets Instant's values, keeps its label.
	p := in.For(KindGroup)
	p.TagReplyPercent, p.MentionMax, p.MaxStreak, p.TriggerWords = 99, 0, 0, nil
	FillFromPreset(&p, KindGroup, "tagReplyPercent", "mentionMax", "maxStreak", "triggerWords")
	Normalize(&p, KindGroup)
	if p.Preset != PresetInstant || p.TagReplyPercent != 0 || p.MentionMax != 1 || p.MaxStreak != 6 {
		t.Errorf("fill: %+v", p)
	}

	// Word lists are cleaned, nil and empty compare equal.
	q := Default(KindGroup)
	q.TriggerWords = []string{" Pizza ", "pizza", "", "game   night"}
	Clamp(&q)
	if !slices.Equal(q.TriggerWords, []string{"Pizza", "game night"}) {
		t.Errorf("words = %q", q.TriggerWords)
	}
	r := Default(KindGroup)
	r.MuteWords = nil
	if !Equal(r, Default(KindGroup)) {
		t.Error("nil and empty word lists should be equal")
	}

	nine, many := 9, make([]string, 21)
	for i := range many {
		many[i] = string(rune('a' + i))
	}
	if ValidateOverrides(model.BehaviorOverrides{MentionMax: &nine}) == nil {
		t.Error("mentionMax 9 should be rejected")
	}
	if ValidateOverrides(model.BehaviorOverrides{MuteWords: &many}) == nil {
		t.Error("21 mute words should be rejected")
	}
	long := []string{strings.Repeat("x", 41)}
	if ValidateOverrides(model.BehaviorOverrides{TriggerWords: &long}) == nil {
		t.Error("a 41-character trigger word should be rejected")
	}

	// A chat that applied the whole Instant preset keeps matching it.
	full := model.BehaviorOverrides{Preset: PresetInstant, ReplyPercent: &in.Group.ReplyPercent, QuoteReplyPercent: &in.Group.QuoteReplyPercent}
	if !FillOverridesFromPreset(&full, KindGroup, "tagReplyPercent", "triggerWords") || full.TagReplyPercent == nil || *full.TagReplyPercent != 0 || full.TriggerWords == nil {
		t.Errorf("override fill: %+v", full)
	}
	if FillOverridesFromPreset(&full, KindGroup, "tagReplyPercent") {
		t.Error("second fill should change nothing")
	}
	partial := model.BehaviorOverrides{Preset: PresetInstant}
	if FillOverridesFromPreset(&partial, KindGroup, "tagReplyPercent") {
		t.Error("a label without values is not a full preset")
	}
}
