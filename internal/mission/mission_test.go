package mission

import (
	"errors"
	"strings"
	"testing"
	"time"

	"whatsappdoppel/internal/goals"
	"whatsappdoppel/internal/model"
)

func TestTemplatesWellFormed(t *testing.T) {
	ts := Templates()
	if len(ts) < 12 {
		t.Fatalf("only %d templates", len(ts))
	}
	ids, cats := map[string]bool{}, map[string]bool{}
	for _, c := range CategoryLabels {
		cats[c.ID] = true
	}
	for _, tp := range ts {
		if ids[tp.ID] {
			t.Errorf("duplicate id %s", tp.ID)
		}
		ids[tp.ID] = true
		if tp.Title == "" || tp.Blurb == "" || tp.Goal == "" || tp.Badge == "" || !cats[tp.Category] || tp.Difficulty < 1 || tp.Difficulty > 3 {
			t.Errorf("template %+v", tp)
		}
		// Every placeholder in the title/goal has a blank, and vice versa.
		keys := map[string]bool{}
		for _, b := range tp.Blanks {
			keys[b.Key] = true
			if b.Label == "" || b.Placeholder == "" {
				t.Errorf("%s blank %+v", tp.ID, b)
			}
		}
		for _, s := range []string{tp.Title, tp.Goal} {
			for _, m := range placeholderRe.FindAllStringSubmatch(s, -1) {
				if !keys[m[1]] {
					t.Errorf("%s: {%s} has no blank", tp.ID, m[1])
				}
			}
		}
		if !strings.Contains(tp.Goal, "{name}") {
			t.Errorf("%s goal does not name the person", tp.ID)
		}
		blanks := map[string]string{}
		for _, b := range tp.Blanks {
			blanks[b.Key] = "x"
		}
		g, err := Fill(tp, blanks)
		if err != nil || strings.ContainsAny(g, "{}") {
			t.Errorf("%s fill = %q %v", tp.ID, g, err)
		}
	}
	// Deep copy.
	ts[0].Blanks[0].Key = "zzz"
	if tp, _ := Template(ts[0].ID); tp.Blanks[0].Key == "zzz" {
		t.Error("Templates shares blanks")
	}
}

func TestFill(t *testing.T) {
	tp, _ := Template("say-word")
	g, err := Fill(tp, map[string]string{"name": "  Josh ", "word": `"pineapple"`})
	if err != nil || g != `Get Josh to say the word "pineapple"` {
		t.Fatalf("fill = %q %v", g, err)
	}
	// The goal engine recognises it as a say-the-word goal (exact detection).
	sw, ok := goals.ParseSayWord(g)
	if !ok || sw.Word != "pineapple" || sw.Who != "Josh" {
		t.Errorf("ParseSayWord = %+v %v", sw, ok)
	}
	if _, err := Fill(tp, map[string]string{"name": "Josh"}); !errors.Is(err, ErrBlank) || !strings.Contains(err.Error(), "the word") {
		t.Errorf("missing = %v", err)
	}
	if _, err := Fill(tp, map[string]string{"name": strings.Repeat("a", MaxBlank+1), "word": "x"}); !errors.Is(err, ErrBlank) {
		t.Errorf("too long = %v", err)
	}
	if got := Title(tp, map[string]string{"name": "Dana"}); got != "Get Dana to say “…”" {
		t.Errorf("title = %q", got)
	}
	if Detector("get-photo") != DetectImage || Detector("say-word") != DetectSayWord || Detector("nope") != "" {
		t.Error("detectors")
	}
	if !MediaDetected("get-photo", "image") || MediaDetected("get-photo", "audio") || !MediaDetected("voice-note", "audio") ||
		MediaDetected("say-word", "image") || MediaDetected("get-photo", "") {
		t.Error("MediaDetected")
	}
}

func rec(chat, tpl string, at time.Time, style, how string) model.MissionRecord {
	t := at
	return model.MissionRecord{ChatKey: chat, TemplateID: tpl, Style: style, How: how, StartedAt: at.Add(-time.Hour), ReachedAt: &t}
}

func byID(as []model.Achievement) map[string]model.Achievement {
	m := map[string]model.Achievement{}
	for _, a := range as {
		m[a.ID] = a
	}
	return m
}

func TestAchievements(t *testing.T) {
	loc := time.UTC
	none := byID(Achievements(nil, loc))
	if len(none) != len(AchievementIDs()) {
		t.Fatalf("got %d achievements", len(none))
	}
	for _, a := range none {
		if a.Unlocked || a.Progress != 0 || a.Target < 1 || a.Title == "" || a.Badge == "" {
			t.Errorf("empty history: %+v", a)
		}
	}

	day := time.Date(2026, 10, 1, 14, 0, 0, 0, loc)
	open := model.MissionRecord{ChatKey: "dm:9", TemplateID: "say-word", StartedAt: day} // not reached: never counts
	records := []model.MissionRecord{
		open,
		rec("dm:1", "say-word", day, model.GoalStyleBalanced, model.GoalHowSaidWord),
		rec("dm:1", "get-photo", day.Add(24*time.Hour), model.GoalStyleDirect, HowMedia),
		rec("group:2", "plan-meetup", day.Add(10*24*time.Hour+13*time.Hour), model.GoalStyleSubtle, model.GoalHowAI), // 03:00
	}
	a := byID(Achievements(records, loc))
	if f := a["first-mission"]; !f.Unlocked || !f.UnlockedAt.Equal(day) || f.Progress != 1 {
		t.Errorf("first = %+v", f)
	}
	if h := a["three-missions"]; !h.Unlocked || !h.UnlockedAt.Equal(*records[3].ReachedAt) || h.Progress != 3 {
		t.Errorf("three = %+v", h)
	}
	if w := a["word-smith"]; w.Unlocked || w.Progress != 1 || w.Target != 3 {
		t.Errorf("word = %+v", w)
	}
	if s := a["social-butterfly"]; s.Unlocked || s.Progress != 2 {
		t.Errorf("butterfly = %+v", s)
	}
	if !a["smooth-operator"].Unlocked || !a["night-shift"].Unlocked || !a["photo-finish"].Unlocked || !a["planner"].Unlocked {
		t.Errorf("singles = %+v", a)
	}
	// Two within 7 days, the third 9 days later: no streak yet.
	if s := a["streak-week"]; s.Unlocked || s.Progress != 2 {
		t.Errorf("streak = %+v", s)
	}
	records = append(records,
		rec("dm:3", "say-word", day.Add(11*24*time.Hour), "", model.GoalHowSaidWord),
		rec("dm:3", "say-word", day.Add(12*24*time.Hour), "", model.GoalHowSaidWord))
	a = byID(Achievements(records, loc))
	if s := a["streak-week"]; !s.Unlocked || s.Progress != 3 || !s.UnlockedAt.Equal(day.Add(12*24*time.Hour)) {
		t.Errorf("streak = %+v", s)
	}
	if w := a["word-smith"]; !w.Unlocked || w.Progress != 3 {
		t.Errorf("word = %+v", w)
	}
	if b := a["social-butterfly"]; !b.Unlocked || !b.UnlockedAt.Equal(day.Add(11*24*time.Hour)) {
		t.Errorf("butterfly = %+v", b)
	}
	if m := a["ten-missions"]; m.Unlocked || m.Progress != 5 {
		t.Errorf("ten = %+v", m)
	}
	// Night shift uses the given zone.
	ny, _ := time.LoadLocation("America/New_York")
	if byID(Achievements(records[:4], ny))["night-shift"].Unlocked {
		t.Error("03:00 UTC is 23:00 in New York")
	}
}
