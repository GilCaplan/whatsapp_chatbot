package behavior

import (
	"math"
	"time"
)

// Timing curves (BehaviorProfile.TimingCurve): how a delay is drawn between
// its min and max — notice, think, distracted and the pause between bubbles.
const (
	CurveUniform     = "uniform"     // any value equally likely ("" too)
	CurveNormal      = "normal"      // bell curve around the middle (σ = range/6)
	CurveExponential = "exponential" // mostly quick, now and then long (mean = range/4)
	CurveFixed       = "fixed"       // always the middle
)

// TimingCurves are the allowed values ("" = uniform).
var TimingCurves = []string{"", CurveUniform, CurveNormal, CurveExponential, CurveFixed}

// Delay draws a delay between minSec and maxSec along curve; uniform (and
// "") is exactly s.Between.
func Delay(s Sampler, minSec, maxSec int, curve string) time.Duration {
	var frac float64
	switch curve {
	case CurveNormal:
		u := min(max(s.Float(), 1e-9), 1-1e-9)
		frac = 0.5 + math.Sqrt2*math.Erfinv(2*u-1)/6
	case CurveExponential:
		u := min(s.Float(), 1-1e-9)
		frac = -math.Log(1-u) / 4
	case CurveFixed:
		frac = 0.5
	default:
		return s.Between(minSec, maxSec)
	}
	return between(minSec, maxSec, min(max(frac, 0), 1))
}
