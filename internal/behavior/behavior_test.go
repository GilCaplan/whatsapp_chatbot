package behavior

import (
	"encoding/json"
	"math/rand/v2"
	"reflect"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"whatsappdoppel/internal/model"
)

func ptr[T any](v T) *T { return &v }

func TestFieldTablesMatch(t *testing.T) {
	// Every profile field (but preset) has an override with the same JSON name
	// and pointer-to-same type, and every int has a range.
	pt := reflect.TypeFor[model.BehaviorProfile]()
	ot := reflect.TypeFor[model.BehaviorOverrides]()
	if pt.NumField() != ot.NumField() {
		t.Fatalf("profile has %d fields, overrides %d", pt.NumField(), ot.NumField())
	}
	for _, f := range fields {
		pf, of := pt.Field(f.pIdx), ot.Field(f.oIdx)
		if of.Type.Kind() != reflect.Pointer || of.Type.Elem() != pf.Type {
			t.Errorf("%s: override type %s for %s", f.name, of.Type, pf.Type)
		}
		if f.kind == reflect.Int {
			if _, ok := ranges[f.name]; !ok {
				t.Errorf("%s has no range", f.name)
			}
		}
		if f.kind == reflect.String {
			if _, ok := enums[f.name]; !ok {
				t.Errorf("%s has no enum", f.name)
			}
		}
	}
}

func TestPresetsWithinRanges(t *testing.T) {
	seen := map[string]bool{}
	for _, d := range Presets() {
		if seen[d.ID] || d.Label == "" || d.Description == "" {
			t.Errorf("preset %+v", d.ID)
		}
		seen[d.ID] = true
		for _, kind := range []string{KindDM, KindGroup} {
			p := d.For(kind)
			if p.Preset != d.ID {
				t.Errorf("%s/%s preset label %q", d.ID, kind, p.Preset)
			}
			if err := Validate(p); err != nil {
				t.Errorf("%s/%s: %v", d.ID, kind, err)
			}
			c := d.For(kind)
			Normalize(&c, kind)
			if !reflect.DeepEqual(c, p) {
				t.Errorf("%s/%s: Normalize is not a no-op:\n%+v\n%+v", d.ID, kind, c, p)
			}
			if MatchPreset(p, kind) != d.ID {
				t.Errorf("%s/%s: MatchPreset = %s", d.ID, kind, MatchPreset(p, kind))
			}
		}
	}
	if Presets()[0].ID != PresetNatural || Default(KindGroup).ReactPercent != 15 || Default(KindDM).WaitForMoreSec != 8 {
		t.Error("natural must be first/default")
	}
	// Presets are copies.
	p := Presets()
	p[0].Private.Availability.Week[0].Ranges[0].From = "01:00"
	if Default(KindDM).Availability.Week[0].Ranges[0].From != "08:00" {
		t.Error("Presets() must deep-copy")
	}
}

func TestClampSwapsPairsAndFillsEnums(t *testing.T) {
	p := Default(KindDM)
	p.NoticeMinSec, p.NoticeMaxSec = 50, 10
	p.ThinkMinSec, p.ThinkMaxSec = 9999, 1
	p.ReplyPercent = 150
	p.TypingCharsPerSec = 0
	p.LengthBias = ""
	p.InjectionFilter = "bogus"
	p.Availability.OutsideHours = ""
	p.Availability.Timezone = "Mars/Olympus"
	p.Availability.Week = []model.DayHours{{Day: "SUN", Ranges: []model.TimeRange{{From: "9:00", To: "24:00"}, {From: "25:00", To: "1:00"}}}, {Day: "funday"}}
	p.Preset = "weird"
	if Validate(p) == nil {
		t.Fatal("invalid profile should not validate")
	}
	Clamp(&p)
	if p.NoticeMinSec != 10 || p.NoticeMaxSec != 50 || p.ThinkMinSec != 1 || p.ThinkMaxSec != 600 {
		t.Errorf("pairs: notice %d-%d think %d-%d", p.NoticeMinSec, p.NoticeMaxSec, p.ThinkMinSec, p.ThinkMaxSec)
	}
	if p.ReplyPercent != 100 || p.TypingCharsPerSec != 1 || p.LengthBias != "normal" || p.InjectionFilter != "balanced" {
		t.Errorf("clamp: %+v", p)
	}
	if p.Availability.OutsideHours != "queue" || p.Availability.Timezone != "" || p.Preset != "" {
		t.Errorf("availability: %+v preset %q", p.Availability, p.Preset)
	}
	w := p.Availability.Week
	if len(w) != 7 || w[0].Day != "mon" || len(w[0].Ranges) != 0 || w[6].Day != "sun" ||
		len(w[6].Ranges) != 1 || w[6].Ranges[0] != (model.TimeRange{From: "09:00", To: "24:00"}) {
		t.Errorf("week: %+v", w)
	}
	if err := Validate(p); err != nil {
		t.Errorf("clamped profile should validate: %v", err)
	}
	b, _ := json.Marshal(p.Availability.Week[0])
	if !strings.Contains(string(b), `"ranges":[]`) {
		t.Errorf("empty day should marshal as [], got %s", b)
	}
	// Normalize relabels.
	q := Default(KindGroup)
	q.ReplyPercent = 90
	Normalize(&q, KindGroup)
	if q.Preset != PresetCustom {
		t.Errorf("modified natural should be custom, got %q", q.Preset)
	}
	r, _ := Preset(PresetBusy)
	bp := r.Private
	bp.Preset = ""
	Normalize(&bp, KindDM)
	if bp.Preset != PresetBusy {
		t.Errorf("unlabelled busy values should be detected, got %q", bp.Preset)
	}
}

func TestValidateOverrides(t *testing.T) {
	ok := model.BehaviorOverrides{ReplyPercent: ptr(50), LengthBias: ptr("longer"), Preset: "busy",
		Availability: &model.Availability{Enabled: true, OutsideHours: "silent", Week: []model.DayHours{{Day: "mon", Ranges: []model.TimeRange{{From: "08:00", To: "12:00"}}}}}}
	if err := ValidateOverrides(ok); err != nil {
		t.Fatal(err)
	}
	for _, bad := range []model.BehaviorOverrides{
		{ReplyPercent: ptr(101)},
		{SplitMaxParts: ptr(1)},
		{LengthBias: ptr("huge")},
		{Preset: "nope"},
		{Availability: &model.Availability{OutsideHours: "queue", Timezone: "Nowhere/City"}},
		{Availability: &model.Availability{OutsideHours: "queue", Week: []model.DayHours{{Day: "mon", Ranges: []model.TimeRange{{From: "8", To: "9"}}}}}},
		{Availability: &model.Availability{OutsideHours: "later"}},
		{Proactive: &model.Proactive{AfterHours: 0, MaxPerDay: 1}},
		{Proactive: &model.Proactive{AfterHours: 5, MaxPerDay: 11}},
	} {
		if ValidateOverrides(bad) == nil {
			t.Errorf("should reject %+v", bad)
		}
	}
	// Pairs across layers are not rejected.
	if err := ValidateOverrides(model.BehaviorOverrides{NoticeMinSec: ptr(3000)}); err != nil {
		t.Error(err)
	}
}

func TestResolveSourcesAndBlocks(t *testing.T) {
	base := Default(KindGroup)
	chat := model.ChatAssignment{Kind: "group"}
	eff := Resolve(base, chat)
	if !Equal(eff.Profile, base) || eff.Profile.Preset != PresetNatural || eff.Kind != KindGroup {
		t.Fatalf("no overrides should equal base: %+v", eff.Profile)
	}
	for _, n := range FieldNames() {
		if eff.Sources[n] != SourceDefault {
			t.Errorf("source %s = %q", n, eff.Sources[n])
		}
	}
	if len(eff.Sources) != len(FieldNames())+1 {
		t.Errorf("sources has %d entries", len(eff.Sources))
	}

	av := model.Availability{Enabled: true, OutsideHours: "silent", Week: []model.DayHours{{Day: "tue", Ranges: []model.TimeRange{{From: "10:00", To: "11:00"}}}}}
	chat.Behavior = model.BehaviorOverrides{
		ReplyPercent: ptr(70),
		NoticeMinSec: ptr(200), // above the inherited max (90): max follows
		Availability: &av,
	}
	until := time.Date(2026, 10, 1, 9, 0, 0, 0, time.UTC)
	chat.SnoozedUntil = &until
	eff = Resolve(base, chat)
	p := eff.Profile
	if p.ReplyPercent != 70 || p.NoticeMinSec != 200 || p.NoticeMaxSec != 200 {
		t.Errorf("scalars: reply %d notice %d-%d", p.ReplyPercent, p.NoticeMinSec, p.NoticeMaxSec)
	}
	if !p.Availability.Enabled || p.Availability.OutsideHours != "silent" || len(p.Availability.Week) != 7 ||
		len(p.Availability.Week[1].Ranges) != 1 || p.Availability.CatchUpMaxMin != 0 {
		t.Errorf("availability block should replace wholesale: %+v", p.Availability)
	}
	if eff.Sources["replyPercent"] != SourceChat || eff.Sources["availability"] != SourceChat ||
		eff.Sources["noticeMaxSec"] != SourceDefault || eff.Sources["proactive"] != SourceDefault {
		t.Errorf("sources: %+v", eff.Sources)
	}
	if got := eff.Overridden(); len(got) != 3 {
		t.Errorf("overridden = %v", got)
	}
	if p.Preset != PresetCustom || eff.SnoozedUntil == nil || !eff.SnoozedUntil.Equal(until) {
		t.Errorf("preset %q snoozed %v", p.Preset, eff.SnoozedUntil)
	}
	// The override must not alias the chat's week.
	av.Week[0].Ranges[0].From = "00:00"
	if eff.Profile.Availability.Week[1].Ranges[0].From != "10:00" {
		t.Error("resolved availability aliases the override")
	}
	if base.ReplyPercent != 100 {
		t.Error("base mutated")
	}

	// Overriding to exactly a preset's values labels it that preset.
	busy, _ := Preset(PresetBusy)
	var o model.BehaviorOverrides
	b, _ := json.Marshal(busy.Group)
	_ = json.Unmarshal(b, &o)
	o.Preset = PresetBusy
	eff = Resolve(base, model.ChatAssignment{Kind: "group", Behavior: o})
	if eff.Profile.Preset != PresetBusy || !Equal(eff.Profile, busy.Group) || eff.Sources["preset"] != SourceChat {
		t.Errorf("busy override: preset %q", eff.Profile.Preset)
	}
}

func TestMigrateLegacyReplies(t *testing.T) {
	r := LegacyReplies{DebounceSeconds: 5, BurstCapSeconds: 20, TypingIndicator: false, IgnoreLinks: false,
		GroupRandomReplyPercent: 70, TriggerPrefix: "!", MaxHistoryMessages: 25, MaxHistoryChars: 9000, InjectionFilter: "strict"}
	priv, grp := MigrateLegacy(r, LegacyApprovals{AutoSendSeconds: 45})
	for _, p := range []model.BehaviorProfile{priv, grp} {
		if p.WaitForMoreSec != 5 || p.BurstCapSec != 20 || p.TypingIndicator || p.IgnoreLinks ||
			p.HistoryMessages != 25 || p.HistoryChars != 9000 || p.InjectionFilter != "strict" ||
			p.AutoSendSeconds != 45 || p.Preset != PresetCustom {
			t.Errorf("migrated: %+v", p)
		}
	}
	if grp.ChimeInPercent != 70 || priv.ChimeInPercent != 40 {
		t.Errorf("chime: group %d private %d", grp.ChimeInPercent, priv.ChimeInPercent)
	}
	// New fields keep natural values.
	if grp.NoticeMinSec != 10 || priv.SplitPercent != 35 || !priv.MarkRead {
		t.Error("new fields should be natural")
	}
	// Old defaults → only the debounce/burst differ from natural.
	priv, grp = MigrateLegacy(LegacyDefaults(), LegacyApprovals{})
	if priv.WaitForMoreSec != 9 || priv.BurstCapSec != 25 || grp.ChimeInPercent != 40 {
		t.Errorf("defaults: %+v", priv)
	}
	// Empty block → defaults.
	priv, _ = MigrateLegacy(LegacyReplies{}, LegacyApprovals{})
	if priv.WaitForMoreSec != 9 || !priv.TypingIndicator {
		t.Errorf("empty block: %+v", priv)
	}

	ov := MigrateChatOverrides(model.ChatOverrides{DebounceSeconds: ptr(4), GroupRandomReplyPercent: ptr(10)}, model.BehaviorOverrides{})
	if *ov.WaitForMoreSec != 4 || *ov.ChimeInPercent != 10 || ov.AIJudgement != nil {
		t.Errorf("chat overrides: %+v", ov)
	}
	ov = MigrateChatOverrides(model.ChatOverrides{AlwaysReplyInGroup: true, GroupRandomReplyPercent: ptr(10)}, model.BehaviorOverrides{})
	if *ov.ChimeInPercent != 100 || *ov.AIJudgement {
		t.Errorf("always reply: %+v", ov)
	}
}

func TestSplitText(t *testing.T) {
	short := "Sure, see you at 8. Bring snacks!"
	if got := SplitText(short, 3); len(got) != 1 || got[0] != short {
		t.Errorf("short text split: %q", got)
	}
	long := "Honestly I loved it. The second half dragged a little though. Want to go again next week?"
	got := SplitText(long, 3)
	if len(got) != 3 || got[0] != "Honestly I loved it." || got[2] != "Want to go again next week?" {
		t.Errorf("sentences: %q", got)
	}
	if got := SplitText(long, 2); len(got) != 2 || strings.Join(got, " ") != long {
		t.Errorf("maxParts 2: %q", got)
	}
	if got := SplitText(long, 1); len(got) != 1 {
		t.Errorf("maxParts 1: %q", got)
	}
	url := "Look at this one https://example.com/a.b?c=1. It is honestly the best one I have seen all year long!"
	for _, p := range SplitText(url, 3) {
		if strings.Contains(p, "example") && !strings.Contains(p, "https://example.com/a.b?c=1") {
			t.Errorf("URL cut: %q", p)
		}
	}
	tiny := "Ok. Ok. Ok. This is the real sentence that is long enough to stand on its own."
	for _, p := range SplitText(tiny, 5) {
		if utf8.RuneCountInString(p) < 12 {
			t.Errorf("part too short: %q (%q)", p, SplitText(tiny, 5))
		}
	}
	lines := "first line is here and long\nsecond line also quite long\nthird one"
	got = SplitText(lines, 3)
	if len(got) != 2 || got[0] != "first line is here and long" || got[1] != "second line also quite long\nthird one" {
		t.Errorf("newlines: %q", got)
	}
}

func TestTypingDuration(t *testing.T) {
	p := Default(KindDM) // 7 cps, 25% jitter, 1–25 s
	mid := Fixed{Frac: 0.5}
	if d := TypingDuration(p, mid, 70); d != 10*time.Second {
		t.Errorf("70 runes = %v", d)
	}
	if d := TypingDuration(p, mid, 140); d != 20*time.Second {
		t.Errorf("scales with length: %v", d)
	}
	if lo, hi := TypingDuration(p, Fixed{Frac: 0}, 70), TypingDuration(p, Fixed{Frac: 1}, 70); lo != 7500*time.Millisecond || hi != 12500*time.Millisecond {
		t.Errorf("jitter bounds: %v %v", lo, hi)
	}
	if d := TypingDuration(p, mid, 1); d != time.Second {
		t.Errorf("min clamp: %v", d)
	}
	if d := TypingDuration(p, mid, 5000); d != 25*time.Second {
		t.Errorf("max clamp: %v", d)
	}
	p.TypingMaxSec = 0
	p.TypingMinSec = 0
	if d := TypingDuration(p, mid, 500); d != 0 {
		t.Errorf("max 0 = instant: %v", d)
	}
}

func TestPlanDelivery(t *testing.T) {
	eff := Resolve(Default(KindGroup), model.ChatAssignment{Kind: "group"})
	text := "Honestly I loved it. The second half dragged a little though. Want to go again next week?"
	q := &model.QuoteRef{MessageID: "M1", SenderJID: "1@s.whatsapp.net", Text: "how was it"}

	lo := PlanDelivery(eff, Fixed{Frac: 0}, PlanInput{Text: text, Group: true, Quote: q})
	if lo.Notice != 10*time.Second || lo.Wait != 12*time.Second || lo.Think != 3*time.Second || lo.Distracted != 0 {
		t.Errorf("min plan: %+v", lo)
	}
	if len(lo.Bubbles) != 1 || lo.Bubbles[0].Quote != nil || !lo.TypingOn {
		t.Errorf("no hits → no split/quote: %+v", lo.Bubbles)
	}
	hi := PlanDelivery(eff, Fixed{Frac: 1, Hits: true}, PlanInput{Text: text, Group: true, Quote: q})
	if hi.Notice != 90*time.Second || hi.Think != 12*time.Second || hi.Distracted != 300*time.Second {
		t.Errorf("max plan: %+v", hi)
	}
	if len(hi.Bubbles) != 3 || hi.Bubbles[0].Quote == nil || hi.Bubbles[0].Quote.MessageID != "M1" ||
		hi.Bubbles[1].Quote != nil || hi.Bubbles[0].GapBefore != 0 || hi.Bubbles[1].GapBefore != 4*time.Second {
		t.Errorf("hits → split + quote first bubble: %+v", hi.Bubbles)
	}
	if hi.Total() != hi.Notice+hi.Wait+hi.Think+hi.Distracted+hi.Bubbles[0].Typing+hi.Bubbles[1].Typing+hi.Bubbles[2].Typing+8*time.Second {
		t.Errorf("total = %v", hi.Total())
	}

	ap := PlanDelivery(eff, Fixed{Frac: 1, Hits: true}, PlanInput{Text: strings.Repeat("word ", 60), Approval: true, Group: true, Quote: q})
	if ap.Think != 0 || ap.Distracted != 0 || ap.Notice == 0 {
		t.Errorf("approval: %+v", ap)
	}
	for _, b := range ap.Bubbles {
		if b.Typing > QuickTypingCap || b.Quote != nil {
			t.Errorf("approval bubble: %+v", b)
		}
	}
	im := PlanDelivery(eff, Fixed{Frac: 1, Hits: true}, PlanInput{Text: text, Immediate: true, Group: true, Quote: q})
	if im.Notice != 0 || im.Wait != 0 || im.Think != 0 || im.Distracted != 0 || im.Bubbles[0].Typing > QuickTypingCap || im.Bubbles[0].Quote != nil {
		t.Errorf("immediate: %+v", im)
	}
	dm := PlanDelivery(Resolve(Default(KindDM), model.ChatAssignment{}), Fixed{Frac: 1, Hits: true}, PlanInput{Text: text, Quote: q})
	for _, b := range dm.Bubbles {
		if b.Quote != nil {
			t.Error("DMs never quote")
		}
	}
}

func TestNextOpen(t *testing.T) {
	utc := time.UTC
	av := model.Availability{Enabled: true, Timezone: "UTC", OutsideHours: "queue",
		Week: []model.DayHours{{Day: "mon", Ranges: []model.TimeRange{{From: "09:00", To: "17:00"}}},
			{Day: "fri", Ranges: []model.TimeRange{{From: "22:00", To: "02:00"}}}}}
	mon := time.Date(2026, 10, 5, 0, 0, 0, 0, utc) // a Monday
	at := func(day, h, m int) time.Time {
		return mon.AddDate(0, 0, day).Add(time.Duration(h)*time.Hour + time.Duration(m)*time.Minute)
	}

	if open, next := NextOpen(av, at(0, 10, 0)); !open || !next.Equal(at(0, 17, 0)) {
		t.Errorf("inside: %v %v", open, next)
	}
	if open, next := NextOpen(av, at(0, 8, 0)); open || !next.Equal(at(0, 9, 0)) {
		t.Errorf("before: %v %v", open, next)
	}
	if open, next := NextOpen(av, at(0, 17, 0)); open || !next.Equal(at(4, 22, 0)) {
		t.Errorf("after → friday: %v %v", open, next)
	}
	if open, next := NextOpen(av, at(5, 1, 30)); !open || !next.Equal(at(5, 2, 0)) {
		t.Errorf("wrap past midnight: %v %v", open, next)
	}
	if open, next := NextOpen(av, at(5, 3, 0)); open || !next.Equal(at(7, 9, 0)) {
		t.Errorf("weekend → next monday: %v %v", open, next)
	}
	// Disabled → always open.
	av2 := av
	av2.Enabled = false
	if open, next := NextOpen(av2, at(0, 3, 0)); !open || !next.IsZero() {
		t.Error("disabled should be open")
	}
	// No hours at all → closed forever.
	if open, next := NextOpen(model.Availability{Enabled: true, Week: NormalizeWeek([]model.DayHours{{Day: "mon"}})}, at(0, 3, 0)); open || !next.IsZero() {
		t.Error("empty week should be closed with no next")
	}
	// All day every day → open, never closes.
	full := model.Availability{Enabled: true, Week: everyDay("00:00", "00:00")}
	if open, next := NextOpen(full, at(0, 3, 0)); !open || !next.IsZero() {
		t.Errorf("24/7: %v %v", open, next)
	}
	// DST: Europe/Berlin springs forward on 2026-03-29 (02:00 → 03:00).
	ber, err := time.LoadLocation("Europe/Berlin")
	if err != nil {
		t.Skip("no tzdata")
	}
	dst := model.Availability{Enabled: true, Timezone: "Europe/Berlin", Week: everyDay("08:00", "01:00")}
	sat := time.Date(2026, 3, 28, 23, 30, 0, 0, ber)
	if open, next := NextOpen(dst, sat); !open || !next.Equal(time.Date(2026, 3, 29, 1, 0, 0, 0, ber)) {
		t.Errorf("DST night: %v %v", open, next)
	}
	if open, next := NextOpen(dst, time.Date(2026, 3, 29, 4, 0, 0, 0, ber)); open || !next.Equal(time.Date(2026, 3, 29, 8, 0, 0, 0, ber)) || next.Sub(time.Date(2026, 3, 29, 1, 0, 0, 0, ber)) != 6*time.Hour {
		t.Errorf("DST morning: %v %v", open, next)
	}

	// Snooze via Effective.
	until := at(0, 12, 0)
	eff := Effective{Profile: model.BehaviorProfile{Availability: av}, SnoozedUntil: &until}
	if open, next := eff.Available(at(0, 10, 0)); open || !next.Equal(until) {
		t.Errorf("snoozed inside hours: %v %v", open, next)
	}
	late := at(0, 18, 0)
	eff.SnoozedUntil = &late
	if open, next := eff.Available(at(0, 10, 0)); open || !next.Equal(at(4, 22, 0)) {
		t.Errorf("snooze ends outside hours: %v %v", open, next)
	}
	if open, _ := eff.Available(at(4, 23, 0)); !open {
		t.Error("expired snooze should not block")
	}
}

func TestSamplerDeterministic(t *testing.T) {
	a, b := NewSampler(rand.NewPCG(1, 2)), NewSampler(rand.NewPCG(1, 2))
	for range 50 {
		if a.Between(0, 100) != b.Between(0, 100) || a.Hit(50) != b.Hit(50) || a.Index(7) != b.Index(7) {
			t.Fatal("same seed should give the same sequence")
		}
	}
	s := NewSampler(nil)
	for range 200 {
		if d := s.Between(3, 5); d < 3*time.Second || d > 5*time.Second {
			t.Fatalf("between out of range: %v", d)
		}
		if d := s.Jitter(10*time.Second, 20); d < 8*time.Second || d > 12*time.Second {
			t.Fatalf("jitter out of range: %v", d)
		}
		if s.Hit(0) || !s.Hit(100) {
			t.Fatal("hit bounds")
		}
		if i := s.Index(3); i < 0 || i > 2 {
			t.Fatal("index")
		}
	}
	f := Fixed{Frac: 0.5}
	if f.Between(2, 4) != 3*time.Second || f.Hit(50) || !f.Hit(100) || f.Index(4) != 2 {
		t.Error("fixed sampler")
	}
}

func TestPickReaction(t *testing.T) {
	p := model.Persona{Emoji: model.EmojiPrefs{Usage: "some", Favorites: []string{" ", "💅", "✨"}}}
	if r := PickReaction(p, Fixed{Frac: 0.9}); r != "✨" {
		t.Errorf("favourite: %q", r)
	}
	if r := PickReaction(model.Persona{Emoji: model.EmojiPrefs{Usage: "none"}}, Fixed{}); r != "👍" {
		t.Errorf("no-emoji persona: %q", r)
	}
	if r := PickReaction(model.Persona{}, Fixed{}); r != DefaultReactions[0] {
		t.Errorf("default: %q", r)
	}
}
