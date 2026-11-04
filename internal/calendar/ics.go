// Package calendar reads the owner's calendars for the personal secretary,
// read-only: iCal (.ics) links — Google Calendar's "secret address in iCal
// format", Outlook's published calendar link, iCloud public calendars, any
// .ics feed — and the MACalendar app's HTTP API. Only busy times leave this
// package (Busy, FreeSlots); titles are kept for the owner's own views. It
// imports only model and the stdlib.
package calendar

import (
	"bufio"
	"io"
	"slices"
	"strconv"
	"strings"
	"time"
)

// Event is one occurrence of a calendar event.
type Event struct {
	Start, End time.Time
	AllDay     bool
	Title      string
}

// maxOccurrences caps how many instances one repeating event may expand to.
const maxOccurrences = 3000

type vevent struct {
	uid, summary, rrule  string
	start, end           time.Time
	allDay, hasEnd       bool
	dur                  time.Duration
	exdates              []time.Time
	recurrenceID         time.Time
	cancelled, available bool
}

// ParseICS returns the events of an iCal feed that overlap [from, to),
// repeating events expanded. Times without a zone are read in loc.
// Cancelled and "free" (TRANSP:TRANSPARENT) events are left out.
func ParseICS(r io.Reader, from, to time.Time, loc *time.Location) ([]Event, error) {
	if loc == nil {
		loc = time.Local
	}
	lines, err := unfold(r)
	if err != nil {
		return nil, err
	}
	var all []vevent
	var cur *vevent
	for _, ln := range lines {
		name, params, value := splitProp(ln)
		switch {
		case name == "BEGIN" && strings.EqualFold(value, "VEVENT"):
			cur = &vevent{}
		case name == "END" && strings.EqualFold(value, "VEVENT"):
			if cur != nil && !cur.start.IsZero() {
				all = append(all, *cur)
			}
			cur = nil
		case cur == nil:
		case name == "UID":
			cur.uid = value
		case name == "SUMMARY":
			cur.summary = unescape(value)
		case name == "DTSTART":
			cur.start, cur.allDay = parseTime(value, params, loc)
		case name == "DTEND":
			cur.end, _ = parseTime(value, params, loc)
			cur.hasEnd = !cur.end.IsZero()
		case name == "DURATION":
			cur.dur = parseDuration(value)
		case name == "RRULE":
			cur.rrule = value
		case name == "EXDATE":
			for _, v := range strings.Split(value, ",") {
				if t, _ := parseTime(v, params, loc); !t.IsZero() {
					cur.exdates = append(cur.exdates, t)
				}
			}
		case name == "RECURRENCE-ID":
			cur.recurrenceID, _ = parseTime(value, params, loc)
		case name == "STATUS":
			cur.cancelled = strings.EqualFold(value, "CANCELLED")
		case name == "TRANSP":
			cur.available = strings.EqualFold(value, "TRANSPARENT")
		}
	}
	// Moved or cancelled single occurrences replace the series' instance.
	moved := map[string][]time.Time{}
	for _, v := range all {
		if !v.recurrenceID.IsZero() {
			moved[v.uid] = append(moved[v.uid], v.recurrenceID)
		}
	}
	var out []Event
	for _, v := range all {
		if v.cancelled || v.available {
			continue
		}
		length := v.dur
		switch {
		case v.hasEnd:
			length = v.end.Sub(v.start)
		case length == 0 && v.allDay:
			length = 24 * time.Hour
		case length == 0:
			length = time.Hour
		}
		emit := func(s time.Time) {
			s = s.In(loc)
			e := s.Add(length)
			if v.allDay { // whole days, also across DST
				days := max(1, int(length.Round(24*time.Hour)/(24*time.Hour)))
				e = time.Date(s.Year(), s.Month(), s.Day()+days, 0, 0, 0, 0, s.Location())
			}
			if e.After(from) && s.Before(to) {
				out = append(out, Event{Start: s, End: e, AllDay: v.allDay, Title: v.summary})
			}
		}
		if v.rrule == "" || !v.recurrenceID.IsZero() {
			emit(v.start)
			continue
		}
		skip := append(slices.Clone(v.exdates), moved[v.uid]...)
		for _, s := range expand(v.start, v.rrule, to) {
			if !slices.ContainsFunc(skip, func(x time.Time) bool { return x.Equal(s) }) {
				emit(s)
			}
		}
	}
	slices.SortFunc(out, func(a, b Event) int { return a.Start.Compare(b.Start) })
	return out, nil
}

// unfold reads content lines, joining continuation lines (RFC 5545 §3.1).
func unfold(r io.Reader) ([]string, error) {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 64*1024), 4*1024*1024)
	var out []string
	for sc.Scan() {
		ln := strings.TrimRight(sc.Text(), "\r")
		if (strings.HasPrefix(ln, " ") || strings.HasPrefix(ln, "\t")) && len(out) > 0 {
			out[len(out)-1] += ln[1:]
			continue
		}
		out = append(out, ln)
	}
	return out, sc.Err()
}

// splitProp splits "NAME;PARAM=V;…:value" (quoted parameter values may hold ':').
func splitProp(ln string) (name string, params map[string]string, value string) {
	inQuote, colon := false, -1
	for i, r := range ln {
		if r == '"' {
			inQuote = !inQuote
		}
		if r == ':' && !inQuote {
			colon = i
			break
		}
	}
	if colon < 0 {
		return strings.ToUpper(ln), nil, ""
	}
	head, value := ln[:colon], ln[colon+1:]
	parts := strings.Split(head, ";")
	name = strings.ToUpper(parts[0])
	params = map[string]string{}
	for _, p := range parts[1:] {
		if k, v, ok := strings.Cut(p, "="); ok {
			params[strings.ToUpper(k)] = strings.Trim(v, `"`)
		}
	}
	return name, params, value
}

func unescape(s string) string {
	return strings.NewReplacer(`\n`, " ", `\N`, " ", `\,`, ",", `\;`, ";", `\\`, `\`).Replace(s)
}

// windowsZones maps common Windows zone names (Outlook feeds) to IANA ones.
var windowsZones = map[string]string{
	"Israel Standard Time": "Asia/Jerusalem", "GMT Standard Time": "Europe/London", "W. Europe Standard Time": "Europe/Berlin",
	"Romance Standard Time": "Europe/Paris", "Central Europe Standard Time": "Europe/Budapest",
	"Eastern Standard Time": "America/New_York", "Central Standard Time": "America/Chicago",
	"Mountain Standard Time": "America/Denver", "Pacific Standard Time": "America/Los_Angeles", "UTC": "UTC",
}

// parseTime reads an iCal DATE or DATE-TIME (Z = UTC, TZID, else loc).
func parseTime(v string, params map[string]string, loc *time.Location) (time.Time, bool) {
	v = strings.TrimSpace(v)
	zone := loc
	if tz := params["TZID"]; tz != "" {
		if iana, ok := windowsZones[tz]; ok {
			tz = iana
		}
		if l, err := time.LoadLocation(tz); err == nil {
			zone = l
		}
	}
	if params["VALUE"] == "DATE" || len(v) == 8 {
		t, err := time.ParseInLocation("20060102", v, loc)
		if err != nil {
			return time.Time{}, false
		}
		return t, true
	}
	if strings.HasSuffix(v, "Z") {
		t, err := time.Parse("20060102T150405Z", v)
		if err != nil {
			return time.Time{}, false
		}
		return t, false // repeats in UTC (RFC 5545); shown in loc by ParseICS
	}
	t, err := time.ParseInLocation("20060102T150405", v, zone)
	if err != nil {
		return time.Time{}, false
	}
	return t, false // in its own zone, so repeats keep its wall-clock time
}

// parseDuration reads an iCal DURATION (P1D, PT1H30M, P1W…).
func parseDuration(v string) time.Duration {
	v = strings.TrimPrefix(strings.TrimPrefix(strings.ToUpper(strings.TrimSpace(v)), "+"), "P")
	var d time.Duration
	num := ""
	inTime := false
	for _, r := range v {
		switch {
		case r >= '0' && r <= '9':
			num += string(r)
		case r == 'T':
			inTime = true
		default:
			n, _ := strconv.Atoi(num)
			num = ""
			switch {
			case r == 'W':
				d += time.Duration(n) * 7 * 24 * time.Hour
			case r == 'D':
				d += time.Duration(n) * 24 * time.Hour
			case r == 'H' && inTime:
				d += time.Duration(n) * time.Hour
			case r == 'M' && inTime:
				d += time.Duration(n) * time.Minute
			case r == 'S' && inTime:
				d += time.Duration(n) * time.Second
			}
		}
	}
	return d
}

var weekdays = map[string]time.Weekday{"SU": time.Sunday, "MO": time.Monday, "TU": time.Tuesday, "WE": time.Wednesday,
	"TH": time.Thursday, "FR": time.Friday, "SA": time.Saturday}

// expand lists the starts of a repeating event up to (not after) to.
// Supported: FREQ DAILY/WEEKLY/MONTHLY/YEARLY with INTERVAL, COUNT, UNTIL,
// BYDAY (weekly days; monthly "2MO"/"-1FR") and BYMONTHDAY.
func expand(start time.Time, rule string, to time.Time) []time.Time {
	r := map[string]string{}
	for _, p := range strings.Split(rule, ";") {
		if k, v, ok := strings.Cut(p, "="); ok {
			r[strings.ToUpper(k)] = strings.ToUpper(v)
		}
	}
	interval := max(1, atoi(r["INTERVAL"]))
	count := atoi(r["COUNT"])
	var until time.Time
	if u := r["UNTIL"]; u != "" {
		until, _ = parseTime(u, nil, start.Location())
		if len(u) == 8 { // a date: the whole day counts
			until = until.Add(24*time.Hour - time.Second)
		}
	}
	h, m, s := start.Clock()
	at := func(y int, mo time.Month, d int) time.Time { return time.Date(y, mo, d, h, m, s, 0, start.Location()) }
	var out []time.Time
	n := 0
	add := func(t time.Time) bool { // false = stop
		if t.Before(start) {
			return true
		}
		if (!until.IsZero() && t.After(until)) || t.After(to) || (count > 0 && n >= count) || n >= maxOccurrences {
			return false
		}
		n++
		out = append(out, t)
		return true
	}
	byday := strings.Split(r["BYDAY"], ",")
	if r["BYDAY"] == "" {
		byday = nil
	}
	switch r["FREQ"] {
	case "DAILY":
		for k := 0; ; k += interval {
			if !add(at(start.Year(), start.Month(), start.Day()+k)) {
				return out
			}
		}
	case "WEEKLY":
		days := []time.Weekday{start.Weekday()}
		if byday != nil {
			days = days[:0]
			for _, d := range byday {
				if wd, ok := weekdays[d[max(0, len(d)-2):]]; ok {
					days = append(days, wd)
				}
			}
		}
		// Weeks start on Monday (WKST default).
		monday := at(start.Year(), start.Month(), start.Day()-(int(start.Weekday())+6)%7)
		for w := 0; ; w += interval {
			var week []time.Time
			for _, wd := range days {
				week = append(week, at(monday.Year(), monday.Month(), monday.Day()+7*w+(int(wd)+6)%7))
			}
			slices.SortFunc(week, func(a, b time.Time) int { return a.Compare(b) })
			for _, t := range week {
				if !add(t) {
					return out
				}
			}
		}
	case "MONTHLY":
		for k := 0; ; k += interval {
			first := at(start.Year(), start.Month()+time.Month(k), 1)
			var month []time.Time
			switch {
			case r["BYMONTHDAY"] != "":
				for _, ds := range strings.Split(r["BYMONTHDAY"], ",") {
					if d := atoi(ds); d > 0 && d <= daysIn(first) {
						month = append(month, at(first.Year(), first.Month(), d))
					}
				}
			case byday != nil:
				for _, bd := range byday {
					if t, ok := nthWeekday(first, bd, at); ok {
						month = append(month, t)
					}
				}
			default:
				if start.Day() <= daysIn(first) {
					month = append(month, at(first.Year(), first.Month(), start.Day()))
				}
			}
			slices.SortFunc(month, func(a, b time.Time) int { return a.Compare(b) })
			for _, t := range month {
				if !add(t) {
					return out
				}
			}
			if first.After(to) || k > 12*maxOccurrences {
				return out
			}
		}
	case "YEARLY":
		for k := 0; ; k += interval {
			t := at(start.Year()+k, start.Month(), start.Day())
			if t.Day() != start.Day() { // 29 Feb in a common year
				if t.After(to) {
					return out
				}
				continue
			}
			if !add(t) {
				return out
			}
		}
	}
	return []time.Time{start} // unknown rule: just the first one
}

// nthWeekday reads a monthly BYDAY like "2MO" (second Monday) or "-1FR"
// (last Friday) for the month of first; a bare "MO" means the first one.
func nthWeekday(first time.Time, bd string, at func(int, time.Month, int) time.Time) (time.Time, bool) {
	if len(bd) < 2 {
		return time.Time{}, false
	}
	wd, ok := weekdays[bd[len(bd)-2:]]
	if !ok {
		return time.Time{}, false
	}
	nth := 1
	if p := bd[:len(bd)-2]; p != "" {
		nth = atoi(p)
	}
	days := daysIn(first)
	var hits []int
	for d := 1; d <= days; d++ {
		if time.Date(first.Year(), first.Month(), d, 12, 0, 0, 0, first.Location()).Weekday() == wd {
			hits = append(hits, d)
		}
	}
	i := nth - 1
	if nth < 0 {
		i = len(hits) + nth
	}
	if i < 0 || i >= len(hits) {
		return time.Time{}, false
	}
	return at(first.Year(), first.Month(), hits[i]), true
}

func daysIn(t time.Time) int {
	return time.Date(t.Year(), t.Month()+1, 0, 12, 0, 0, 0, t.Location()).Day()
}

func atoi(s string) int {
	n, _ := strconv.Atoi(strings.TrimSpace(s))
	return n
}
