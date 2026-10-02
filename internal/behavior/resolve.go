package behavior

import (
	"reflect"
	"slices"
	"time"

	"whatsappdoppel/internal/model"
)

// Source values in Effective.Sources.
const (
	SourceChat    = "chat"
	SourceDefault = "default"
)

// Effective is the fully resolved behaviour of one chat.
type Effective struct {
	Kind         string                // dm|group
	Profile      model.BehaviorProfile // fully resolved
	Sources      map[string]string     // JSON name → "chat" | "default" (blocks: "availability", "proactive")
	SnoozedUntil *time.Time
}

// KindOf returns "group" for group chats and "dm" otherwise.
func KindOf(c model.ChatAssignment) string {
	if c.Kind == KindGroup {
		return KindGroup
	}
	return KindDM
}

// Resolve layers a chat's overrides on top of base (the app profile for the
// chat's kind). Min/max pairs are reconciled (max = max(min, max)); the preset
// label is the chat's explicit preset when its values still match, else the
// preset the values match, else "custom".
func Resolve(base model.BehaviorProfile, chat model.ChatAssignment) Effective {
	kind := KindOf(chat)
	eff := Effective{Kind: kind, Profile: clone(base), Sources: make(map[string]string, len(fields)+1)}
	pv := reflect.ValueOf(&eff.Profile).Elem()
	ov := reflect.ValueOf(chat.Behavior)
	overridden := false
	for _, f := range fields {
		o := ov.Field(f.oIdx)
		if o.IsNil() {
			eff.Sources[f.name] = SourceDefault
			continue
		}
		overridden = true
		eff.Sources[f.name] = SourceChat
		switch x := o.Interface().(type) {
		case *model.Availability:
			a := cloneAvailability(*x)
			clampAvailability(&a)
			eff.Profile.Availability = a
		default:
			pv.Field(f.pIdx).Set(o.Elem())
		}
	}
	for _, pr := range minMaxPairs {
		lo, hi := intPtr(&eff.Profile, pr[0]), intPtr(&eff.Profile, pr[1])
		*hi = max(*lo, *hi)
	}
	Clamp(&eff.Profile)
	eff.Sources["preset"] = SourceDefault
	if chat.Behavior.Preset != "" {
		eff.Sources["preset"] = SourceChat
	}
	if overridden || chat.Behavior.Preset != "" {
		label := chat.Behavior.Preset
		if d, ok := Preset(label); !ok || !Equal(eff.Profile, d.For(kind)) {
			label = MatchPreset(eff.Profile, kind)
		}
		eff.Profile.Preset = label
	}
	if chat.SnoozedUntil != nil {
		t := *chat.SnoozedUntil
		eff.SnoozedUntil = &t
	}
	return eff
}

// Overridden lists the JSON names the chat overrides (sorted as in the profile).
func (e Effective) Overridden() []string {
	var out []string
	for _, f := range fields {
		if e.Sources[f.name] == SourceChat {
			out = append(out, f.name)
		}
	}
	return out
}

// Snoozed reports whether the chat is "away" at now.
func (e Effective) Snoozed(now time.Time) bool {
	return e.SnoozedUntil != nil && now.Before(*e.SnoozedUntil)
}

// Available reports whether the persona is active at now (not snoozed and
// inside active hours) and when that changes next: the next opening when
// closed, the next closing when open (zero = never).
func (e Effective) Available(now time.Time) (open bool, next time.Time) {
	if e.Snoozed(now) {
		until := *e.SnoozedUntil
		if o, n := NextOpen(e.Profile.Availability, until); !o {
			return false, n
		}
		return false, until
	}
	return NextOpen(e.Profile.Availability, now)
}

// Location returns the availability time zone (Local when "" or unknown).
func Location(av model.Availability) *time.Location {
	if av.Timezone != "" {
		if loc, err := time.LoadLocation(av.Timezone); err == nil {
			return loc
		}
	}
	return time.Local
}

type interval struct{ start, end time.Time }

// NextOpen evaluates active hours at now. Disabled availability is always open
// (next zero). When open, next is when the current stretch ends (zero if it
// never does); when closed, next is the next opening (zero if the week has no
// hours at all). Ranges wrap past midnight when To <= From; From == To is the
// whole day. Computed in the availability's zone, so DST days are handled by
// time.Date's normalisation.
func NextOpen(av model.Availability, now time.Time) (open bool, next time.Time) {
	if !av.Enabled {
		return true, time.Time{}
	}
	loc := Location(av)
	week := NormalizeWeek(av.Week)
	local := now.In(loc)
	y, m, d := local.Date()
	var ivs []interval
	// Day -1 covers ranges that wrap into today; scan up to 9 days ahead so a
	// stretch that spans the whole week still finds its end.
	for off := -1; off <= 8; off++ {
		day := time.Date(y, m, d+off, 0, 0, 0, 0, loc)
		idx := (int(day.Weekday()) + 6) % 7 // Monday = 0
		for _, r := range week[idx].Ranges {
			from, ok1 := parseHM(r.From, false)
			to, ok2 := parseHM(r.To, true)
			if !ok1 || !ok2 {
				continue
			}
			start := time.Date(y, m, d+off, from/60, from%60, 0, 0, loc)
			endDay := d + off
			if to <= from {
				endDay++
			}
			end := time.Date(y, m, endDay, to/60, to%60, 0, 0, loc)
			if to == from {
				end = time.Date(y, m, d+off+1, from/60, from%60, 0, 0, loc)
			}
			if end.After(start) {
				ivs = append(ivs, interval{start, end})
			}
		}
	}
	if len(ivs) == 0 {
		return false, time.Time{}
	}
	slices.SortFunc(ivs, func(a, b interval) int { return a.start.Compare(b.start) })
	merged := []interval{ivs[0]}
	for _, iv := range ivs[1:] {
		last := &merged[len(merged)-1]
		if !iv.start.After(last.end) {
			if iv.end.After(last.end) {
				last.end = iv.end
			}
			continue
		}
		merged = append(merged, iv)
	}
	for i, iv := range merged {
		if !now.Before(iv.start) && now.Before(iv.end) {
			if i == len(merged)-1 && iv.end.Sub(now) > 7*24*time.Hour {
				return true, time.Time{} // open around the clock
			}
			return true, iv.end
		}
		if iv.start.After(now) {
			return false, iv.start
		}
	}
	return false, time.Time{}
}
