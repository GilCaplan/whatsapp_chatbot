package behavior

import (
	"encoding/json"
	"slices"
	"testing"

	"whatsappdoppel/internal/model"
)

func TestDialTableShape(t *testing.T) {
	owner := map[string]string{}
	for _, d := range Dials() {
		if len(d.Levels) != 5 {
			t.Fatalf("%s has %d levels", d.ID, len(d.Levels))
		}
		for i, l := range d.Levels {
			if l.Level != i+1 || l.Label == "" || l.Blurb == "" {
				t.Errorf("%s level %d = %+v", d.ID, i+1, l)
			}
			// Every level of a dial sets the same fields, each in range.
			for _, kind := range []string{KindDM, KindGroup} {
				if !slices.Equal(sortedKeys(l.For(kind)), sortedKeys(d.Levels[0].For(kind))) {
					t.Errorf("%s level %d %s sets different fields", d.ID, l.Level, kind)
				}
				p := Default(kind)
				ApplyDial(&p, kind, d.ID, l.Level)
				if err := Validate(p); err != nil {
					t.Errorf("%s level %d %s: %v", d.ID, l.Level, kind, err)
				}
			}
		}
		for _, f := range d.Fields {
			if o, dup := owner[f]; dup {
				t.Errorf("%s is owned by %s and %s", f, o, d.ID)
			}
			owner[f] = d.ID
			if testGroupOnly[f] {
				if _, ok := d.Levels[0].Private[f]; ok {
					t.Errorf("%s: group-only field %s in the private table", d.ID, f)
				}
			}
		}
	}
	// The served table is plain JSON.
	if _, err := json.Marshal(DialTable()); err != nil {
		t.Fatal(err)
	}
	if len(DialTable()) != 3 || DialTable()[DialSpeed].Label != "Speed" {
		t.Errorf("table = %v", DialTable())
	}
}

// testGroupOnly mirrors the "group" comments in model.BehaviorProfile.
var testGroupOnly = map[string]bool{"chimeInPercent": true, "aiJudgement": true, "quoteReplyPercent": true,
	"tagReplyPercent": true, "mentionMax": true, "allowMentions": true}

func sortedKeys(m map[string]any) []string {
	var out []string
	for k := range m {
		out = append(out, k)
	}
	slices.Sort(out)
	return out
}

func TestDialTableMonotone(t *testing.T) {
	for _, d := range Dials() {
		for _, kind := range []string{KindDM, KindGroup} {
			for _, f := range d.Fields {
				if !dialMonotone(d, kind, f) {
					var seq []any
					for _, l := range d.Levels {
						seq = append(seq, l.For(kind)[f])
					}
					t.Errorf("%s/%s: %s is not monotone: %v", d.ID, kind, f, seq)
				}
			}
		}
	}
}

func TestPresetsLandOnDials(t *testing.T) {
	want := map[string][3]int{ // speed, chattiness, boldness
		PresetNatural:  {3, 3, 3},
		PresetInstant:  {5, 3, 1},
		PresetBusy:     {2, 1, 3},
		PresetSlow:     {1, 2, 3},
		PresetNightOwl: {4, 3, 3},
	}
	for _, pd := range Presets() {
		w, ok := want[pd.ID]
		if !ok {
			t.Fatalf("no expectation for preset %s", pd.ID)
		}
		for _, kind := range []string{KindDM, KindGroup} {
			got := ReadDials(pd.For(kind), kind)
			for i, id := range DialIDs {
				pos := got[id]
				if !pos.Exact || pos.Level != w[i] || !slices.Contains(pos.Matches, w[i]) {
					t.Errorf("%s/%s %s = %+v, want exact %d", pd.ID, kind, id, pos, w[i])
				}
			}
		}
	}
}

func TestApplyDialRoundTrip(t *testing.T) {
	for _, kind := range []string{KindDM, KindGroup} {
		for _, id := range DialIDs {
			for lvl := 1; lvl <= 5; lvl++ {
				p := Default(kind)
				if !ApplyDial(&p, kind, id, lvl) {
					t.Fatal("apply failed")
				}
				pos := ReadDials(p, kind)[id]
				if !pos.Exact || !slices.Contains(pos.Matches, lvl) {
					t.Errorf("%s/%s level %d reads %+v", kind, id, lvl, pos)
				}
				// Other dials keep their (Natural) level.
				for _, other := range DialIDs {
					if other != id && ReadDials(p, kind)[other].Level != 3 {
						t.Errorf("%s/%s level %d moved %s", kind, id, lvl, other)
					}
				}
			}
		}
	}
	p := Default(KindDM)
	if ApplyDial(&p, KindDM, "warmth", 2) || ApplyDial(&p, KindDM, DialSpeed, 6) {
		t.Error("unknown dial/level applied")
	}
	// Private chattiness 3–5 coincide: Natural wins the tie, all are listed.
	if pos := ReadDials(Default(KindDM), KindDM)[DialChattiness]; pos.Level != 3 || !slices.Equal(pos.Matches, []int{3, 4, 5}) {
		t.Errorf("private chattiness = %+v", pos)
	}
}

func TestReadDialsNearest(t *testing.T) {
	// Natural with a slightly slower typing speed: still nearest to Natural, not exact.
	p := Default(KindDM)
	p.TypingCharsPerSec = 6
	if pos := ReadDials(p, KindDM)[DialSpeed]; pos.Level != 3 || pos.Exact || len(pos.Matches) != 0 {
		t.Errorf("tuned natural = %+v", pos)
	}
	// Instant timings with a 1 s notice: nearest is Instant.
	p = Default(KindDM)
	ApplyDial(&p, KindDM, DialSpeed, 5)
	p.NoticeMaxSec = 1
	if pos := ReadDials(p, KindDM)[DialSpeed]; pos.Level != 5 || pos.Exact {
		t.Errorf("tuned instant = %+v", pos)
	}
	// Very slow notice with everything else Slow-texter-ish.
	p = Default(KindGroup)
	ApplyDial(&p, KindGroup, DialSpeed, 1)
	p.NoticeMaxSec = 3600
	if pos := ReadDials(p, KindGroup)[DialSpeed]; pos.Level != 1 || pos.Exact {
		t.Errorf("slower than slow = %+v", pos)
	}
	// Group chime-in 90% with AI judgement on: between Talkative and everything.
	p = Default(KindGroup)
	p.ChimeInPercent = 90
	if pos := ReadDials(p, KindGroup)[DialChattiness]; pos.Exact || pos.Level < 4 {
		t.Errorf("chatty group = %+v", pos)
	}
	// "No limit" is the far end: a cap of 500/day is nearer unlimited levels than Reserved (40).
	p = Default(KindDM)
	p.MaxRepliesPerDay = 500
	if pos := ReadDials(p, KindDM)[DialChattiness]; pos.Level != 3 || pos.Exact {
		t.Errorf("high cap = %+v", pos)
	}
	// Typos alone move boldness.
	p = Default(KindDM)
	p.TypoPercent = 12
	p.ReactPercent = 10
	if pos := ReadDials(p, KindDM)[DialBoldness]; pos.Level != 5 || !pos.Exact {
		t.Errorf("daring = %+v", pos)
	}
}

func TestLegacyPresetRelabel(t *testing.T) {
	for _, id := range []string{PresetBusy, PresetSlow} {
		for _, kind := range []string{KindDM, KindGroup} {
			old, ok := LegacyPreset(id, kind)
			if !ok {
				t.Fatal(id)
			}
			if MatchPreset(old, kind) == id {
				t.Fatalf("%s/%s: legacy values still match the preset", id, kind)
			}
			p := old
			p.Preset = id
			if !RelabelLegacyPreset(&p, kind) || p.Preset != id || MatchPreset(p, kind) != id {
				t.Errorf("%s/%s not relabelled: %s", id, kind, MatchPreset(p, kind))
			}
			// A customised old preset stays as it is.
			c := old
			c.Preset = id
			c.SplitPercent = 99
			if RelabelLegacyPreset(&c, kind) || c.SplitPercent != 99 {
				t.Errorf("%s/%s customised profile relabelled", id, kind)
			}
		}
	}
	n := Default(KindDM)
	if RelabelLegacyPreset(&n, KindDM) {
		t.Error("natural relabelled")
	}

	// Chat overrides that applied the whole old Slow preset.
	old, _ := LegacyPreset(PresetSlow, KindGroup)
	o := overridesOf(old)
	o.Preset = PresetSlow
	if !RelabelLegacyOverrides(&o, KindGroup) || *o.ReactPercent != 15 || *o.DistractedPercent != 35 {
		t.Errorf("overrides = %v %v", *o.ReactPercent, *o.DistractedPercent)
	}
	if RelabelLegacyOverrides(&o, KindGroup) {
		t.Error("relabelled twice")
	}
	var partial model.BehaviorOverrides
	partial.Preset = PresetBusy
	if RelabelLegacyOverrides(&partial, KindDM) {
		t.Error("partial overrides relabelled")
	}
}

// overridesOf turns a whole profile into overrides (as applying a preset does).
func overridesOf(p model.BehaviorProfile) model.BehaviorOverrides {
	b, _ := json.Marshal(p)
	var o model.BehaviorOverrides
	_ = json.Unmarshal(b, &o)
	return o
}
