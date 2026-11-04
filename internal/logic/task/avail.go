// Package task runs tasks (model.TaskSpec): errands with a clear result,
// like agreeing on a meeting time. It holds the parts code decides on, so
// they can be tested without a model: templates and spec checks, the rule
// list, which times to offer and whether a time works (the calendar stays
// here: an LLM only ever sees the times it may say), and the leak check on
// every draft. Leaf package: imports model, calendar (intervals) + stdlib.
package task

import (
	"slices"
	"strings"
	"time"

	"whatsappdoppel/internal/core/model"
	"whatsappdoppel/internal/logic/calendar"
)

// Picker chooses among n options (behavior.Sampler fits; tests pass a fixed one).
type Picker interface {
	Index(n int) int
}

// Grid is the step of every time a task offers: round times only.
const Grid = 30 * time.Minute

// Availability is when a meeting can happen: your meeting days and hours,
// the horizon, and your calendars' busy times (never their titles).
type Availability struct {
	Busy     []calendar.Interval
	Known    bool // the calendars could be read (or none are used)
	Days     []time.Weekday
	From, To string // "09:00", "18:00"
	Length   time.Duration
	Buffer   time.Duration // kept free around busy times when offering
	Start    time.Time     // horizon
	End      time.Time
	Loc      *time.Location
}

func (a Availability) loc() *time.Location {
	if a.Loc == nil {
		return time.Local
	}
	return a.Loc
}

// window is the meeting window on t's day (ok=false: not a meeting day).
func (a Availability) window(t time.Time) (time.Time, time.Time, bool) {
	t = t.In(a.loc())
	if len(a.Days) > 0 && !slices.Contains(a.Days, t.Weekday()) {
		return time.Time{}, time.Time{}, false
	}
	f, errF := time.Parse("15:04", a.From)
	e, errE := time.Parse("15:04", a.To)
	if errF != nil || errE != nil || !e.After(f) {
		return time.Time{}, time.Time{}, false
	}
	day := func(c time.Time) time.Time {
		return time.Date(t.Year(), t.Month(), t.Day(), c.Hour(), c.Minute(), 0, 0, a.loc())
	}
	return day(f), day(e), true
}

// clear reports whether [s, e) overlaps no busy time.
func (a Availability) clear(s, e time.Time) bool {
	for _, b := range a.Busy {
		if b.Start.Before(e) && b.End.After(s) {
			return false
		}
	}
	return true
}

// Works reports whether a meeting can start at t: a meeting day, inside the
// hours and the horizon, and free.
func (a Availability) Works(t time.Time) bool {
	if !a.Known || a.Length <= 0 || t.Before(a.Start) || !t.Before(a.End) {
		return false
	}
	ws, we, ok := a.window(t)
	end := t.Add(a.Length)
	return ok && !t.Before(ws) && !end.After(we) && a.clear(t, end)
}

// Candidates are the times a task may offer, earliest first: on the grid,
// inside the hours, with the buffer kept free around busy times.
func (a Availability) Candidates() []time.Time {
	if !a.Known || a.Length <= 0 || !a.End.After(a.Start) {
		return nil
	}
	var out []time.Time
	loc := a.loc()
	s := a.Start.In(loc)
	for d := time.Date(s.Year(), s.Month(), s.Day(), 0, 0, 0, 0, loc); d.Before(a.End); d = d.AddDate(0, 0, 1) {
		ws, we, ok := a.window(d)
		if !ok {
			continue
		}
		for t := ws; !t.Add(a.Length).After(we); t = t.Add(Grid) {
			if t.Before(a.Start) || !t.Before(a.End) {
				continue
			}
			if a.clear(t.Add(-a.Buffer), t.Add(a.Length+a.Buffer)) {
				out = append(out, t)
			}
		}
	}
	return out
}

// Part is the part of day of t: morning, afternoon or evening.
func Part(t time.Time) string {
	switch h := t.Hour(); {
	case h < 12:
		return "morning"
	case h < 17:
		return "afternoon"
	}
	return "evening"
}

func sameDay(a, b time.Time) bool {
	ay, am, ad := a.Date()
	by, bm, bd := b.Date()
	return ay == by && am == bm && ad == bd
}

func without(ts, avoid []time.Time) []time.Time {
	var out []time.Time
	for _, t := range ts {
		if !slices.ContainsFunc(avoid, t.Equal) {
			out = append(out, t)
		}
	}
	return out
}

// OfferOpts narrow an offer.
type OfferOpts struct {
	Avoid  []time.Time // already offered or turned down
	Prefer string      // part of day ("" = any)
	Day    *time.Time  // only this day
	Part   string      // only this part of that day ("" = any)
}

// Offer picks up to n times: on different days where it can (the soonest
// days first), a random time within each day, the preferred part of day
// when there is one. Never the same time twice.
func (a Availability) Offer(n int, o OfferOpts, pick Picker) []time.Time {
	cands := without(a.Candidates(), o.Avoid)
	if o.Day != nil {
		day := o.Day.In(a.loc())
		cands = slices.DeleteFunc(cands, func(t time.Time) bool {
			return !sameDay(t.In(a.loc()), day) || (o.Part != "" && Part(t.In(a.loc())) != o.Part)
		})
	}
	if o.Prefer != "" {
		if pref := slices.DeleteFunc(slices.Clone(cands), func(t time.Time) bool { return Part(t.In(a.loc())) != o.Prefer }); len(pref) > 0 {
			cands = pref
		}
	}
	if n <= 0 || len(cands) == 0 {
		return nil
	}
	// Group by day, soonest first.
	var days [][]time.Time
	for _, t := range cands {
		if k := len(days); k > 0 && sameDay(days[k-1][0].In(a.loc()), t.In(a.loc())) {
			days[k-1] = append(days[k-1], t)
		} else {
			days = append(days, []time.Time{t})
		}
	}
	var out []time.Time
	for round := 0; len(out) < n && round < n; round++ {
		for i := range days {
			if len(out) >= n {
				break
			}
			left := slices.DeleteFunc(slices.Clone(days[i]), func(t time.Time) bool {
				return slices.ContainsFunc(out, func(o time.Time) bool { return absDur(o.Sub(t)) < 2*time.Hour })
			})
			if len(left) == 0 {
				continue
			}
			out = append(out, left[index(pick, len(left))])
		}
	}
	slices.SortFunc(out, func(x, y time.Time) int { return x.Compare(y) })
	return out
}

// Alternative is the nearest time on t's day the task may offer, else one
// on another day in the same part of day.
func (a Availability) Alternative(t time.Time, avoid []time.Time, pick Picker) (time.Time, bool) {
	var best time.Time
	found := false
	for _, c := range without(a.Candidates(), avoid) {
		if !sameDay(c.In(a.loc()), t.In(a.loc())) {
			continue
		}
		if !found || absDur(c.Sub(t)) < absDur(best.Sub(t)) {
			best, found = c, true
		}
	}
	if found {
		return best, true
	}
	if alt := a.Offer(1, OfferOpts{Avoid: avoid, Prefer: Part(t.In(a.loc()))}, pick); len(alt) > 0 {
		return alt[0], true
	}
	return time.Time{}, false
}

// Window is a part of a day with at least one time to offer.
type Window struct {
	Day  time.Time // midnight
	Part string
}

// Windows lists up to n parts of days to offer (strategy windows), on
// different days where it can, soonest first.
func (a Availability) Windows(n int, avoid []time.Time) []Window {
	loc := a.loc()
	var out []Window
	seenDay := map[string]bool{}
	cands := without(a.Candidates(), avoid)
	for pass := 0; pass < 2 && len(out) < n; pass++ {
		for _, t := range cands {
			if len(out) >= n {
				break
			}
			t = t.In(loc)
			d := time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, loc)
			w := Window{Day: d, Part: Part(t)}
			k := d.Format("2006-01-02")
			if slices.Contains(out, w) || (pass == 0 && seenDay[k]) {
				continue
			}
			seenDay[k] = true
			out = append(out, w)
		}
	}
	slices.SortFunc(out, func(x, y Window) int {
		if c := x.Day.Compare(y.Day); c != 0 {
			return c
		}
		return strings.Compare(partOrder(x.Part), partOrder(y.Part))
	})
	return out
}

func partOrder(p string) string {
	return map[string]string{"morning": "0", "afternoon": "1", "evening": "2"}[p]
}

func absDur(d time.Duration) time.Duration {
	if d < 0 {
		return -d
	}
	return d
}

func index(p Picker, n int) int {
	if p == nil || n <= 1 {
		return 0
	}
	i := p.Index(n)
	return min(max(i, 0), n-1)
}

// Weekdays parses day names ("mon".."sun"); none = Monday to Friday.
func Weekdays(days []string) []time.Weekday {
	names := map[string]time.Weekday{"sun": time.Sunday, "mon": time.Monday, "tue": time.Tuesday, "wed": time.Wednesday,
		"thu": time.Thursday, "fri": time.Friday, "sat": time.Saturday}
	var out []time.Weekday
	for _, d := range days {
		if w, ok := names[strings.ToLower(strings.TrimSpace(d))]; ok && !slices.Contains(out, w) {
			out = append(out, w)
		}
	}
	if len(out) == 0 {
		return []time.Weekday{time.Monday, time.Tuesday, time.Wednesday, time.Thursday, time.Friday}
	}
	return out
}

// For builds the availability of a task from its meeting settings.
func For(m model.TaskMeeting, p model.TaskPrivacy, busy []calendar.Interval, known bool, now time.Time, loc *time.Location) Availability {
	if loc == nil {
		loc = time.Local
	}
	start := now.Add(2 * time.Hour).Truncate(Grid).Add(Grid) // not something in the next couple of hours
	return Availability{Busy: busy, Known: known, Days: Weekdays(m.Days), From: m.From, To: m.To,
		Length: time.Duration(m.Minutes) * time.Minute, Buffer: time.Duration(p.BufferMin) * time.Minute,
		Start: start, End: start.AddDate(0, 0, max(m.HorizonDays, 1)), Loc: loc}
}
