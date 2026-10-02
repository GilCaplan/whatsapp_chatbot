package wa

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"testing"
	"time"

	"whatsappdoppel/internal/events"
	"whatsappdoppel/internal/model"
)

func TestFakeChats(t *testing.T) {
	ctx := context.Background()
	f := NewFake(events.NewHub())
	defer f.Close()

	items, total, err := f.ListChats(ctx, model.ChatQuery{Tab: "recent"})
	if err != nil || len(items) == 0 || !items[0].IsSelf || items[0].Key != "dm:"+FakeSelfPhone || items[0].Name != SelfChatName {
		t.Fatalf("recent: err=%v total=%d first=%+v", err, total, items)
	}
	for i := 2; i < len(items); i++ {
		if items[i].LastMessageAt.After(*items[i-1].LastMessageAt) {
			t.Errorf("recent not newest-first at %d", i)
		}
	}
	_, total, _ = f.ListChats(ctx, model.ChatQuery{Tab: "contacts"})
	if total != 30 {
		t.Errorf("contacts total = %d, want 30", total)
	}
	groups, total, _ := f.ListChats(ctx, model.ChatQuery{Tab: "groups"})
	if total != 3 || groups[0].ParticipantCount == 0 {
		t.Errorf("groups total = %d %+v", total, groups)
	}
	found, total, _ := f.ListChats(ctx, model.ChatQuery{Tab: "contacts", Q: "dana"})
	if total != 1 || !strings.HasPrefix(found[0].Name, "Dana") {
		t.Errorf("search dana: %d %+v", total, found)
	}
	page, total, _ := f.ListChats(ctx, model.ChatQuery{Tab: "contacts", Limit: 10, Offset: 25})
	if total != 30 || len(page) != 5 {
		t.Errorf("paging: total=%d len=%d", total, len(page))
	}
	if _, _, err := f.ListChats(ctx, model.ChatQuery{Tab: "bogus"}); err == nil {
		t.Error("bogus tab: want error")
	}

	// Deterministic across instances.
	g := NewFake(nil)
	a, _, _ := f.ListChats(ctx, model.ChatQuery{Tab: "contacts", Limit: 100})
	b, _, _ := g.ListChats(ctx, model.ChatQuery{Tab: "contacts", Limit: 100})
	for i := range a {
		if a[i].Key != b[i].Key || a[i].Name != b[i].Name {
			t.Fatalf("non-deterministic contact %d: %+v vs %+v", i, a[i], b[i])
		}
	}
}

func TestFakeResolveChat(t *testing.T) {
	ctx := context.Background()
	f := NewFake(nil)
	contacts, _, _ := f.ListChats(ctx, model.ChatQuery{Tab: "contacts", Limit: 1})
	c := contacts[0]
	for _, in := range []string{c.JID, c.Key, c.AltJID, "+" + c.Phone} {
		got, err := f.ResolveChat(ctx, in)
		if err != nil || got.Key != c.Key || got.Name != c.Name || got.JID != c.JID || got.AltJID != c.AltJID {
			t.Errorf("ResolveChat(%q) = %+v, %v", in, got, err)
		}
	}
	self, err := f.ResolveChat(ctx, FakeSelfPhone)
	if err != nil || !self.IsSelf || self.Name != SelfChatName {
		t.Errorf("self: %+v %v", self, err)
	}
	g, err := f.ResolveChat(ctx, "120363000000000002@g.us")
	if err != nil || g.Kind != "group" || g.Name != "Family" {
		t.Errorf("group: %+v %v", g, err)
	}
	unknown, err := f.ResolveChat(ctx, "4479001234567")
	if err != nil || unknown.Key != "dm:4479001234567" || unknown.Name != "+4479001234567" {
		t.Errorf("unknown: %+v %v", unknown, err)
	}
	if lid, err := f.ResolveChat(ctx, "777@lid"); err != nil || lid.Key != "lid:777" {
		t.Errorf("lid: %+v %v", lid, err)
	}
	if _, err := f.ResolveChat(ctx, "nobody"); err == nil {
		t.Error("want error for garbage")
	}
}

func TestFakeSendInjectAndPair(t *testing.T) {
	ctx := context.Background()
	hub := events.NewHub()
	sub, _, cancel := hub.Subscribe(0)
	defer cancel()
	f := NewFake(hub)
	defer f.Close()
	if err := f.Start(ctx); err != nil {
		t.Fatal(err)
	}
	if st := f.Status(); st.State != model.WAConnected || st.Me == nil || st.Me.PushName != "You" || st.Me.Phone != FakeSelfPhone {
		t.Fatalf("status %+v", st)
	}
	if got := f.OwnJIDs(); len(got) != 2 {
		t.Errorf("OwnJIDs = %v", got)
	}
	if data, ct, err := f.Avatar(ctx, FakeSelfPhone); data != nil || ct != "" || err != nil {
		t.Error("fake avatar should be empty")
	}

	if err := f.Send(ctx, "dm:15550100001", "hello"); err != nil {
		t.Fatal(err)
	}
	if s := f.Sent(); len(s) != 1 || s[0].Text != "hello" {
		t.Errorf("sent = %+v", s)
	}

	var mu sync.Mutex
	var got []model.Incoming
	f.SetMessageHandler(func(in model.Incoming) { mu.Lock(); got = append(got, in); mu.Unlock() })
	f.Inject(model.Incoming{ChatKey: "group:120363000000000001", Text: "hi all"})
	mu.Lock()
	if len(got) != 1 || !got[0].IsGroup || got[0].ChatJID != "120363000000000001@g.us" || got[0].Timestamp.IsZero() || got[0].MessageID == "" {
		t.Errorf("injected = %+v", got)
	}
	mu.Unlock()

	if err := f.Pair(); err != nil {
		t.Fatal(err)
	}
	if f.Status().State != model.WAAwaitingQR || f.CurrentQR() == nil {
		t.Fatalf("after Pair: %+v", f.Status())
	}
	sawQR := false
	deadline := time.After(fakePairDelay + 3*time.Second)
	for f.Status().State != model.WAConnected {
		select {
		case ev := <-sub:
			if ev.Type == events.TypeWAQR {
				var fr model.QRFrame
				if err := json.Unmarshal(ev.Data, &fr); err != nil || !strings.HasPrefix(fr.PNG, "data:image/png;base64,") || fr.ExpiresInSec <= 0 {
					t.Errorf("bad QR frame: %v %+v", err, fr)
				}
				sawQR = true
			}
		case <-deadline:
			t.Fatalf("fake pairing never connected: %+v", f.Status())
		}
	}
	if !sawQR {
		t.Error("no wa.qr event published")
	}
	if f.CurrentQR() != nil {
		t.Error("QR should be cleared once connected")
	}

	f.Disconnect()
	if err := f.Send(ctx, "dm:15550100001", "x"); err == nil {
		t.Error("send while disconnected should fail")
	}
	if err := f.Reconnect(); err != nil || f.Status().State != model.WAConnected {
		t.Error("reconnect failed")
	}
}
