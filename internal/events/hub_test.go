package events

import (
	"testing"

	"whatsappdoppel/internal/model"
)

func TestReplayAndActivity(t *testing.T) {
	h := NewHub()
	h.Publish(TypeChatsChanged, struct{}{})
	h.Publish(TypeWAQR, model.QRFrame{PNG: "x"}) // not replayed
	a := h.Activity(model.ActivityEvent{Type: model.ActSent, ChatKey: "dm:1", Text: "yo"})
	if a.ID != 1 || a.TS.IsZero() {
		t.Fatalf("activity not stamped: %+v", a)
	}
	_, backlog, cancel := h.Subscribe(1)
	defer cancel()
	for _, ev := range backlog {
		if ev.Type == TypeWAQR {
			t.Fatal("wa.qr must not be replayed")
		}
	}
	if len(backlog) != 1 || backlog[0].Type != TypeActivity {
		t.Fatalf("backlog = %+v", backlog)
	}
	items, last := h.ActivitySince(0, "dm:1", map[string]bool{model.ActSent: true}, 10)
	if len(items) != 1 || last != 1 {
		t.Fatalf("ActivitySince = %d items, last %d", len(items), last)
	}
	if items, _ := h.ActivitySince(0, "dm:2", nil, 10); len(items) != 0 {
		t.Fatal("chat filter failed")
	}
}

func TestSubscriberReceives(t *testing.T) {
	h := NewHub()
	ch, _, cancel := h.Subscribe(0)
	h.Publish(TypeSystem, map[string]string{"kind": "quitting"})
	ev := <-ch
	if ev.Type != TypeSystem {
		t.Fatalf("got %s", ev.Type)
	}
	cancel()
	cancel() // idempotent
}
