package behavior

import (
	"fmt"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"whatsappdoppel/internal/model"
)

// Range is the inclusive allowed range of an integer field.
type Range struct {
	Min int `json:"min"`
	Max int `json:"max"`
}

// Days are the weekday ids used in Availability.Week, in order.
var Days = []string{"mon", "tue", "wed", "thu", "fri", "sat", "sun"}

// ranges is keyed by JSON name; nested fields use "block.field".
var ranges = map[string]Range{
	"replyPercent":      {0, 100},
	"chimeInPercent":    {0, 100},
	"reactPercent":      {0, 100},
	"distractedPercent": {0, 100},
	"splitPercent":      {0, 100},
	"quoteReplyPercent": {0, 100},
	"tagReplyPercent":   {0, 100},
	"mentionMax":        {1, 5},
	"typoPercent":       {0, 30},

	"respondToAllMaxMembers": {0, 1024},
	"maxStreak":              {0, 50},

	"typingJitterPercent": {0, 80},

	"noticeMinSec":     {0, 3600},
	"noticeMaxSec":     {0, 3600},
	"distractedMinSec": {0, 3600},
	"distractedMaxSec": {0, 3600},
	"cooldownSec":      {0, 3600},

	"waitForMoreSec": {0, 600},
	"thinkMinSec":    {0, 600},
	"thinkMaxSec":    {0, 600},
	"burstCapSec":    {0, 1800},

	"typingCharsPerSec": {1, 40},
	"typingMinSec":      {0, 60},
	"typingMaxSec":      {0, 180},

	"bubbleGapMinSec": {0, 60},
	"bubbleGapMaxSec": {0, 60},
	"splitMaxParts":   {2, 5},

	"maxRepliesPerHour": {0, 600},
	"maxRepliesPerDay":  {0, 5000},

	"historyMessages": {2, 500},
	"historyChars":    {500, 200000},
	"staleAfterMin":   {1, 1440},

	"autoSendSeconds": {0, 86400},

	"availability.catchUpMaxMin": {0, 240},
	"proactive.afterHours":       {1, 720},
	"proactive.maxPerDay":        {1, 10},
	"proactive.spreadMinutes":    {0, 720},
}

var enums = map[string][]string{
	"injectionFilter":           {"strict", "balanced", "off"},
	"lengthBias":                {"shorter", "normal", "longer", "match"},
	"typoFixStyle":              {"correction", "edit", "none"},
	"availability.outsideHours": {"queue", "silent"},
	"availability.week.day":     Days,
}

// enumDefaults is what Clamp substitutes for an empty or unknown enum value.
var enumDefaults = map[string]string{
	"injectionFilter":           "balanced",
	"lengthBias":                "normal",
	"typoFixStyle":              "correction",
	"availability.outsideHours": "queue",
}

// Ranges returns a copy of the integer ranges, keyed by JSON name
// (nested: "availability.catchUpMaxMin", "proactive.afterHours", ...).
func Ranges() map[string]Range {
	out := make(map[string]Range, len(ranges))
	for k, v := range ranges {
		out[k] = v
	}
	return out
}

// Enums returns a copy of the allowed string values, keyed by JSON name
// (includes "preset").
func Enums() map[string][]string {
	out := make(map[string][]string, len(enums)+1)
	for k, v := range enums {
		out[k] = slices.Clone(v)
	}
	out["preset"] = PresetIDs()
	return out
}

// minMaxPairs are reconciled (max >= min) by Clamp and Resolve.
var minMaxPairs = [][2]string{
	{"noticeMinSec", "noticeMaxSec"},
	{"thinkMinSec", "thinkMaxSec"},
	{"distractedMinSec", "distractedMaxSec"},
	{"typingMinSec", "typingMaxSec"},
	{"bubbleGapMinSec", "bubbleGapMaxSec"},
}

// ─── reflection tables (built once) ──────────────────────────

type field struct {
	name  string // JSON name
	pIdx  int    // index in BehaviorProfile
	oIdx  int    // index in BehaviorOverrides
	kind  reflect.Kind
	block bool // availability / proactive (struct)
}

var fields = buildFields()

func jsonName(f reflect.StructField) string {
	n, _, _ := strings.Cut(f.Tag.Get("json"), ",")
	return n
}

func buildFields() []field {
	pt := reflect.TypeFor[model.BehaviorProfile]()
	ot := reflect.TypeFor[model.BehaviorOverrides]()
	oIdx := map[string]int{}
	for i := range ot.NumField() {
		oIdx[jsonName(ot.Field(i))] = i
	}
	var out []field
	for i := range pt.NumField() {
		sf := pt.Field(i)
		name := jsonName(sf)
		if name == "preset" {
			continue
		}
		oi, ok := oIdx[name]
		if !ok {
			panic("behavior: BehaviorOverrides lacks field " + name)
		}
		out = append(out, field{name: name, pIdx: i, oIdx: oi, kind: sf.Type.Kind(), block: sf.Type.Kind() == reflect.Struct})
	}
	return out
}

func fieldByName(name string) (field, bool) {
	for _, f := range fields {
		if f.name == name {
			return f, true
		}
	}
	return field{}, false
}

// FieldNames lists every overridable JSON field name of a profile (no "preset").
func FieldNames() []string {
	out := make([]string, len(fields))
	for i, f := range fields {
		out[i] = f.name
	}
	return out
}

func intPtr(p *model.BehaviorProfile, name string) *int {
	f, ok := fieldByName(name)
	if !ok || f.kind != reflect.Int {
		panic("behavior: no int field " + name)
	}
	return reflect.ValueOf(p).Elem().Field(f.pIdx).Addr().Interface().(*int)
}

// ─── validation ──────────────────────────────────────────────

func checkRange(name string, v int) error {
	r, ok := ranges[name]
	if !ok {
		return nil
	}
	if v < r.Min || v > r.Max {
		return fmt.Errorf("%s must be between %d and %d", name, r.Min, r.Max)
	}
	return nil
}

func checkEnum(name, v string) error {
	allowed, ok := enums[name]
	if !ok || slices.Contains(allowed, v) {
		return nil
	}
	return fmt.Errorf("%s must be one of %s", name, strings.Join(allowed, ", "))
}

func checkPreset(v string) error {
	if v == "" || slices.Contains(PresetIDs(), v) {
		return nil
	}
	return fmt.Errorf("preset must be one of %s", strings.Join(PresetIDs(), ", "))
}

// Validate reports the first out-of-range or invalid value of a complete profile.
// min > max pairs are accepted (Clamp swaps them).
func Validate(p model.BehaviorProfile) error {
	if err := checkPreset(p.Preset); err != nil {
		return err
	}
	v := reflect.ValueOf(p)
	for _, f := range fields {
		if err := validateValue(f, v.Field(f.pIdx)); err != nil {
			return err
		}
	}
	return nil
}

// ValidateOverrides checks each non-nil override on its own (pairs spanning
// layers are reconciled by Resolve, never rejected).
func ValidateOverrides(o model.BehaviorOverrides) error {
	if err := checkPreset(o.Preset); err != nil {
		return err
	}
	v := reflect.ValueOf(o)
	for _, f := range fields {
		ov := v.Field(f.oIdx)
		if ov.IsNil() {
			continue
		}
		if err := validateValue(f, ov.Elem()); err != nil {
			return err
		}
	}
	return nil
}

func validateValue(f field, v reflect.Value) error {
	switch f.kind {
	case reflect.Int:
		return checkRange(f.name, int(v.Int()))
	case reflect.String:
		return checkEnum(f.name, v.String())
	case reflect.Slice:
		return checkWords(f.name, v.Interface().([]string))
	case reflect.Struct:
		switch x := v.Interface().(type) {
		case model.Availability:
			return validateAvailability(x)
		case model.Proactive:
			return validateProactive(x)
		}
	}
	return nil
}

func validateAvailability(a model.Availability) error {
	if a.Timezone != "" && a.Timezone != model.PersonaZoneMarker {
		if _, err := time.LoadLocation(a.Timezone); err != nil {
			return fmt.Errorf("availability.timezone %q is not a known time zone", a.Timezone)
		}
	}
	if err := checkEnum("availability.outsideHours", a.OutsideHours); err != nil {
		return err
	}
	if err := checkRange("availability.catchUpMaxMin", a.CatchUpMaxMin); err != nil {
		return err
	}
	seen := map[string]bool{}
	for _, d := range a.Week {
		day := strings.ToLower(strings.TrimSpace(d.Day))
		if err := checkEnum("availability.week.day", day); err != nil {
			return err
		}
		if seen[day] {
			return fmt.Errorf("availability.week lists %s twice", day)
		}
		seen[day] = true
		for _, r := range d.Ranges {
			if _, ok := parseHM(r.From, false); !ok {
				return fmt.Errorf("availability.week %s: %q is not a time (HH:MM)", day, r.From)
			}
			if _, ok := parseHM(r.To, true); !ok {
				return fmt.Errorf("availability.week %s: %q is not a time (HH:MM)", day, r.To)
			}
		}
	}
	return nil
}

func validateProactive(p model.Proactive) error {
	for name, v := range map[string]int{
		"proactive.afterHours":    p.AfterHours,
		"proactive.maxPerDay":     p.MaxPerDay,
		"proactive.spreadMinutes": p.SpreadMinutes,
	} {
		if err := checkRange(name, v); err != nil {
			return err
		}
	}
	return nil
}

// Word lists (triggerWords, muteWords).
const (
	MaxWords     = 20
	MaxWordRunes = 40
)

func checkWords(name string, ws []string) error {
	n := 0
	for _, w := range ws {
		w = strings.TrimSpace(w)
		if w == "" {
			continue
		}
		n++
		if utf8.RuneCountInString(w) > MaxWordRunes {
			return fmt.Errorf("%s: %q is longer than %d characters", name, w, MaxWordRunes)
		}
	}
	if n > MaxWords {
		return fmt.Errorf("%s can have at most %d entries", name, MaxWords)
	}
	return nil
}

// NormalizeWords trims and collapses whitespace, drops empty entries and
// case-insensitive duplicates, caps each entry at MaxWordRunes and the list at
// MaxWords. The result is never nil.
func NormalizeWords(ws []string) []string {
	out := make([]string, 0, len(ws))
	seen := map[string]bool{}
	for _, w := range ws {
		w = strings.Join(strings.Fields(w), " ")
		if r := []rune(w); len(r) > MaxWordRunes {
			w = strings.TrimSpace(string(r[:MaxWordRunes]))
		}
		k := strings.ToLower(w)
		if w == "" || seen[k] {
			continue
		}
		seen[k] = true
		out = append(out, w)
		if len(out) == MaxWords {
			break
		}
	}
	return out
}

// parseHM parses "H:MM"/"HH:MM" into minutes after midnight. "24:00" is only
// allowed as an end time.
func parseHM(s string, end bool) (int, bool) {
	h, m, ok := strings.Cut(strings.TrimSpace(s), ":")
	if !ok || len(m) != 2 || len(h) == 0 || len(h) > 2 {
		return 0, false
	}
	hh, err1 := strconv.Atoi(h)
	mm, err2 := strconv.Atoi(m)
	if err1 != nil || err2 != nil || hh < 0 || mm < 0 || mm > 59 {
		return 0, false
	}
	if hh == 24 && mm == 0 && end {
		return 24 * 60, true
	}
	if hh > 23 {
		return 0, false
	}
	return hh*60 + mm, true
}

func fmtHM(m int) string { return fmt.Sprintf("%02d:%02d", m/60, m%60) }

// ─── clamping / normalisation ────────────────────────────────

func clampInt(name string, v *int) {
	if r, ok := ranges[name]; ok {
		*v = min(max(*v, r.Min), r.Max)
	}
}

func clampEnum(name string, v *string) {
	*v = strings.ToLower(strings.TrimSpace(*v))
	if !slices.Contains(enums[name], *v) {
		*v = enumDefaults[name]
	}
}

// Clamp silently repairs a profile: integers into range, unknown enums to
// their default, min/max pairs ordered, the week normalised to mon..sun and
// an unknown time zone replaced by "" (this Mac).
func Clamp(p *model.BehaviorProfile) {
	v := reflect.ValueOf(p).Elem()
	for _, f := range fields {
		fv := v.Field(f.pIdx)
		switch f.kind {
		case reflect.Int:
			n := int(fv.Int())
			clampInt(f.name, &n)
			fv.SetInt(int64(n))
		case reflect.String:
			s := fv.String()
			clampEnum(f.name, &s)
			fv.SetString(s)
		case reflect.Slice:
			fv.Set(reflect.ValueOf(NormalizeWords(fv.Interface().([]string))))
		}
	}
	for _, pr := range minMaxPairs {
		lo, hi := intPtr(p, pr[0]), intPtr(p, pr[1])
		if *lo > *hi {
			*lo, *hi = *hi, *lo
		}
	}
	clampAvailability(&p.Availability)
	clampProactive(&p.Proactive)
	if p.Preset != "" && !slices.Contains(PresetIDs(), p.Preset) {
		p.Preset = ""
	}
}

func clampAvailability(a *model.Availability) {
	if a.Timezone != "" && a.Timezone != model.PersonaZoneMarker {
		if _, err := time.LoadLocation(a.Timezone); err != nil {
			a.Timezone = ""
		}
	}
	clampEnum("availability.outsideHours", &a.OutsideHours)
	clampInt("availability.catchUpMaxMin", &a.CatchUpMaxMin)
	a.Week = NormalizeWeek(a.Week)
}

func clampProactive(p *model.Proactive) {
	clampInt("proactive.afterHours", &p.AfterHours)
	clampInt("proactive.maxPerDay", &p.MaxPerDay)
	clampInt("proactive.spreadMinutes", &p.SpreadMinutes)
}

// NormalizeWeek returns exactly 7 entries mon..sun with valid "HH:MM" ranges.
// An empty week becomes the default (every day 08:00–23:00); duplicate days merge.
func NormalizeWeek(w []model.DayHours) []model.DayHours {
	if len(w) == 0 {
		return everyDay("08:00", "23:00")
	}
	byDay := map[string][]model.TimeRange{}
	for _, d := range w {
		day := strings.ToLower(strings.TrimSpace(d.Day))
		if !slices.Contains(Days, day) {
			continue
		}
		for _, r := range d.Ranges {
			from, ok1 := parseHM(r.From, false)
			to, ok2 := parseHM(r.To, true)
			if ok1 && ok2 {
				byDay[day] = append(byDay[day], model.TimeRange{From: fmtHM(from), To: fmtHM(to)})
			}
		}
	}
	out := make([]model.DayHours, 0, 7)
	for _, d := range Days {
		r := byDay[d]
		if r == nil {
			r = []model.TimeRange{}
		}
		out = append(out, model.DayHours{Day: d, Ranges: r})
	}
	return out
}

// Normalize clamps p and fixes its preset label: a named preset whose values
// no longer match becomes "custom"; an empty label is derived from the values.
func Normalize(p *model.BehaviorProfile, kind string) {
	Clamp(p)
	switch d, ok := Preset(p.Preset); {
	case p.Preset == "":
		p.Preset = MatchPreset(*p, kind)
	case ok && !Equal(*p, d.For(kind)):
		p.Preset = PresetCustom
	}
}
