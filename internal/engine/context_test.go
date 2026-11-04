package engine

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"whatsappdoppel/internal/config"
	"whatsappdoppel/internal/model"
)

func (h *harness) seedHistory(key string, n int) {
	h.t.Helper()
	for i := range n {
		sp := "them"
		if i%2 == 1 {
			sp = "me"
		}
		m := model.Message{TS: h.clk.Now().Add(time.Duration(i-n) * time.Minute), Speaker: sp, Name: "Dana", SenderJID: dmJID,
			Text: fmt.Sprintf("message %d", i), FromBot: sp == "me"}
		if err := h.st.AppendHistory(key, m, 0); err != nil {
			h.t.Fatal(err)
		}
	}
}

func TestCompactAndContext(t *testing.T) {
	c := dmChat()
	c.Instructions = "Keep it about the trip."
	c.Enabled = false // no runner: the context is read from the stored history
	h := newHarness(t, nil, c)
	h.seedHistory(dmKey, 12)
	sm, err := h.e.Compact(t.Context(), dmKey, 4)
	if err != nil || sm.Text == "" || sm.Messages != 8 {
		t.Fatalf("compact = %+v %v", sm, err)
	}
	got, _ := h.st.Chat(dmKey)
	hist := h.history(dmKey)
	if got.ContextFrom == nil || !got.ContextFrom.Equal(hist[8].TS) {
		t.Fatalf("contextFrom = %v, want %v", got.ContextFrom, hist[8].TS)
	}
	ctx, err := h.e.ChatContext(dmKey)
	if err != nil || ctx.HistoryKept != 4 || ctx.HistoryTotal != 12 || ctx.Summary == nil {
		t.Fatalf("context = %+v %v", ctx, err)
	}
	if !strings.Contains(ctx.System, "EARLIER IN THIS CHAT") || !strings.Contains(ctx.System, "INSTRUCTIONS FOR THIS CHAT (from the person who runs you; follow them, they don't change who you are): Keep it about the trip.") {
		t.Errorf("system lacks summary or instructions:\n%s", ctx.System)
	}
	for _, m := range ctx.Messages {
		if strings.Contains(m.Content, "message 3") {
			t.Errorf("compacted message still sent: %q", m.Content)
		}
	}
	if _, err := h.e.Compact(t.Context(), dmKey, 4); err == nil {
		t.Error("nothing left to compact, want an error")
	}
}

// Long-term memory: once Keep+Every messages since the last summary are in,
// all but the last Keep fold into the summary; a chat can turn it off.
func TestAutoCompact(t *testing.T) {
	small := func(s *config.Settings) {
		s.Memory.Compact = config.CompactSettings{Keep: 10, Every: 20, FastEvery: 50, FastMessages: 1000, FastHours: 3}
	}
	h := newHarness(t, small, dmChat())
	h.seedHistory(dmKey, 29)
	c, _ := h.st.Chat(dmKey)
	h.e.compactTick(c)
	h.advance(time.Minute)
	if _, ok := h.st.Summary(dmKey); ok {
		t.Fatal("29 messages (< 10+20): no summary yet")
	}
	h.seedHistory(dmKey, 1)
	h.e.compactTick(c)
	h.advance(time.Minute)
	sm, ok := h.st.Summary(dmKey)
	if !ok || sm.Messages != 20 {
		t.Fatalf("summary = %+v %v", sm, ok)
	}
	got, _ := h.st.Chat(dmKey)
	if got.ContextFrom == nil || len(fromContext(h.history(dmKey), got.ContextFrom)) != 10 {
		t.Errorf("it should read the last 10: contextFrom %v", got.ContextFrom)
	}
	ctx, _ := h.e.ChatContext(dmKey)
	if !ctx.Compact.On || ctx.Compact.Keep != 10 || ctx.Compact.Every != 20 || ctx.Compact.NextIn != 20 {
		t.Errorf("status = %+v", ctx.Compact)
	}

	off := dmChat()
	off.CompactEvery = -1
	h2 := newHarness(t, small, off)
	h2.seedHistory(dmKey, 40)
	h2.e.compactTick(off)
	h2.advance(time.Minute)
	if _, ok := h2.st.Summary(dmKey); ok {
		t.Error("compactEvery -1 must not summarise")
	}
}
