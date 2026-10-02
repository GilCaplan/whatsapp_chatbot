package behavior

import (
	"math"
	"reflect"
	"slices"

	"whatsappdoppel/internal/model"
)

// Vibe dials: three simple controls (Speed, Chattiness, Boldness), five
// discrete levels each, that stand in front of the full behaviour form. A dial
// is derived, never stored: moving it writes its fields from DialTable, and
// ReadDials inverts a profile back to the nearest level. Every field belongs
// to at most one dial; everything else (split, length, availability,
// proactive, history, safety…) stays in the Advanced form. All presets land
// exactly on a level of every dial (TestPresetsLandOnDials).

// Dial ids.
const (
	DialSpeed      = "speed"
	DialChattiness = "chattiness"
	DialBoldness   = "boldness"
)

// DialIDs lists the dials in display order.
var DialIDs = []string{DialSpeed, DialChattiness, DialBoldness}

// DialLevel is one notch of a dial with the field values it sets per chat kind.
type DialLevel struct {
	Level int    `json:"level"`
	Label string `json:"label"`
	Blurb string `json:"blurb"`
	// PrivateBlurb replaces Blurb in private chats when the level means
	// something different there ("" = same as Blurb).
	PrivateBlurb string         `json:"privateBlurb,omitempty"`
	Private      map[string]any `json:"private"`
	Group        map[string]any `json:"group"`
}

// DialDef is one dial.
type DialDef struct {
	ID     string      `json:"id"`
	Label  string      `json:"label"`
	Blurb  string      `json:"blurb"`
	Fields []string    `json:"fields"` // every field it owns (group superset)
	Levels []DialLevel `json:"levels"` // 1..5
}

// For returns the level's values for a chat kind.
func (l DialLevel) For(kind string) map[string]any {
	if kind == KindGroup {
		return l.Group
	}
	return l.Private
}

// DialPos is where a profile sits on one dial.
type DialPos struct {
	Level int  `json:"level"`
	Exact bool `json:"exact"` // every field of the dial equals the level's value
	// Matches lists every level the profile equals exactly (levels can
	// coincide for a kind, e.g. Chattiness 3–5 in private chats).
	Matches []int `json:"matches"`
}

// unlimitedZero fields treat 0 as "no limit" (= +∞ when comparing levels).
var unlimitedZero = map[string]bool{"maxRepliesPerHour": true, "maxRepliesPerDay": true}

func speedLevel(nMin, nMax, wait, burst, tMin, tMax, dPct, dMin, dMax, cps, jit, tyMin, tyMax int) map[string]any {
	return map[string]any{
		"noticeMinSec": nMin, "noticeMaxSec": nMax, "waitForMoreSec": wait, "burstCapSec": burst,
		"thinkMinSec": tMin, "thinkMaxSec": tMax,
		"distractedPercent": dPct, "distractedMinSec": dMin, "distractedMaxSec": dMax,
		"typingCharsPerSec": cps, "typingJitterPercent": jit, "typingMinSec": tyMin, "typingMaxSec": tyMax,
	}
}

func chatPrivate(reply, perHour, perDay, cooldown int) map[string]any {
	return map[string]any{"replyPercent": reply, "maxRepliesPerHour": perHour, "maxRepliesPerDay": perDay, "cooldownSec": cooldown}
}

func chatGroup(reply, chime int, ai bool, perHour, perDay, cooldown int) map[string]any {
	m := chatPrivate(reply, perHour, perDay, cooldown)
	m["chimeInPercent"], m["aiJudgement"] = chime, ai
	return m
}

func boldPrivate(react, typo int) map[string]any {
	return map[string]any{"reactPercent": react, "typoPercent": typo}
}

func boldGroup(react, quote, tag, mentions, typo int) map[string]any {
	return map[string]any{"reactPercent": react, "quoteReplyPercent": quote, "tagReplyPercent": tag, "mentionMax": mentions, "typoPercent": typo}
}

func buildDials() []DialDef {
	speed := DialDef{ID: DialSpeed, Label: "Speed", Blurb: "How quickly it notices your messages and types back.",
		Levels: []DialLevel{
			{Level: 1, Label: "Slow texter", Blurb: "Checks the phone now and then and types slowly. Replies can take many minutes.",
				Private: speedLevel(60, 900, 20, 120, 10, 60, 35, 300, 1800, 4, 35, 1, 60),
				Group:   speedLevel(120, 1800, 30, 120, 10, 60, 35, 300, 1800, 4, 35, 1, 60)},
			{Level: 2, Label: "Takes a while", Blurb: "Notices after a few minutes and sometimes gets pulled away.",
				Private: speedLevel(20, 240, 15, 60, 5, 30, 30, 120, 900, 6, 25, 1, 30),
				Group:   speedLevel(60, 600, 20, 90, 5, 30, 30, 120, 900, 6, 25, 1, 30)},
			{Level: 3, Label: "Natural", Blurb: "A short pause, then it types like someone with the phone nearby.",
				Private: speedLevel(3, 25, 8, 30, 2, 8, 10, 30, 120, 7, 25, 1, 25),
				Group:   speedLevel(10, 90, 12, 45, 3, 12, 15, 60, 300, 7, 25, 1, 25)},
			{Level: 4, Label: "Quick", Blurb: "Picks up fast but still reads, thinks and types like a person.",
				Private: speedLevel(2, 15, 8, 30, 2, 8, 10, 30, 120, 7, 25, 1, 25),
				Group:   speedLevel(5, 60, 12, 45, 2, 8, 10, 30, 120, 7, 25, 1, 25)},
			{Level: 5, Label: "Instant", Blurb: "Answers within seconds. Handy for testing, less human.",
				Private: speedLevel(0, 0, 2, 10, 0, 1, 0, 0, 0, 20, 10, 0, 5),
				Group:   speedLevel(0, 0, 3, 10, 0, 1, 0, 0, 0, 20, 10, 0, 5)},
		}}
	chat := DialDef{ID: DialChattiness, Label: "Chattiness", Blurb: "How often it answers and joins in.",
		Levels: []DialLevel{
			{Level: 1, Label: "Reserved", Blurb: "Leaves the odd message on read and keeps a cap on how often it replies.",
				Private: chatPrivate(85, 6, 40, 60), Group: chatGroup(100, 20, true, 4, 30, 120)},
			{Level: 2, Label: "Picks its moments", Blurb: "Answers what's meant for it, rarely jumps into group chatter.",
				PrivateBlurb: "Answers every message, with a short breather between replies.",
				Private:      chatPrivate(100, 0, 0, 30), Group: chatGroup(100, 25, true, 0, 0, 30)},
			{Level: 3, Label: "Natural", Blurb: "Answers messages for it and joins group chats when it makes sense.",
				PrivateBlurb: "Answers every message, no limits.",
				Private:      chatPrivate(100, 0, 0, 0), Group: chatGroup(100, 40, true, 0, 0, 0)},
			{Level: 4, Label: "Talkative", Blurb: "Joins group conversations a lot more often.",
				PrivateBlurb: "Answers every message. In private chats this is the same as Natural.",
				Private:      chatPrivate(100, 0, 0, 0), Group: chatGroup(100, 65, true, 0, 0, 0)},
			{Level: 5, Label: "Answers everything", Blurb: "Replies to every single message, even in busy groups.",
				PrivateBlurb: "Answers every message. In private chats this is the same as Natural.",
				Private:      chatPrivate(100, 0, 0, 0), Group: chatGroup(100, 100, false, 0, 0, 0)},
		}}
	bold := DialDef{ID: DialBoldness, Label: "Boldness", Blurb: "Reactions, quotes, tags and the odd typo.",
		Levels: []DialLevel{
			{Level: 1, Label: "Plain", Blurb: "No reactions, no quotes, no typos. Clean and simple.",
				Private: boldPrivate(0, 0), Group: boldGroup(0, 0, 0, 1, 0)},
			{Level: 2, Label: "Mild", Blurb: "A rare typo, and the odd reaction in groups.",
				Private: boldPrivate(0, 2), Group: boldGroup(8, 10, 5, 1, 2)},
			{Level: 3, Label: "Natural", Blurb: "Reacts and quotes now and then and makes the occasional typo.",
				PrivateBlurb: "Makes the occasional typo and fixes it, like a real person.",
				Private:      boldPrivate(0, 5), Group: boldGroup(15, 25, 15, 1, 5)},
			{Level: 4, Label: "Lively", Blurb: "Reacts more, quotes and tags people, a few more typos.",
				Private: boldPrivate(5, 8), Group: boldGroup(25, 35, 25, 2, 8)},
			{Level: 5, Label: "Daring", Blurb: "Lots of reactions, quotes and tags, and typos it fixes on the fly.",
				Private: boldPrivate(10, 12), Group: boldGroup(35, 50, 40, 3, 12)},
		}}
	out := []DialDef{speed, chat, bold}
	for i := range out {
		seen := map[string]bool{}
		for _, l := range out[i].Levels {
			for _, m := range []map[string]any{l.Private, l.Group} {
				for f := range m {
					seen[f] = true
				}
			}
		}
		// Keep the profile's field order (stable JSON, readable UI).
		for _, f := range fields {
			if seen[f.name] {
				out[i].Fields = append(out[i].Fields, f.name)
			}
		}
	}
	return out
}

var dials = buildDials()

// Dials returns the dial definitions in display order (deep copy).
func Dials() []DialDef {
	out := make([]DialDef, len(dials))
	for i, d := range dials {
		d.Fields = slices.Clone(d.Fields)
		lv := make([]DialLevel, len(d.Levels))
		for j, l := range d.Levels {
			l.Private, l.Group = cloneMap(l.Private), cloneMap(l.Group)
			lv[j] = l
		}
		d.Levels = lv
		out[i] = d
	}
	return out
}

// DialTable returns the dials keyed by id (served in GET /api/behavior/presets).
func DialTable() map[string]DialDef {
	out := map[string]DialDef{}
	for _, d := range Dials() {
		out[d.ID] = d
	}
	return out
}

func cloneMap(m map[string]any) map[string]any {
	out := make(map[string]any, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}

func dialByID(id string) (DialDef, bool) {
	for _, d := range dials {
		if d.ID == id {
			return d, true
		}
	}
	return DialDef{}, false
}

// DialFields returns the fields a dial sets for a chat kind.
func DialFields(id, kind string) []string {
	d, ok := dialByID(id)
	if !ok {
		return nil
	}
	m := d.Levels[0].For(kind)
	var out []string
	for _, f := range d.Fields {
		if _, ok := m[f]; ok {
			out = append(out, f)
		}
	}
	return out
}

// DialValues returns the field values a dial level sets for a chat kind
// (nil for an unknown dial or level).
func DialValues(id, kind string, level int) map[string]any {
	d, ok := dialByID(id)
	if !ok || level < 1 || level > len(d.Levels) {
		return nil
	}
	return cloneMap(d.Levels[level-1].For(kind))
}

// ApplyDial sets the dial's fields of p to a level's values (the preset
// label is left alone; Normalize relabels). Reports false for an unknown
// dial or level.
func ApplyDial(p *model.BehaviorProfile, kind, id string, level int) bool {
	vals := DialValues(id, kind, level)
	if vals == nil {
		return false
	}
	pv := reflect.ValueOf(p).Elem()
	for name, v := range vals {
		f, ok := fieldByName(name)
		if !ok {
			panic("behavior: dial field " + name)
		}
		pv.Field(f.pIdx).Set(reflect.ValueOf(v))
	}
	return true
}

// dialNum maps a field value to a comparable number (bools 0/1, "no limit" → +∞).
func dialNum(name string, v any) float64 {
	switch x := v.(type) {
	case bool:
		if x {
			return 1
		}
		return 0
	case int:
		if x == 0 && unlimitedZero[name] {
			return math.Inf(1)
		}
		return float64(x)
	}
	return 0
}

// dialDistance is how far a profile value is from a level value, 0–1 per
// field: log-scaled for durations/counts (5 s vs 15 s matters more than
// 900 s vs 910 s), "no limit" counts as the top of the range.
func dialDistance(name string, have, want any) float64 {
	if hb, ok := have.(bool); ok {
		if hb == want.(bool) {
			return 0
		}
		return 1
	}
	a, b := have.(int), want.(int)
	if a == b {
		return 0
	}
	top := 100.0
	if r, ok := ranges[name]; ok {
		top = float64(r.Max)
	}
	conv := func(n int) float64 {
		if n == 0 && unlimitedZero[name] {
			return math.Log1p(top)
		}
		return math.Log1p(float64(max(n, 0)))
	}
	return math.Min(1, math.Abs(conv(a)-conv(b))/math.Log1p(top))
}

// ReadDials places a profile on every dial for a chat kind: the exact level
// when all of the dial's fields match one (the one closest to Natural when
// several do), else the nearest level with Exact false (the Advanced form
// changed something the dial doesn't express).
func ReadDials(p model.BehaviorProfile, kind string) map[string]DialPos {
	pv := reflect.ValueOf(p)
	out := make(map[string]DialPos, len(dials))
	for _, d := range dials {
		pos := DialPos{Matches: []int{}}
		best := math.Inf(1)
		for _, l := range d.Levels {
			dist := 0.0
			for name, want := range l.For(kind) {
				f, _ := fieldByName(name)
				dist += dialDistance(name, pv.Field(f.pIdx).Interface(), want)
			}
			if dist == 0 {
				pos.Matches = append(pos.Matches, l.Level)
			}
			// Ties go to the level nearest the middle (Natural).
			if dist < best-1e-9 || (math.Abs(dist-best) < 1e-9 && abs(l.Level-3) < abs(pos.Level-3)) {
				best, pos.Level = dist, l.Level
			}
		}
		pos.Exact = best == 0
		out[d.ID] = pos
	}
	return out
}

func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}

// dialMonotone reports whether one field moves in a single direction across
// the levels for a kind (equal neighbours allowed). Used by tests.
func dialMonotone(d DialDef, kind, name string) bool {
	up, down := true, true
	prev := math.NaN()
	for _, l := range d.Levels {
		v, ok := l.For(kind)[name]
		if !ok {
			return true
		}
		n := dialNum(name, v)
		if !math.IsNaN(prev) {
			if n < prev {
				up = false
			}
			if n > prev {
				down = false
			}
		}
		prev = n
	}
	return up || down
}
