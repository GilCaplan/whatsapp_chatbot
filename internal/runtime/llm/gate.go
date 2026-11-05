package llm

import (
	"context"
	"sync"
)

// A local model answers one call at a time, so calls queue here instead of
// inside Ollama, where the order can't be chosen: replies go first, and the
// rest wait in the order they came.

type replyKey struct{}

// ForReply marks ctx as writing a reply a person is waiting for.
func ForReply(ctx context.Context) context.Context {
	return context.WithValue(ctx, replyKey{}, true)
}

type gate struct {
	mu      sync.Mutex
	busy    bool
	replies []chan struct{}
	others  []chan struct{}
}

// acquire waits for the model; it fails only when ctx ends first.
func (g *gate) acquire(ctx context.Context) error {
	g.mu.Lock()
	if !g.busy {
		g.busy = true
		g.mu.Unlock()
		return nil
	}
	ch := make(chan struct{})
	q := &g.others
	if ctx.Value(replyKey{}) != nil {
		q = &g.replies
	}
	*q = append(*q, ch)
	g.mu.Unlock()
	select {
	case <-ch:
		return nil
	case <-ctx.Done():
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	for i, c := range *q {
		if c == ch {
			*q = append((*q)[:i], (*q)[i+1:]...)
			return ctx.Err()
		}
	}
	// Handed the model just as ctx ended: pass it on.
	g.releaseLocked()
	return ctx.Err()
}

func (g *gate) release() {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.releaseLocked()
}

func (g *gate) releaseLocked() {
	for _, q := range []*[]chan struct{}{&g.replies, &g.others} {
		if len(*q) > 0 {
			close((*q)[0])
			*q = (*q)[1:]
			return
		}
	}
	g.busy = false
}
