package prompt

import (
	"fmt"
	"slices"
	"strings"
	"time"

	"whatsappdoppel/internal/model"
	"whatsappdoppel/internal/world"
)

// ─── World & time (wave 3) ───────────────────────────────────
//
// The reply prompt knows what time it is where the persona lives, whether
// it's the weekend there, what the persona is busy with (its routine) and
// whether it's answering late. Nothing here is added when Options.Now is
// zero, so goldens without a clock are unchanged.

// WorldSection renders "RIGHT NOW: …" for the persona at now. mac is this
// Mac's zone (where the people chatting probably are; nil = Local).
func WorldSection(w model.World, now time.Time, mac *time.Location) string {
	if now.IsZero() {
		return ""
	}
	loc := world.Location(w)
	local := now.In(loc)
	var b strings.Builder
	b.WriteString("\n\nRIGHT NOW: " + local.Format("Monday 2 January, 15:04") + " — " + world.PartOfDay(local.Hour()))
	if place := world.Place(w); place != "" {
		b.WriteString(" in " + place)
	}
	b.WriteString(" (your local time).")
	days, known := world.WeekendFor(w, loc)
	today := world.DayName(local)
	tomorrow := world.DayName(local.AddDate(0, 0, 1))
	switch {
	case slices.Contains(days, today):
		b.WriteString(" It's the weekend")
		if known {
			b.WriteString(" (" + world.FmtDayRange(days) + " here)")
		}
		b.WriteString(".")
	case known && slices.Contains(days, tomorrow):
		b.WriteString(" The weekend (" + world.FmtDayRange(days) + " here) starts tomorrow.")
	case known:
		b.WriteString(" The weekend here is " + world.FmtDayRange(days) + ".")
	}
	if a, ok := world.RoutineAt(w, now); ok {
		b.WriteString("\nYou're in the middle of: " + a.Block.Label + " (" + a.Block.From + "–" + a.Block.To + ").")
	} else if a, ok := world.JustFinished(w, now, 45*time.Minute); ok {
		b.WriteString(fmt.Sprintf("\nYou just finished: %s (ended %s ago).", a.Block.Label, agoText(now.Sub(a.End))))
	} else if next := upcoming(w, now, time.Hour); next != nil {
		b.WriteString(fmt.Sprintf("\nComing up: %s at %s.", next.Block.Label, next.Start.In(loc).Format("15:04")))
	}
	if mac == nil {
		mac = time.Local
	}
	if other := now.In(mac); other.Format("2006-01-02 15:04") != local.Format("2006-01-02 15:04") {
		zone := world.ZoneName(mac)
		when := other.Format("15:04")
		if other.YearDay() != local.YearDay() {
			when += " on " + other.Format("Monday")
		}
		if zone != "" {
			b.WriteString(fmt.Sprintf("\nThe people you're chatting with are probably on %s time, where it's %s.", zone, when))
		} else {
			b.WriteString(fmt.Sprintf("\nThe people you're chatting with are probably in another time zone, where it's %s.", when))
		}
	}
	b.WriteString("\nBe aware of the time when it matters (late at night, early morning, weekends, meal times), but don't announce it.")
	return b.String()
}

// upcoming is the next routine block starting within d after now.
func upcoming(w model.World, now time.Time, d time.Duration) *world.Active {
	for _, a := range world.RoutineBetween(w, now, now.Add(d)) {
		if a.Start.After(now) {
			return &a
		}
	}
	return nil
}

func agoText(d time.Duration) string {
	m := int(d.Round(time.Minute) / time.Minute)
	switch {
	case m <= 1:
		return "a minute"
	case m < 60:
		return fmt.Sprintf("%d minutes", m)
	}
	return "about an hour"
}

// LateNote says the persona is answering long after the message it answers.
type LateNote struct {
	Minutes int
	Busy    string // routine block that kept it away, e.g. "gym until 20:00" ("" = no specific reason)
}

// LateSection renders the late-reply note ("" for nil).
func LateSection(l *LateNote) string {
	if l == nil || l.Minutes <= 0 {
		return ""
	}
	when := lateText(l.Minutes)
	if busy := cleanLine(l.Busy, 60); busy != "" {
		return fmt.Sprintf("\n\nLATE REPLY: you're answering %s after their message — your routine had you busy (%s), so you're only seeing it now. If it fits, a brief natural reason or a quick \"sorry, just saw this\" is fine. Don't over-apologise, and skip it if it would feel odd.", when, busy)
	}
	return fmt.Sprintf("\n\nLATE REPLY: you're answering %s after their message (you were away from your phone). A quick \"sorry, just saw this\" is fine if it fits; don't invent a specific reason.", when)
}

func lateText(min int) string {
	switch {
	case min < 60:
		return fmt.Sprintf("about %d minutes", min)
	case min < 90:
		return "about an hour"
	case min < 24*60:
		return fmt.Sprintf("about %d hours", (min+30)/60)
	}
	return "more than a day"
}
