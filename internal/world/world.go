// Package world is where a persona lives: its time zone, weekend, part of the
// day and daily routine (model.World). It is a leaf package (model + stdlib)
// used by the prompt (what time it is for the persona), the engine (routine
// gating, late-reply notes) and persona validation.
package world

import (
	"fmt"
	"os"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"whatsappdoppel/internal/model"
)

// Days are the routine day names, Monday first.
var Days = []string{"mon", "tue", "wed", "thu", "fri", "sat", "sun"}

// dayIndex maps a weekday to its index in Days (Monday = 0).
func dayIndex(wd time.Weekday) int { return (int(wd) + 6) % 7 }

// DayName is the routine day name of t's weekday ("mon".."sun").
func DayName(t time.Time) string { return Days[dayIndex(t.Weekday())] }

// Location is the persona's time zone: World.Timezone, else (empty or
// unknown) this Mac's zone.
func Location(w model.World) *time.Location {
	if tz := strings.TrimSpace(w.Timezone); tz != "" {
		if loc, err := time.LoadLocation(tz); err == nil {
			return loc
		}
	}
	return time.Local
}

// ZoneName is a zone's IANA name; for time.Local it is read from $TZ or the
// /etc/localtime link ("" when unknown).
func ZoneName(loc *time.Location) string {
	if loc == nil {
		return ""
	}
	if n := loc.String(); n != "Local" && n != "" {
		return n
	}
	if tz := strings.TrimPrefix(os.Getenv("TZ"), ":"); tz != "" && ValidZone(tz) {
		return tz
	}
	if target, err := os.Readlink("/etc/localtime"); err == nil {
		if _, name, ok := strings.Cut(target, "zoneinfo/"); ok && ValidZone(name) {
			return name
		}
	}
	return ""
}

// ValidZone reports whether tz is a loadable IANA zone name ("" is valid: this Mac).
func ValidZone(tz string) bool {
	if tz == "" {
		return true
	}
	if strings.EqualFold(tz, "local") {
		return false
	}
	_, err := time.LoadLocation(tz)
	return err == nil
}

// ─── weekend ─────────────────────────────────────────────────

var friSat = []string{"fri", "sat"}
var satSun = []string{"sat", "sun"}

// weekendByCountry maps lower-case country names and ISO codes to their
// weekend. Countries not listed are reported unknown (ok=false).
var weekendByCountry = map[string][]string{}

func init() {
	fs := []string{
		"il", "isr", "israel", "ישראל",
		"sa", "saudi arabia", "saudi", "ksa", "qa", "qatar", "kw", "kuwait", "bh", "bahrain", "om", "oman",
		"eg", "egypt", "jo", "jordan", "iq", "iraq", "ly", "libya", "sd", "sudan", "sy", "syria",
		"ye", "yemen", "dz", "algeria", "bd", "bangladesh",
	}
	for _, c := range fs {
		weekendByCountry[c] = friSat
	}
	ss := []string{
		"us", "usa", "united states", "united states of america", "america", "uk", "gb", "united kingdom", "england", "scotland", "wales",
		"ie", "ireland", "ca", "canada", "au", "australia", "nz", "new zealand", "de", "germany", "fr", "france", "es", "spain",
		"it", "italy", "nl", "netherlands", "holland", "be", "belgium", "ch", "switzerland", "at", "austria", "pt", "portugal",
		"se", "sweden", "no", "norway", "dk", "denmark", "fi", "finland", "pl", "poland", "cz", "czechia", "czech republic",
		"gr", "greece", "ro", "romania", "hu", "hungary", "ua", "ukraine", "ru", "russia", "tr", "turkey", "türkiye",
		"ae", "uae", "united arab emirates", "dubai", "ma", "morocco", "tn", "tunisia", "lb", "lebanon",
		"in", "india", "jp", "japan", "cn", "china", "kr", "south korea", "korea", "sg", "singapore", "th", "thailand",
		"ph", "philippines", "vn", "vietnam", "id", "indonesia", "my", "malaysia", "br", "brazil", "ar", "argentina",
		"mx", "mexico", "cl", "chile", "co", "colombia", "pe", "peru", "za", "south africa", "ng", "nigeria", "ke", "kenya",
	}
	for _, c := range ss {
		weekendByCountry[c] = satSun
	}
}

// weekendByZone guesses the weekend from a time zone when no country is set.
var weekendByZone = map[string][]string{
	"Asia/Jerusalem": friSat, "Asia/Tel_Aviv": friSat, "Israel": friSat,
	"Asia/Riyadh": friSat, "Asia/Qatar": friSat, "Asia/Kuwait": friSat, "Asia/Bahrain": friSat,
	"Asia/Muscat": friSat, "Africa/Cairo": friSat, "Egypt": friSat, "Asia/Amman": friSat,
	"Asia/Baghdad": friSat, "Africa/Tripoli": friSat, "Africa/Khartoum": friSat, "Asia/Damascus": friSat,
	"Asia/Aden": friSat, "Africa/Algiers": friSat, "Asia/Dhaka": friSat,
}

// Weekend returns the weekend days of a country (free text or ISO code).
// ok is false for an unknown or empty country.
func Weekend(country string) (days []string, ok bool) {
	c := strings.ToLower(strings.TrimSpace(country))
	c = strings.TrimPrefix(c, "the ")
	if d, ok := weekendByCountry[c]; ok {
		return slices.Clone(d), true
	}
	return nil, false
}

// WeekendFor is the weekend of a World: by country, else guessed from the
// time zone (loc's name), else Saturday–Sunday with known=false.
func WeekendFor(w model.World, loc *time.Location) (days []string, known bool) {
	if d, ok := Weekend(w.Country); ok {
		return d, true
	}
	name := strings.TrimSpace(w.Timezone)
	if name == "" && loc != nil {
		name = loc.String()
	}
	if d, ok := weekendByZone[name]; ok {
		return slices.Clone(d), true
	}
	return slices.Clone(satSun), false
}

// ─── part of the day ─────────────────────────────────────────

// PartOfDay names the time of day for an hour (0–23).
func PartOfDay(h int) string {
	switch {
	case h < 5:
		return "late night"
	case h < 8:
		return "early morning"
	case h < 12:
		return "morning"
	case h < 14:
		return "lunchtime"
	case h < 18:
		return "afternoon"
	case h < 22:
		return "evening"
	}
	return "late evening"
}

// ─── routine ─────────────────────────────────────────────────

// Active is one occurrence of a routine block: the block and when this
// occurrence starts and ends (in the persona's zone).
type Active struct {
	Block      model.RoutineBlock
	Start, End time.Time
}

// ParseHM parses "HH:MM" into minutes after midnight ("24:00" only when end).
func ParseHM(s string, end bool) (int, bool) {
	h, m, ok := strings.Cut(strings.TrimSpace(s), ":")
	if !ok || len(m) != 2 || h == "" || len(h) > 2 {
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

// occurrences lists the routine's block occurrences that start between
// from-1 day and to (in loc), sorted by start.
func occurrences(w model.World, loc *time.Location, from, to time.Time) []Active {
	if len(w.Routine) == 0 || to.Before(from) {
		return nil
	}
	lf := from.In(loc)
	y, m, d := lf.Date()
	days := int(to.Sub(from)/(24*time.Hour)) + 2
	var out []Active
	for off := -1; off <= days; off++ {
		day := time.Date(y, m, d+off, 0, 0, 0, 0, loc)
		name := DayName(day)
		for _, b := range w.Routine {
			if len(b.Days) > 0 && !slices.Contains(b.Days, name) {
				continue
			}
			f, ok1 := ParseHM(b.From, false)
			t, ok2 := ParseHM(b.To, true)
			if !ok1 || !ok2 || f == t {
				continue
			}
			start := time.Date(y, m, d+off, f/60, f%60, 0, 0, loc)
			endDay := d + off
			if t <= f {
				endDay++
			}
			end := time.Date(y, m, endDay, t/60, t%60, 0, 0, loc)
			if !end.After(start) {
				continue
			}
			out = append(out, Active{Block: b, Start: start, End: end})
		}
	}
	slices.SortStableFunc(out, func(a, b Active) int { return a.Start.Compare(b.Start) })
	return out
}

// reachRank orders reaches from most to least reachable.
func reachRank(r string) int {
	switch r {
	case model.ReachUnreachable:
		return 2
	case model.ReachSlow:
		return 1
	}
	return 0
}

// RoutineAt returns the routine block the persona is in at now (the least
// reachable one when blocks overlap).
func RoutineAt(w model.World, now time.Time) (Active, bool) {
	loc := Location(w)
	var best Active
	found := false
	for _, a := range occurrences(w, loc, now, now) {
		if now.Before(a.Start) || !now.Before(a.End) {
			continue
		}
		if !found || reachRank(a.Block.Reach) > reachRank(best.Block.Reach) {
			best, found = a, true
		}
	}
	return best, found
}

// RoutineBetween lists the block occurrences that overlap [from, to).
func RoutineBetween(w model.World, from, to time.Time) []Active {
	var out []Active
	for _, a := range occurrences(w, Location(w), from, to) {
		if a.Start.Before(to) && a.End.After(from) {
			out = append(out, a)
		}
	}
	return out
}

// JustFinished returns the block that ended most recently within the last
// `within` before now (and is not running again at now).
func JustFinished(w model.World, now time.Time, within time.Duration) (Active, bool) {
	var best Active
	found := false
	for _, a := range RoutineBetween(w, now.Add(-within), now) {
		if a.End.After(now) {
			continue
		}
		if !found || a.End.After(best.End) {
			best, found = a, true
		}
	}
	return best, found
}

// Unreachable returns the unreachable block at now, if any (the persona is
// off its phone until End).
func Unreachable(w model.World, now time.Time) (Active, bool) {
	a, ok := RoutineAt(w, now)
	if !ok || a.Block.Reach != model.ReachUnreachable {
		return Active{}, false
	}
	// Chained unreachable blocks (work → commute) end at the last one.
	for i := 0; i < model.MaxRoutineBlocks; i++ {
		next, ok := RoutineAt(w, a.End)
		if !ok || next.Block.Reach != model.ReachUnreachable || !next.End.After(a.End) {
			break
		}
		a.End = next.End
	}
	return a, true
}

// ─── normalisation ───────────────────────────────────────────

// Normalize trims and validates a World in place: unknown zones are cleared
// (this Mac), invalid blocks dropped, labels/days cleaned, reach defaulted to
// normal, at most MaxRoutineBlocks blocks, and Routine is never nil.
func Normalize(w *model.World) {
	w.City = clip(w.City, 80)
	w.Country = clip(w.Country, 80)
	w.Timezone = strings.TrimSpace(w.Timezone)
	if !ValidZone(w.Timezone) {
		w.Timezone = ""
	}
	out := make([]model.RoutineBlock, 0, len(w.Routine))
	for _, b := range w.Routine {
		if len(out) == model.MaxRoutineBlocks {
			break
		}
		if nb, ok := normalizeBlock(b); ok {
			out = append(out, nb)
		}
	}
	w.Routine = out
}

// Validate reports the first problem with a World (used by the API to give
// a clear error instead of silently dropping input).
func Validate(w model.World) error {
	if !ValidZone(strings.TrimSpace(w.Timezone)) {
		return fmt.Errorf("world.timezone %q is not a known time zone", w.Timezone)
	}
	if len(w.Routine) > model.MaxRoutineBlocks {
		return fmt.Errorf("world.routine has %d blocks (at most %d)", len(w.Routine), model.MaxRoutineBlocks)
	}
	for i, b := range w.Routine {
		if strings.TrimSpace(b.Label) == "" {
			return fmt.Errorf("world.routine[%d]: label is required", i)
		}
		f, ok1 := ParseHM(b.From, false)
		t, ok2 := ParseHM(b.To, true)
		if !ok1 || !ok2 {
			return fmt.Errorf("world.routine[%d]: times must be HH:MM", i)
		}
		if f == t {
			return fmt.Errorf("world.routine[%d]: from and to are the same time", i)
		}
		for _, d := range b.Days {
			if !slices.Contains(Days, strings.ToLower(strings.TrimSpace(d))) {
				return fmt.Errorf("world.routine[%d]: unknown day %q", i, d)
			}
		}
		if r := strings.TrimSpace(b.Reach); r != "" && !slices.Contains(model.Reaches, strings.ToLower(r)) {
			return fmt.Errorf("world.routine[%d]: reach must be normal, slow or unreachable", i)
		}
	}
	return nil
}

func normalizeBlock(b model.RoutineBlock) (model.RoutineBlock, bool) {
	b.Label = clip(b.Label, model.MaxRoutineLabel)
	if b.Label == "" {
		return b, false
	}
	f, ok1 := ParseHM(b.From, false)
	t, ok2 := ParseHM(b.To, true)
	if !ok1 || !ok2 || f == t {
		return b, false
	}
	b.From, b.To = fmtHM(f), fmtHM(t)
	days := make([]string, 0, len(b.Days))
	for _, d := range b.Days {
		d = strings.ToLower(strings.TrimSpace(d))
		if len(d) > 3 {
			d = d[:3]
		}
		if slices.Contains(Days, d) && !slices.Contains(days, d) {
			days = append(days, d)
		}
	}
	slices.SortFunc(days, func(a, b string) int { return slices.Index(Days, a) - slices.Index(Days, b) })
	if len(days) == len(Days) {
		days = []string{}
	}
	b.Days = days
	b.Reach = strings.ToLower(strings.TrimSpace(b.Reach))
	if !slices.Contains(model.Reaches, b.Reach) {
		b.Reach = model.ReachNormal
	}
	return b, true
}

func fmtHM(m int) string { return fmt.Sprintf("%02d:%02d", m/60, m%60) }

// clip trims s to one line of at most n runes.
func clip(s string, n int) string {
	s = strings.Join(strings.Fields(s), " ")
	if utf8.RuneCountInString(s) > n {
		s = strings.TrimSpace(string([]rune(s)[:n]))
	}
	return s
}

// ─── describing ──────────────────────────────────────────────

// Place is where the persona lives in words ("Tel Aviv", "Tel Aviv, Israel",
// "Israel"); "" when nothing is set.
func Place(w model.World) string {
	switch {
	case w.City != "" && w.Country != "" && !strings.EqualFold(w.City, w.Country):
		return w.City + ", " + w.Country
	case w.City != "":
		return w.City
	}
	return w.Country
}

// FmtDays renders a block's days in words: "every day", "weekdays",
// "Mon, Wed, Fri".
func FmtDays(days []string) string {
	switch {
	case len(days) == 0 || len(days) == 7:
		return "every day"
	case slices.Equal(days, []string{"mon", "tue", "wed", "thu", "fri"}):
		return "Mon–Fri"
	case slices.Equal(days, []string{"sun", "mon", "tue", "wed", "thu"}) || slices.Equal(days, []string{"mon", "tue", "wed", "thu", "sun"}):
		return "Sun–Thu"
	case slices.Equal(days, []string{"sat", "sun"}):
		return "weekends"
	}
	out := make([]string, len(days))
	for i, d := range days {
		out[i] = strings.ToUpper(d[:1]) + d[1:]
	}
	return strings.Join(out, ", ")
}

// FmtDayRange renders weekend days as "Friday–Saturday".
func FmtDayRange(days []string) string {
	full := map[string]string{"mon": "Monday", "tue": "Tuesday", "wed": "Wednesday", "thu": "Thursday", "fri": "Friday", "sat": "Saturday", "sun": "Sunday"}
	out := make([]string, 0, len(days))
	for _, d := range days {
		out = append(out, full[d])
	}
	return strings.Join(out, "–")
}
