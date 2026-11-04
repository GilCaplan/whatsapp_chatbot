package llm

import (
	"context"
	"sync"
	"time"
)

// Meter counts the AI calls made with a context (the context-system
// sandbox): Registry.Resolve hands out providers that report every Chat on a
// context carrying a meter. Calls are filed under the meter's current step.
type Meter struct {
	mu    sync.Mutex
	step  string
	Calls int
	By    map[string]int
	Chars int // request sizes (system + messages)
	Time  time.Duration
	// Last is the size of the last request in the "reply" step.
	Last int
}

type meterKey struct{}

// WithMeter returns ctx carrying m.
func WithMeter(ctx context.Context, m *Meter) context.Context {
	return context.WithValue(ctx, meterKey{}, m)
}

// MeterFrom returns the meter on ctx (nil = none).
func MeterFrom(ctx context.Context) *Meter {
	m, _ := ctx.Value(meterKey{}).(*Meter)
	return m
}

// Step sets the step later calls are filed under.
func (m *Meter) Step(s string) {
	if m == nil {
		return
	}
	m.mu.Lock()
	m.step = s
	m.mu.Unlock()
}

// Snapshot returns the call count and per-step counts so far.
func (m *Meter) Snapshot() (int, map[string]int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	by := make(map[string]int, len(m.By))
	for k, v := range m.By {
		by[k] = v
	}
	return m.Calls, by
}

func (m *Meter) record(req Request, d time.Duration) {
	n := len(req.System)
	for _, x := range req.Messages {
		n += len(x.Content)
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.By == nil {
		m.By = map[string]int{}
	}
	m.Calls++
	m.By[m.step]++
	m.Chars += n
	m.Time += d
	if m.step == "reply" {
		m.Last = n
	}
}

type metered struct{ Provider }

func (p metered) Chat(ctx context.Context, req Request) (Response, error) {
	m := MeterFrom(ctx)
	if m == nil {
		return p.Provider.Chat(ctx, req)
	}
	start := time.Now()
	resp, err := p.Provider.Chat(ctx, req)
	m.record(req, time.Since(start))
	return resp, err
}
