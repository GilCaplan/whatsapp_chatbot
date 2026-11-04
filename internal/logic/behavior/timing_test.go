package behavior

import (
	"testing"
	"time"
)

func TestDelayCurves(t *testing.T) {
	if got := Delay(Fixed{Frac: 0.9}, 0, 100, ""); got != Fixed(Fixed{Frac: 0.9}).Between(0, 100) {
		t.Errorf("uniform = %v", got)
	}
	if got := Delay(Fixed{Frac: 0.9}, 0, 100, CurveFixed); got != 50*time.Second {
		t.Errorf("fixed = %v", got)
	}
	if got := Delay(Fixed{Frac: 0.5}, 0, 60, CurveNormal); got != 30*time.Second {
		t.Errorf("normal median = %v", got)
	}
	// Shapes over many draws: the bell curve keeps ~95% within two σ (range/6), the
	// exponential is mostly quick, every draw stays in range.
	rng := NewSampler(nil)
	const n = 20000
	mid, quick := 0, 0
	for range n {
		if d := Delay(rng, 0, 100, CurveNormal); d >= 17*time.Second && d <= 83*time.Second {
			mid++
		} else if d < 0 || d > 100*time.Second {
			t.Fatalf("normal out of range: %v", d)
		}
		if d := Delay(rng, 0, 100, CurveExponential); d < 25*time.Second {
			quick++
		} else if d > 100*time.Second {
			t.Fatalf("exponential out of range: %v", d)
		}
	}
	if mid < n*93/100 {
		t.Errorf("normal: %d of %d within two standard deviations", mid, n)
	}
	if quick < n*55/100 || quick > n*70/100 {
		t.Errorf("exponential: %d of %d under a quarter", quick, n)
	}
}
