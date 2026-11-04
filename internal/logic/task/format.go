package task

import (
	"fmt"
	"strings"
	"time"

	"whatsappdoppel/internal/core/model"
)

var heDays = [...]string{"יום ראשון", "יום שני", "יום שלישי", "יום רביעי", "יום חמישי", "יום שישי", "שבת"}

var heParts = map[string]string{"morning": "בבוקר", "afternoon": "אחר הצהריים", "evening": "בערב"}

func clock(t time.Time, tone model.TaskTone) string {
	if tone.Clock == "12h" {
		if t.Minute() == 0 {
			return t.Format("3 PM")
		}
		return t.Format("3:04 PM")
	}
	return t.Format("15:04")
}

// Day writes a day the way tone says ("Tue 10 Nov" / "Tue Nov 10"; Hebrew
// "יום שלישי 10.11" when code is "he").
func Day(t time.Time, tone model.TaskTone, code string) string {
	if code == "he" {
		return fmt.Sprintf("%s %d.%d", heDays[t.Weekday()], t.Day(), int(t.Month()))
	}
	if tone.DateStyle == "mdy" {
		return t.Format("Mon Jan 2")
	}
	return t.Format("Mon 2 Jan")
}

// When writes a meeting time ("Tue 10 Nov 10:00").
func When(t time.Time, tone model.TaskTone, code string) string {
	if code == "he" {
		return Day(t, tone, code) + " ב-" + clock(t, tone)
	}
	if tone.DateStyle == "mdy" {
		return Day(t, tone, code) + ", " + clock(t, tone)
	}
	return Day(t, tone, code) + " " + clock(t, tone)
}

// WindowText writes a part of a day ("Tue 10 Nov, morning").
func WindowText(w Window, tone model.TaskTone, code string) string {
	if code == "he" {
		return Day(w.Day, tone, code) + " " + heParts[w.Part]
	}
	return Day(w.Day, tone, code) + ", " + w.Part
}

// List joins items with commas and "or".
func List(items []string, code string) string {
	or := " or "
	if code == "he" {
		or = " או "
	}
	switch len(items) {
	case 0:
		return ""
	case 1:
		return items[0]
	}
	return strings.Join(items[:len(items)-1], ", ") + or + items[len(items)-1]
}
