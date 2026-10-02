package behavior

import (
	"math/rand/v2"
	"sync"
	"time"
)

// Sampler is the only source of randomness for reply behaviour, so tests can
// make every delay and roll deterministic.
type Sampler interface {
	// Hit rolls a percent chance: 0 never hits, >= 100 always hits.
	Hit(percent int) bool
	// Between returns a duration uniformly in [minSec, maxSec] seconds.
	Between(minSec, maxSec int) time.Duration
	// Jitter scales d by a random factor in [1-pct/100, 1+pct/100].
	Jitter(d time.Duration, pct int) time.Duration
	// Index returns a value in [0, n) (0 when n <= 0).
	Index(n int) int
}

// NewSampler returns a goroutine-safe Sampler on src (nil = time-seeded PCG).
func NewSampler(src rand.Source) Sampler {
	if src == nil {
		now := uint64(time.Now().UnixNano())
		src = rand.NewPCG(now, now>>17^0x9e3779b97f4a7c15)
	}
	return &randSampler{r: rand.New(src)}
}

type randSampler struct {
	mu sync.Mutex
	r  *rand.Rand
}

func (s *randSampler) float() float64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.r.Float64()
}

func (s *randSampler) Hit(percent int) bool {
	switch {
	case percent <= 0:
		return false
	case percent >= 100:
		return true
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.r.IntN(100) < percent
}

func (s *randSampler) Between(minSec, maxSec int) time.Duration {
	return between(minSec, maxSec, s.float())
}

func (s *randSampler) Jitter(d time.Duration, pct int) time.Duration {
	return jitter(d, pct, s.float())
}

func (s *randSampler) Index(n int) int {
	if n <= 1 {
		return 0
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.r.IntN(n)
}

// Fixed is a deterministic Sampler for tests: Between returns
// min + Frac*(max-min), Jitter maps Frac 0 → -pct, 0.5 → 0, 1 → +pct,
// Hit returns Hits for 1..99 percent (0 never, 100 always).
type Fixed struct {
	Hits bool
	Frac float64
}

func (f Fixed) Hit(percent int) bool {
	switch {
	case percent <= 0:
		return false
	case percent >= 100:
		return true
	}
	return f.Hits
}

func (f Fixed) Between(minSec, maxSec int) time.Duration { return between(minSec, maxSec, f.Frac) }

func (f Fixed) Jitter(d time.Duration, pct int) time.Duration { return jitter(d, pct, f.Frac) }

func (f Fixed) Index(n int) int {
	if n <= 1 {
		return 0
	}
	return min(int(f.Frac*float64(n)), n-1)
}

func between(minSec, maxSec int, frac float64) time.Duration {
	if maxSec < minSec {
		minSec, maxSec = maxSec, minSec
	}
	frac = min(max(frac, 0), 1)
	lo, hi := time.Duration(minSec)*time.Second, time.Duration(maxSec)*time.Second
	d := lo + time.Duration(frac*float64(hi-lo))
	return d.Round(100 * time.Millisecond)
}

func jitter(d time.Duration, pct int, frac float64) time.Duration {
	if pct <= 0 || d <= 0 {
		return d
	}
	frac = min(max(frac, 0), 1)
	f := 1 + (2*frac-1)*float64(pct)/100
	return time.Duration(float64(d) * f)
}
