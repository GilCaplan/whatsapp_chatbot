package task

import (
	"cmp"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode"
)

// LeakInput is what a draft is checked against. Titles are your calendar's
// event titles: they stay in code (only used for this check, never sent to
// a model, never written into a problem).
type LeakInput struct {
	Reply   string
	Allowed []time.Time // times it may mention (offers, theirs, the agreed one)
	Days    []time.Time // days it may mention without a time (parts of days, the day they named)
	Loc     *time.Location
	Titles  []string
	Exempt  []string // text whose words are fine (the task's own subject, place, your cover lines)
	Reasons bool     // also block reasons for being busy ("I have a meeting then")
}

var (
	reClock   = regexp.MustCompile(`\b([01]?\d|2[0-3]):([0-5]\d)\b`)
	reAmPm    = regexp.MustCompile(`(?i)\b(1[0-2]|0?[1-9])(?::([0-5]\d))?\s*([ap])\.?m\.?\b`)
	reAt      = regexp.MustCompile(`(?i)\b(?:at|around|by)\s+([01]?\d|2[0-3])\b(?:\s*(?:o'?clock))?`)
	reHeAt    = regexp.MustCompile(`בשעה\s*([01]?\d|2[0-3])\b`)
	reDayMon  = regexp.MustCompile(`(?i)\b([0-3]?\d)(?:st|nd|rd|th)?\s+(?:of\s+)?(jan|feb|mar|apr|may|jun|jul|aug|sep|oct|nov|dec)[a-z]*\b`)
	reMonDay  = regexp.MustCompile(`(?i)\b(jan|feb|mar|apr|may|jun|jul|aug|sep|oct|nov|dec)[a-z]*\.?\s+([0-3]?\d)(?:st|nd|rd|th)?\b`)
	reNumDate = regexp.MustCompile(`\b([0-3]?\d)[./]([01]?\d)\b`)
)

var months = map[string]time.Month{"jan": 1, "feb": 2, "mar": 3, "apr": 4, "may": 5, "jun": 6, "jul": 7, "aug": 8, "sep": 9, "oct": 10, "nov": 11, "dec": 12}

var enDays = map[string]time.Weekday{"sunday": 0, "monday": 1, "tuesday": 2, "wednesday": 3, "thursday": 4, "friday": 5, "saturday": 6,
	"sun": 0, "mon": 1, "tue": 2, "tues": 2, "wed": 3, "thu": 4, "thur": 4, "thurs": 4, "fri": 5, "sat": 6}

var heDayNames = map[string]time.Weekday{"יום ראשון": 0, "יום שני": 1, "יום שלישי": 2, "יום רביעי": 3, "יום חמישי": 4, "יום שישי": 5, "שבת": 6,
	"ביום ראשון": 0, "ביום שני": 1, "ביום שלישי": 2, "ביום רביעי": 3, "ביום חמישי": 4, "ביום שישי": 5, "בשבת": 6}

// reasonWords give away why a time doesn't work (English + Hebrew).
var reasonWords = []string{
	"another meeting", "have a meeting", "got a meeting", "in a meeting", "meeting then", "a call then", "on a call",
	"doctor", "dentist", "appointment then", "flight", "flying", "my calendar", "the calendar", "calendar says", "calendar shows",
	"my schedule", "busy with", "booked", "taken by", "i'm at", "i am at", "i'll be at", "i will be at",
	"פגישה אחרת", "יש לי פגישה", "בפגישה", "רופא", "רופאה", "תור ל", "טיסה", "ביומן", "היומן", "עסוק ב", "עסוקה ב", "תפוס ב", "תפוסה ב",
}

// stopWords never count as calendar words (too common).
var stopWords = map[string]bool{
	"with": true, "from": true, "this": true, "that": true, "have": true, "will": true, "your": true, "about": true,
	"meet": true, "meeting": true, "call": true, "time": true, "week": true, "today": true, "tomorrow": true, "morning": true,
	"afternoon": true, "evening": true, "lunch": true, "dinner": true, "coffee": true, "work": true, "sure": true, "free": true,
	"then": true, "there": true, "here": true, "what": true, "when": true, "works": true, "good": true, "great": true,
}

// Leak lists what is wrong with a draft (plain English, never a calendar
// word): times or days it may not mention, words from your calendar, and
// (Reasons) why a time doesn't work.
func Leak(in LeakInput) []string {
	loc := in.Loc
	if loc == nil {
		loc = time.Local
	}
	text := in.Reply
	low := strings.ToLower(text)
	var out []string
	add := func(s string) {
		if !slices.Contains(out, s) {
			out = append(out, s)
		}
	}
	type hm struct{ h, m int }
	hours := map[hm]bool{}
	days := map[time.Weekday]bool{}
	type md struct {
		m time.Month
		d int
	}
	dates := map[md]bool{}
	dom := map[int]bool{}
	for _, t := range in.Allowed {
		t = t.In(loc)
		hours[hm{t.Hour(), t.Minute()}] = true
		days[t.Weekday()] = true
		dates[md{t.Month(), t.Day()}] = true
		dom[t.Day()] = true
	}
	for _, t := range in.Days {
		t = t.In(loc)
		days[t.Weekday()] = true
		dates[md{t.Month(), t.Day()}] = true
		dom[t.Day()] = true
	}
	hourOK := func(h, m int, ampm string) bool {
		switch ampm {
		case "a":
			if h == 12 {
				h = 0
			}
			return hours[hm{h, m}]
		case "p":
			if h < 12 {
				h += 12
			}
			return hours[hm{h, m}]
		}
		return hours[hm{h, m}] || (h < 12 && hours[hm{h + 12, m}])
	}
	anyMinute := func(h int) bool {
		for k := range hours {
			if k.h == h || (h < 12 && k.h == h+12) {
				return true
			}
		}
		return false
	}
	// Times.
	masked := text
	for _, mm := range reAmPm.FindAllStringSubmatch(text, -1) {
		h, _ := strconv.Atoi(mm[1])
		m, _ := strconv.Atoi(cmp.Or(mm[2], "0"))
		if !hourOK(h, m, strings.ToLower(mm[3])) {
			add("mentions a time it may not offer (" + strings.TrimSpace(mm[0]) + ")")
		}
	}
	masked = reAmPm.ReplaceAllString(masked, " ")
	for _, mm := range reClock.FindAllStringSubmatch(masked, -1) {
		h, _ := strconv.Atoi(mm[1])
		m, _ := strconv.Atoi(mm[2])
		if !hourOK(h, m, "") {
			add("mentions a time it may not offer (" + mm[0] + ")")
		}
	}
	masked = reClock.ReplaceAllString(masked, " ")
	for _, re := range []*regexp.Regexp{reAt, reHeAt} {
		for _, mm := range re.FindAllStringSubmatch(masked, -1) {
			if h, _ := strconv.Atoi(mm[1]); !anyMinute(h) {
				add("mentions a time it may not offer (" + strings.TrimSpace(mm[0]) + ")")
			}
		}
	}
	// Dates (in the text without its clock times: "Nov 15:00" is no "Nov 15").
	dlow := strings.ToLower(masked)
	for _, mm := range reDayMon.FindAllStringSubmatch(dlow, -1) {
		d, _ := strconv.Atoi(mm[1])
		if !dates[md{months[mm[2][:3]], d}] {
			add("mentions a date it may not offer (" + mm[0] + ")")
		}
	}
	for _, mm := range reMonDay.FindAllStringSubmatch(dlow, -1) {
		d, _ := strconv.Atoi(mm[2])
		if !dates[md{months[mm[1][:3]], d}] {
			add("mentions a date it may not offer (" + mm[0] + ")")
		}
	}
	for _, mm := range reNumDate.FindAllStringSubmatch(masked, -1) {
		a, _ := strconv.Atoi(mm[1])
		b, _ := strconv.Atoi(mm[2])
		okDM := dates[md{time.Month(b), a}]
		okMD := dates[md{time.Month(a), b}]
		validDM := a >= 1 && a <= 31 && b >= 1 && b <= 12
		validMD := a >= 1 && a <= 12 && b >= 1 && b <= 31
		if !okDM && !okMD && (validDM || validMD) && !hourOK(a, b, "") { // "10.30" may be a time
			add("mentions a date it may not offer (" + mm[0] + ")")
		}
	}
	// Days.
	for _, w := range words(low) {
		if d, ok := enDays[w]; ok && !days[d] {
			add("mentions a day it may not offer (" + w + ")")
		}
	}
	for name, d := range heDayNames {
		if strings.Contains(text, name) && !days[d] {
			add("mentions a day it may not offer")
			break
		}
	}
	// Calendar words.
	exempt := map[string]bool{}
	for _, e := range in.Exempt {
		for _, w := range words(strings.ToLower(e)) {
			exempt[w] = true
		}
	}
	replyWords := map[string]bool{}
	for _, w := range words(low) {
		replyWords[w] = true
	}
	for _, title := range in.Titles {
		tl := strings.ToLower(strings.TrimSpace(title))
		if tl == "" {
			continue
		}
		if len([]rune(tl)) >= 6 && strings.Contains(low, tl) && !exempt[tl] {
			add("uses words from your calendar")
			break
		}
		hit := false
		for _, w := range words(tl) {
			if len([]rune(w)) >= 4 && !stopWords[w] && !exempt[w] && replyWords[w] {
				if _, isDay := enDays[w]; !isDay {
					hit = true
					break
				}
			}
		}
		if hit {
			add("uses words from your calendar")
			break
		}
	}
	// Reasons.
	if in.Reasons {
		exemptText := strings.ToLower(strings.Join(in.Exempt, "\n"))
		for _, r := range reasonWords {
			if strings.Contains(low, r) && !strings.Contains(exemptText, r) {
				add("says why a time doesn't work")
				break
			}
		}
	}
	return out
}

func words(s string) []string {
	return strings.FieldsFunc(s, func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) && r != '\'' })
}
