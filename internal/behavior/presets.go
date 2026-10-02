// Package behavior holds the reply-behaviour logic shared by config, server
// and engine: presets, ranges/validation, the layered resolver (app profile →
// per-chat overrides), active hours, the delivery planner (notice/think/typing
// timings, bubble splitting) and the injectable randomness. It depends only on
// model and the standard library.
package behavior

import (
	"reflect"
	"slices"

	"whatsappdoppel/internal/model"
)

// Preset ids.
const (
	PresetNatural  = "natural"
	PresetInstant  = "instant"
	PresetBusy     = "busy"
	PresetSlow     = "slow"
	PresetNightOwl = "nightowl"
	PresetCustom   = "custom"
)

// Chat kinds.
const (
	KindDM    = "dm"
	KindGroup = "group"
)

// PresetDef is one named preset with values for both chat kinds.
type PresetDef struct {
	ID          string                `json:"id"`
	Label       string                `json:"label"`
	Description string                `json:"description"`
	Private     model.BehaviorProfile `json:"private"`
	Group       model.BehaviorProfile `json:"group"`
}

// For returns the preset's profile for a chat kind ("group" or anything else = private).
func (p PresetDef) For(kind string) model.BehaviorProfile {
	if kind == KindGroup {
		return clone(p.Group)
	}
	return clone(p.Private)
}

// everyDay returns a 7-day week with the same single range.
func everyDay(from, to string) []model.DayHours {
	out := make([]model.DayHours, 0, 7)
	for _, d := range Days {
		out = append(out, model.DayHours{Day: d, Ranges: []model.TimeRange{{From: from, To: to}}})
	}
	return out
}

func naturalPrivate() model.BehaviorProfile {
	return model.BehaviorProfile{
		Preset:                  PresetNatural,
		ReplyPercent:            100,
		ReplyWhenNameMentioned:  true,
		ReplyWhenAtMentioned:    true,
		SkipWhenOthersMentioned: true,
		AIJudgement:             true,
		ChimeInPercent:          40,
		IgnoreLinks:             true,
		ReactPercent:            0,
		PauseWhenYouReply:       true,
		HistoryMessages:         40,
		HistoryChars:            12000,
		StaleAfterMin:           15,

		RespondToAllMaxMembers:     8,
		AnswerAnyoneWhoAddressesIt: true,
		MaxStreak:                  0,
		TriggerWords:               []string{},
		MuteWords:                  []string{},

		NoticeMinSec:        3,
		NoticeMaxSec:        25,
		MarkRead:            true,
		WaitForMoreSec:      8,
		BurstCapSec:         30,
		ThinkMinSec:         2,
		ThinkMaxSec:         8,
		DistractedPercent:   10,
		DistractedMinSec:    30,
		DistractedMaxSec:    120,
		TypingIndicator:     true,
		TypingCharsPerSec:   7,
		TypingJitterPercent: 25,
		TypingMinSec:        1,
		TypingMaxSec:        25,

		SplitPercent:      35,
		SplitMaxParts:     3,
		BubbleGapMinSec:   1,
		BubbleGapMaxSec:   4,
		QuoteReplyPercent: 0,
		AllowMentions:     true,
		MentionMax:        1,
		TagReplyPercent:   0,
		LengthBias:        "normal",
		TypoPercent:       5,
		TypoFixStyle:      "correction",

		Availability: model.Availability{
			Enabled:       false,
			Timezone:      "",
			Week:          everyDay("08:00", "23:00"),
			OutsideHours:  "queue",
			CatchUpMaxMin: 20,
		},
		Proactive: model.Proactive{Enabled: false, AfterHours: 48, MaxPerDay: 1, SpreadMinutes: 180},

		AutoSendSeconds: 0,
		InjectionFilter: "balanced",
	}
}

func naturalGroup() model.BehaviorProfile {
	p := naturalPrivate()
	p.ReactPercent = 15
	p.NoticeMinSec, p.NoticeMaxSec = 10, 90
	p.WaitForMoreSec, p.BurstCapSec = 12, 45
	p.ThinkMinSec, p.ThinkMaxSec = 3, 12
	p.DistractedPercent, p.DistractedMinSec, p.DistractedMaxSec = 15, 60, 300
	p.SplitPercent = 20
	p.QuoteReplyPercent = 25
	p.TagReplyPercent = 15
	p.MaxStreak = 6
	p.LengthBias = "shorter"
	return p
}

func presetTable() []PresetDef {
	np, ng := naturalPrivate(), naturalGroup()

	ip, ig := np, ng
	for _, p := range []*model.BehaviorProfile{&ip, &ig} {
		p.Preset = PresetInstant
		p.ReactPercent = 0
		p.NoticeMinSec, p.NoticeMaxSec = 0, 0
		p.ThinkMinSec, p.ThinkMaxSec = 0, 1
		p.DistractedPercent, p.DistractedMinSec, p.DistractedMaxSec = 0, 0, 0
		p.TypingCharsPerSec, p.TypingJitterPercent, p.TypingMinSec, p.TypingMaxSec = 20, 10, 0, 5
		p.SplitPercent = 0
		p.QuoteReplyPercent = 0
		p.LengthBias = "normal"
		p.TypoPercent = 0
	}
	ip.WaitForMoreSec, ip.BurstCapSec = 2, 10
	ig.WaitForMoreSec, ig.BurstCapSec = 3, 10
	ig.TagReplyPercent = 0

	bp, bg := np, ng
	for _, p := range []*model.BehaviorProfile{&bp, &bg} {
		p.Preset = PresetBusy
		p.ChimeInPercent = 20
		p.ThinkMinSec, p.ThinkMaxSec = 5, 30
		p.DistractedPercent, p.DistractedMinSec, p.DistractedMaxSec = 30, 120, 900
		p.TypingCharsPerSec, p.TypingJitterPercent, p.TypingMinSec, p.TypingMaxSec = 6, 25, 1, 30
		p.SplitPercent = 25
	}
	bp.ReplyPercent, bg.ReplyPercent = 85, 100
	bp.ReactPercent, bg.ReactPercent = 0, 15 // v4: was 5/25 (lands on Boldness 3)
	bp.NoticeMinSec, bp.NoticeMaxSec = 20, 240
	bg.NoticeMinSec, bg.NoticeMaxSec = 60, 600
	bp.WaitForMoreSec, bp.BurstCapSec = 15, 60
	bg.WaitForMoreSec, bg.BurstCapSec = 20, 90
	bp.LengthBias, bg.LengthBias = "normal", "shorter"
	bp.MaxRepliesPerHour, bp.MaxRepliesPerDay, bp.CooldownSec = 6, 40, 60
	bg.MaxRepliesPerHour, bg.MaxRepliesPerDay, bg.CooldownSec = 4, 30, 120

	sp, sg := np, ng
	for _, p := range []*model.BehaviorProfile{&sp, &sg} {
		p.Preset = PresetSlow
		p.ChimeInPercent = 25
		p.ThinkMinSec, p.ThinkMaxSec = 10, 60
		p.DistractedPercent, p.DistractedMinSec, p.DistractedMaxSec = 35, 300, 1800 // v4: was 25 (Speed 1)
		p.TypingCharsPerSec, p.TypingJitterPercent, p.TypingMinSec, p.TypingMaxSec = 4, 35, 1, 60
		p.SplitPercent = 15
		p.LengthBias = "shorter"
		p.CooldownSec = 30
	}
	sp.ReactPercent, sg.ReactPercent = 0, 15 // v4: was 0/10 (Boldness 3)
	sp.NoticeMinSec, sp.NoticeMaxSec = 60, 900
	sg.NoticeMinSec, sg.NoticeMaxSec = 120, 1800
	sp.WaitForMoreSec, sp.BurstCapSec = 20, 120
	sg.WaitForMoreSec, sg.BurstCapSec = 30, 120

	op, og := np, ng
	for _, p := range []*model.BehaviorProfile{&op, &og} {
		p.Preset = PresetNightOwl
		p.ThinkMinSec, p.ThinkMaxSec = 2, 8
		p.DistractedPercent, p.DistractedMinSec, p.DistractedMaxSec = 10, 30, 120
		p.Availability = model.Availability{
			Enabled:       true,
			Week:          everyDay("18:00", "02:30"),
			OutsideHours:  "queue",
			CatchUpMaxMin: 30,
		}
	}
	op.NoticeMinSec, op.NoticeMaxSec = 2, 15
	og.NoticeMinSec, og.NoticeMaxSec = 5, 60

	return []PresetDef{
		{ID: PresetNatural, Label: "Natural", Description: "Replies like a friend who has their phone nearby: a short pause, sometimes a few bubbles.", Private: np, Group: ng},
		{ID: PresetInstant, Label: "Instant", Description: "Answers within seconds, one message at a time. Good for testing.", Private: ip, Group: ig},
		{ID: PresetBusy, Label: "Busy", Description: "Takes a while to notice, sometimes skips a message, keeps it brief and caps how often it replies.", Private: bp, Group: bg},
		{ID: PresetSlow, Label: "Slow texter", Description: "Checks the phone now and then and types slowly. Replies can take many minutes.", Private: sp, Group: sg},
		{ID: PresetNightOwl, Label: "Night owl", Description: "Only active in the evening and at night; messages sent during the day are answered after 18:00.", Private: op, Group: og},
	}
}

var presets = presetTable()

// Presets returns every preset (Natural first). The result is a deep copy.
func Presets() []PresetDef {
	out := make([]PresetDef, len(presets))
	for i, p := range presets {
		p.Private, p.Group = clone(p.Private), clone(p.Group)
		out[i] = p
	}
	return out
}

// Preset returns a preset by id.
func Preset(id string) (PresetDef, bool) {
	for _, p := range presets {
		if p.ID == id {
			p.Private, p.Group = clone(p.Private), clone(p.Group)
			return p, true
		}
	}
	return PresetDef{}, false
}

// Default returns the default (Natural) profile for a chat kind.
func Default(kind string) model.BehaviorProfile {
	p, _ := Preset(PresetNatural)
	return p.For(kind)
}

// PresetIDs lists valid preset labels including "custom".
func PresetIDs() []string {
	ids := make([]string, 0, len(presets)+1)
	for _, p := range presets {
		ids = append(ids, p.ID)
	}
	return append(ids, PresetCustom)
}

// FillFromPreset copies the named JSON fields from the preset p is labelled
// with (Natural when the label is "custom" or unknown), keeping the label.
// Used to give fields added in a newer settings version their preset values.
func FillFromPreset(p *model.BehaviorProfile, kind string, names ...string) {
	d, ok := Preset(p.Preset)
	if !ok {
		d, _ = Preset(PresetNatural)
	}
	src := d.For(kind)
	dst := reflect.ValueOf(p).Elem()
	sv := reflect.ValueOf(src)
	for _, n := range names {
		f, ok := fieldByName(n)
		if !ok {
			panic("behavior: FillFromPreset: unknown field " + n)
		}
		dst.Field(f.pIdx).Set(sv.Field(f.pIdx))
	}
}

// FillOverridesFromPreset gives a chat that applied a whole named preset
// (label set and the older fields overridden) the preset's values for the
// named fields it lacks, so it keeps matching that preset after new fields
// were added. Reports whether anything changed.
func FillOverridesFromPreset(o *model.BehaviorOverrides, kind string, names ...string) bool {
	d, ok := Preset(o.Preset)
	if !ok || o.QuoteReplyPercent == nil || o.ReplyPercent == nil {
		return false
	}
	src := reflect.ValueOf(d.For(kind))
	ov := reflect.ValueOf(o).Elem()
	changed := false
	for _, n := range names {
		f, ok := fieldByName(n)
		if !ok {
			panic("behavior: FillOverridesFromPreset: unknown field " + n)
		}
		if !ov.Field(f.oIdx).IsNil() {
			continue
		}
		ptr := reflect.New(src.Field(f.pIdx).Type())
		ptr.Elem().Set(src.Field(f.pIdx))
		ov.Field(f.oIdx).Set(ptr)
		changed = true
	}
	return changed
}

// Equal reports whether two profiles have the same values, ignoring the preset label.
func Equal(a, b model.BehaviorProfile) bool {
	a.Preset, b.Preset = "", ""
	for _, p := range []*model.BehaviorProfile{&a, &b} {
		if p.TriggerWords == nil {
			p.TriggerWords = []string{}
		}
		if p.MuteWords == nil {
			p.MuteWords = []string{}
		}
	}
	if !slices.EqualFunc(a.Availability.Week, b.Availability.Week, func(x, y model.DayHours) bool {
		return x.Day == y.Day && slices.Equal(x.Ranges, y.Ranges)
	}) {
		return false
	}
	a.Availability.Week, b.Availability.Week = nil, nil
	return reflect.DeepEqual(a, b)
}

// MatchPreset returns the id of the preset whose values (for kind) equal p,
// or "custom".
func MatchPreset(p model.BehaviorProfile, kind string) string {
	for _, d := range presets {
		if Equal(p, d.For(kind)) {
			return d.ID
		}
	}
	return PresetCustom
}

// Clone deep-copies a profile (the availability week).
func Clone(p model.BehaviorProfile) model.BehaviorProfile { return clone(p) }

func clone(p model.BehaviorProfile) model.BehaviorProfile {
	p.Availability = cloneAvailability(p.Availability)
	if p.TriggerWords != nil {
		p.TriggerWords = slices.Clone(p.TriggerWords)
	}
	if p.MuteWords != nil {
		p.MuteWords = slices.Clone(p.MuteWords)
	}
	return p
}

func cloneAvailability(a model.Availability) model.Availability {
	if a.Week != nil {
		w := make([]model.DayHours, len(a.Week))
		for i, d := range a.Week {
			w[i] = model.DayHours{Day: d.Day, Ranges: slices.Clone(d.Ranges)}
		}
		a.Week = w
	}
	return a
}
