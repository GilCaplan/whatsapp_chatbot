package llm

import (
	"context"
	"testing"
	"time"
)

func waitQueued(t *testing.T, g *gate, others, replies int) {
	t.Helper()
	for range 500 {
		g.mu.Lock()
		o, r := len(g.others), len(g.replies)
		g.mu.Unlock()
		if o == others && r == replies {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("calls did not queue")
}

func TestGateRepliesGoFirst(t *testing.T) {
	var g gate
	ctx := context.Background()
	if err := g.acquire(ctx); err != nil {
		t.Fatal(err)
	}
	order := make(chan string, 2)
	take := func(ctx context.Context, name string) {
		if err := g.acquire(ctx); err == nil {
			order <- name
			g.release()
		}
	}
	go take(ctx, "job")
	waitQueued(t, &g, 1, 0)
	go take(ForReply(ctx), "reply")
	waitQueued(t, &g, 1, 1)
	g.release()
	if first := <-order; first != "reply" {
		t.Fatalf("first = %s", first)
	}
	<-order
}

func TestGateCancelledWaiterLeavesTheQueue(t *testing.T) {
	var g gate
	bg := context.Background()
	g.acquire(bg)
	ctx, cancel := context.WithCancel(bg)
	done := make(chan error)
	go func() { done <- g.acquire(ctx) }()
	waitQueued(t, &g, 1, 0)
	cancel()
	if err := <-done; err == nil {
		t.Fatal("expected an error")
	}
	waitQueued(t, &g, 0, 0)
	g.release()
	if err := g.acquire(bg); err != nil {
		t.Fatal(err)
	}
}
