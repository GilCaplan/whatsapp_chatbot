package persona

import (
	"slices"
	"strings"
	"time"

	"whatsappdoppel/internal/core/model"
)

// Personal secretary limits.
const (
	MaxFAQs        = 20
	MaxVIPs        = 30
	MaxFAQQuestion = 200
	MaxFAQAnswer   = 600
)

var weekdayIDs = []string{"mon", "tue", "wed", "thu", "fri", "sat", "sun"}

// normalizeSecretary cleans a secretary block: defaults (friendly tone,
// weekdays 09:00–18:00, 30-minute meetings), trimmed texts and lists.
func normalizeSecretary(s *model.SecretaryConfig) {
	s.OwnerName = clamp(s.OwnerName, MaxName)
	s.Tone = strings.ToLower(strings.TrimSpace(s.Tone))
	if s.Tone != "formal" {
		s.Tone = "friendly"
	}
	var days []string
	for _, d := range s.MeetDays {
		d = strings.ToLower(strings.TrimSpace(d))
		if len(d) > 3 {
			d = d[:3]
		}
		if slices.Contains(weekdayIDs, d) && !slices.Contains(days, d) {
			days = append(days, d)
		}
	}
	if len(days) == 0 {
		days = []string{"mon", "tue", "wed", "thu", "fri"}
	}
	slices.SortFunc(days, func(a, b string) int { return slices.Index(weekdayIDs, a) - slices.Index(weekdayIDs, b) })
	s.MeetDays = days
	from, errF := time.Parse("15:04", strings.TrimSpace(s.MeetFrom))
	to, errT := time.Parse("15:04", strings.TrimSpace(s.MeetTo))
	if errF != nil || errT != nil || !to.After(from) {
		s.MeetFrom, s.MeetTo = "09:00", "18:00"
	} else {
		s.MeetFrom, s.MeetTo = from.Format("15:04"), to.Format("15:04")
	}
	if s.MeetingMinutes <= 0 {
		s.MeetingMinutes = 30
	}
	s.MeetingMinutes = min(max(s.MeetingMinutes, 10), 240)
	s.Shareable = clamp(s.Shareable, MaxShortField*2)
	s.Never = clamp(s.Never, MaxShortField*2)
	s.AwayMessage = clamp(s.AwayMessage, MaxShortField)
	var faqs []model.FAQ
	for _, f := range s.FAQs {
		q, a := clamp(f.Q, MaxFAQQuestion), clamp(f.A, MaxFAQAnswer)
		if q != "" && a != "" && len(faqs) < MaxFAQs {
			faqs = append(faqs, model.FAQ{Q: q, A: a})
		}
	}
	s.FAQs = faqs
	var vips []string
	for _, v := range s.VIPs {
		if v = clamp(v, 60); v != "" && !slices.Contains(vips, v) && len(vips) < MaxVIPs {
			vips = append(vips, v)
		}
	}
	s.VIPs = vips
}

// Weekdays turns MeetDays into time.Weekdays.
func Weekdays(days []string) []time.Weekday {
	var out []time.Weekday
	for _, d := range days {
		if i := slices.Index(weekdayIDs, d); i >= 0 {
			out = append(out, time.Weekday((i+1)%7))
		}
	}
	return out
}
