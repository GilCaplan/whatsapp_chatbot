package behavior

import (
	"fmt"
	"math"
	"slices"
	"time"

	"whatsappdoppel/internal/model"
)

// Scheduled messages (model.ProactiveSchedule): validation, defaults and the
// gaps between repeats.

// Repeat kinds: an exact gap, or a random one drawn from a distribution
// around EveryMin.
const (
	RepeatOff     = "off"
	RepeatFixed   = "fixed"   // exactly EveryMin
	RepeatUniform = "uniform" // EveryMin ± SpreadMin, every value as likely
	RepeatNormal  = "normal"  // bell curve: mean EveryMin, standard deviation SpreadMin
	RepeatPoisson = "poisson" // random moments at an average rate (exponential gaps, mean EveryMin)
)

const (
	MaxScheduleTimes         = 20
	DefaultScheduleMaxPerDay = 12
	DefaultScheduleEveryMin  = 120
	// minScheduleGap keeps random gaps from collapsing to nothing.
	minScheduleGap = time.Minute
)

// RepeatKinds lists the repeat values (enum "proactive.schedule.repeat").
var RepeatKinds = []string{RepeatOff, RepeatFixed, RepeatUniform, RepeatNormal, RepeatPoisson}

// Repeats reports whether the schedule repeats.
func Repeats(s model.ProactiveSchedule) bool { return s.Repeat != "" && s.Repeat != RepeatOff }

// ScheduleOn reports whether the schedule plans anything at all.
func ScheduleOn(s model.ProactiveSchedule) bool {
	return s.OnStart || len(s.Times) > 0 || Repeats(s)
}

// ScheduleCap is the most scheduled messages per 24 h.
func ScheduleCap(s model.ProactiveSchedule) int {
	if s.MaxPerDay <= 0 {
		return DefaultScheduleMaxPerDay
	}
	return s.MaxPerDay
}

// RepeatKey identifies the repeat settings a planned moment was drawn with;
// when they change, the next repeat is drawn again.
func RepeatKey(s model.ProactiveSchedule) string {
	if !Repeats(s) {
		return ""
	}
	return fmt.Sprintf("%s/%d/%d", s.Repeat, s.EveryMin, s.SpreadMin)
}

// ScheduleGap draws the wait until the next repeat (never below a minute).
func ScheduleGap(rng Sampler, s model.ProactiveSchedule) time.Duration {
	everyMin := s.EveryMin
	if everyMin <= 0 {
		everyMin = DefaultScheduleEveryMin
	}
	every := float64(everyMin) * float64(time.Minute)
	spread := float64(max(s.SpreadMin, 0)) * float64(time.Minute)
	// u in (0, 1): the inverse CDFs below are infinite at the ends.
	u := min(max(rng.Float(), 1e-6), 1-1e-6)
	var d float64
	switch s.Repeat {
	case RepeatUniform:
		lo := max(every-spread, float64(minScheduleGap))
		d = lo + u*(every+spread-lo)
	case RepeatNormal:
		d = every + spread*math.Sqrt2*math.Erfinv(2*u-1)
	case RepeatPoisson:
		d = -every * math.Log(1-u)
	default:
		d = every
	}
	return max(time.Duration(d), minScheduleGap).Round(time.Second)
}

// NextTime returns the first fixed moment after done that is not before
// from (zero when there is none). Times are sorted by Clamp.
func NextTime(s model.ProactiveSchedule, done, from time.Time) time.Time {
	for _, t := range s.Times {
		if t.After(done) && !t.Before(from) {
			return t
		}
	}
	return time.Time{}
}

func validateSchedule(s model.ProactiveSchedule) error {
	if s.Repeat != "" {
		if err := checkEnum("proactive.schedule.repeat", s.Repeat); err != nil {
			return err
		}
	}
	if Repeats(s) && s.EveryMin != 0 {
		if err := checkRange("proactive.schedule.everyMin", s.EveryMin); err != nil {
			return err
		}
		if err := checkRange("proactive.schedule.spreadMin", s.SpreadMin); err != nil {
			return err
		}
	}
	if s.MaxPerDay != 0 {
		if err := checkRange("proactive.schedule.maxPerDay", s.MaxPerDay); err != nil {
			return err
		}
	}
	if len(s.Times) > MaxScheduleTimes {
		return fmt.Errorf("proactive.schedule.times: at most %d moments", MaxScheduleTimes)
	}
	return nil
}

// clampSchedule keeps zero values (= the defaults) so presets stay exact.
func clampSchedule(s *model.ProactiveSchedule) {
	if s.Repeat != "" && !slices.Contains(RepeatKinds, s.Repeat) {
		s.Repeat = RepeatOff
	}
	if s.EveryMin != 0 {
		clampInt("proactive.schedule.everyMin", &s.EveryMin)
	}
	clampInt("proactive.schedule.spreadMin", &s.SpreadMin)
	if s.MaxPerDay != 0 {
		clampInt("proactive.schedule.maxPerDay", &s.MaxPerDay)
	}
	s.Times = NormalizeTimes(s.Times)
}

// NormalizeTimes sorts the fixed moments, drops zero values and duplicates
// and keeps at most MaxScheduleTimes.
func NormalizeTimes(ts []time.Time) []time.Time {
	if len(ts) == 0 {
		return nil
	}
	out := make([]time.Time, 0, len(ts))
	for _, t := range ts {
		if !t.IsZero() {
			out = append(out, t)
		}
	}
	slices.SortFunc(out, func(a, b time.Time) int { return a.Compare(b) })
	out = slices.CompactFunc(out, func(a, b time.Time) bool { return a.Equal(b) })
	if len(out) > MaxScheduleTimes {
		out = out[:MaxScheduleTimes]
	}
	return out
}
