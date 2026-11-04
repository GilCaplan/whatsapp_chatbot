package engine

import (
	"testing"
	"time"

	"whatsappdoppel/internal/behavior"
	"whatsappdoppel/internal/store"
)

// Over many typos in one chat no kind comes back more than twice in the
// window, never twice in a row, and the correction shape rotates.
func TestTypoVariety(t *testing.T) {
	rng := behavior.NewSampler(nil)
	var marks []store.TypoMark
	text := "you're going to get some rest tomorrow and then we talk"
	for i := range 40 {
		tp, ok := behavior.VariedTypo(text, rng, typoAvoid(marks))
		if !ok {
			t.Fatalf("round %d: no typo", i)
		}
		if n := len(marks); n > 0 && marks[n-1].Kind == tp.Kind {
			t.Fatalf("round %d: %s twice in a row", i, tp.Kind)
		}
		count := 0
		for _, m := range marks {
			if m.Kind == tp.Kind {
				count++
			}
		}
		if count >= typoKindMax {
			t.Fatalf("round %d: %s already used %d times", i, tp.Kind, count)
		}
		marks = append(marks, store.TypoMark{At: time.Now(), Kind: tp.Kind})
		if len(marks) > store.MaxTypoMarks {
			marks = marks[1:]
		}
	}
	h := newHarness(t, nil, dmChat())
	r := h.e.runner(dmKey)
	last := -1
	var ms []store.TypoMark
	for range 9 {
		f := r.fixForm(ms)
		if f == last {
			t.Fatalf("form %d twice in a row", f)
		}
		last = f
		ms = append(ms, store.TypoMark{Kind: "swap", Form: f})
	}
}
