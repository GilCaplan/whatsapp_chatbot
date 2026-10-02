package behavior

import (
	"strings"
	"testing"

	"whatsappdoppel/internal/model"
)

// seqSampler returns Index values from a list (cycling), for exact typo tests.
type seqSampler struct {
	Fixed
	idx []int
	n   int
}

func (s *seqSampler) Index(n int) int {
	if n <= 1 || len(s.idx) == 0 {
		return 0
	}
	v := s.idx[s.n%len(s.idx)] % n
	s.n++
	return v
}

func TestMakeTypoNoCandidates(t *testing.T) {
	for _, txt := range []string{
		"שלום מה שלומך היום",            // Hebrew
		"ok lol yes",                    // short words
		"check https://example.com/abc", // only "check" — but a link token is skipped
		"@Dana @Josh",                   // tags
		"HELLO THERE",                   // all caps
		"on 0541234567",                 // numbers
		"abc123def4",                    // glued to digits
	} {
		tp, ok := MakeTypo(txt, Fixed{Frac: 0.5})
		if txt == "check https://example.com/abc" {
			if !ok || !strings.Contains(tp.Typed, "https://example.com/abc") || tp.Word != "check" {
				t.Errorf("link text: %+v %v", tp, ok)
			}
			continue
		}
		if ok {
			t.Errorf("MakeTypo(%q) = %+v, want no typo", txt, tp)
		}
	}
}

func TestMakeTypoKinds(t *testing.T) {
	text := "see you tomorrow at the cafe"
	// word index 0 → "tomorrow" (only candidate ≥4 letters besides "cafe")
	cases := []struct {
		kindIdx int
		want    string
		kind    string
	}{
		{0, "tmoorrow", TypoSwap},    // swap at position 1 (first eligible)
		{1, "tmorrow", TypoDrop},     // drop index 1
		{2, "toomorrow", TypoDouble}, // double index 1
		{3, "tpmorrow", TypoNear},    // neighbour of 'o' at index 1 → 'i','p','l' → idx0 'i'? see below
	}
	for _, c := range cases {
		s := &seqSampler{idx: []int{0, c.kindIdx, 0, 0}}
		tp, ok := MakeTypo(text, s)
		if !ok || tp.Kind != c.kind || tp.Word != "tomorrow" {
			t.Fatalf("kind %s: %+v %v", c.kind, tp, ok)
		}
		if tp.Typed != strings.Replace(text, "tomorrow", tp.Wrong, 1) {
			t.Errorf("typed %q", tp.Typed)
		}
		if tp.Wrong == "tomorrow" || tp.Wrong[0] != 't' {
			t.Errorf("wrong %q", tp.Wrong)
		}
		if c.kind != TypoNear && tp.Wrong != c.want {
			t.Errorf("%s: got %q want %q", c.kind, tp.Wrong, c.want)
		}
		if FixBubble(tp) != "*tomorrow" {
			t.Error("fix bubble")
		}
	}
}

func TestMakeTypoKeepsCase(t *testing.T) {
	s := &seqSampler{idx: []int{0, 3, 0, 0}}
	tp, ok := MakeTypo("Honestly", s)
	if !ok || tp.Wrong[0] != 'H' {
		t.Fatalf("%+v %v", tp, ok)
	}
}

func TestTypoFieldsValidate(t *testing.T) {
	p := model.BehaviorProfile{}
	Normalize(&p, KindDM)
	if p.TypoFixStyle == "" {
		t.Error("typoFixStyle default missing")
	}
	bad := 31
	if err := ValidateOverrides(model.BehaviorOverrides{TypoPercent: &bad}); err == nil {
		t.Error("typoPercent 31 must be rejected")
	}
	style := "shout"
	if err := ValidateOverrides(model.BehaviorOverrides{TypoFixStyle: &style}); err == nil {
		t.Error("unknown typoFixStyle must be rejected")
	}
}

func TestPersonaZoneMarker(t *testing.T) {
	av := model.Availability{Enabled: true, Timezone: model.PersonaZoneMarker, OutsideHours: "queue", CatchUpMaxMin: 10}
	if err := ValidateOverrides(model.BehaviorOverrides{Availability: &av}); err != nil {
		t.Fatalf("persona marker rejected: %v", err)
	}
	eff := Resolve(model.BehaviorProfile{}, model.ChatAssignment{Behavior: model.BehaviorOverrides{Availability: &av}})
	if eff.Profile.Availability.Timezone != model.PersonaZoneMarker {
		t.Fatalf("clamp dropped the marker: %q", eff.Profile.Availability.Timezone)
	}
	if got := WithZone(eff, "Asia/Tokyo").Profile.Availability.Timezone; got != "Asia/Tokyo" {
		t.Errorf("WithZone = %q", got)
	}
	if got := WithZone(eff, "").Profile.Availability.Timezone; got != "" {
		t.Errorf("WithZone same-as-me = %q", got)
	}
	if eff.Profile.Availability.Timezone != model.PersonaZoneMarker {
		t.Error("WithZone must not modify its argument's profile")
	}
}
