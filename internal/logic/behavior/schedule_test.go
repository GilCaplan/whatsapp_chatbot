package behavior

import (
	"testing"
	"time"

	"whatsappdoppel/internal/core/model"
)

func TestScheduleGap(t *testing.T) {
	sc := func(repeat string, every, spread int) model.ProactiveSchedule {
		return model.ProactiveSchedule{Repeat: repeat, EveryMin: every, SpreadMin: spread}
	}
	for _, c := range []struct {
		name string
		s    model.ProactiveSchedule
		frac float64
		want time.Duration
	}{
		{"fixed", sc(RepeatFixed, 60, 30), 0.9, time.Hour},
		{"uniform low", sc(RepeatUniform, 60, 30), 0, 30 * time.Minute},
		{"uniform high", sc(RepeatUniform, 60, 30), 1, 90 * time.Minute},
		{"normal median", sc(RepeatNormal, 60, 20), 0.5, time.Hour},
		{"normal floor", sc(RepeatNormal, 5, 60), 0, time.Minute},
		{"poisson median", sc(RepeatPoisson, 60, 0), 0.5, 41*time.Minute + 35*time.Second},
		{"default every", sc(RepeatFixed, 0, 0), 0.5, DefaultScheduleEveryMin * time.Minute},
	} {
		if got := ScheduleGap(Fixed{Frac: c.frac}, c.s); got != c.want {
			t.Errorf("%s: gap = %v, want %v", c.name, got, c.want)
		}
	}
	// Poisson gaps average EveryMin.
	rng := NewSampler(nil)
	var total time.Duration
	const n = 20000
	for range n {
		total += ScheduleGap(rng, sc(RepeatPoisson, 60, 0))
	}
	if avg := total / n; avg < 57*time.Minute || avg > 63*time.Minute {
		t.Errorf("poisson average = %v", avg)
	}
}

func TestScheduleValidateAndClamp(t *testing.T) {
	ok := model.Proactive{AfterHours: 48, MaxPerDay: 1, Schedule: model.ProactiveSchedule{Repeat: RepeatNormal, EveryMin: 60, SpreadMin: 15}}
	if err := validateProactive(ok); err != nil {
		t.Fatal(err)
	}
	for name, s := range map[string]model.ProactiveSchedule{
		"repeat":    {Repeat: "sometimes"},
		"every":     {Repeat: RepeatFixed, EveryMin: 1},
		"maxPerDay": {MaxPerDay: 500},
		"times":     {Times: make([]time.Time, MaxScheduleTimes+1)},
	} {
		p := ok
		p.Schedule = s
		if err := validateProactive(p); err == nil {
			t.Errorf("%s: want an error", name)
		}
	}
	t0 := time.Date(2026, 1, 1, 9, 0, 0, 0, time.UTC)
	s := model.ProactiveSchedule{Repeat: "nope", EveryMin: 1, Times: []time.Time{t0.Add(time.Hour), {}, t0, t0}}
	clampSchedule(&s)
	if s.Repeat != RepeatOff || s.EveryMin != 5 || len(s.Times) != 2 || !s.Times[0].Equal(t0) {
		t.Errorf("clamped = %+v", s)
	}
	if got := NextTime(s, t0, t0); !got.Equal(t0.Add(time.Hour)) {
		t.Errorf("next after done = %v", got)
	}
}
