package wa

import (
	"context"
	"testing"

	"whatsappdoppel/internal/events"
	"whatsappdoppel/internal/model"
)

func TestFakeRecordsReadsReactionsQuotes(t *testing.T) {
	ctx := context.Background()
	f := NewFake(events.NewHub())
	defer f.Close()
	chat := "15550100001@s.whatsapp.net"
	if err := f.MarkRead(ctx, chat, "", []string{"A", "B"}); err != nil {
		t.Fatal(err)
	}
	if err := f.MarkRead(ctx, chat, "", nil); err != nil {
		t.Fatal(err)
	}
	if r := f.Reads(); len(r) != 1 || r[0].ChatJID != chat || len(r[0].IDs) != 2 {
		t.Errorf("reads = %+v", r)
	}
	if err := f.React(ctx, "120363000000000001@g.us", chat, "A", "👍"); err != nil {
		t.Fatal(err)
	}
	if r := f.Reactions(); len(r) != 1 || r[0].Emoji != "👍" || r[0].MessageID != "A" || r[0].SenderJID != chat {
		t.Errorf("reactions = %+v", r)
	}
	q := model.QuoteRef{MessageID: "A", SenderJID: chat, Text: "hi"}
	if err := f.SendQuoted(ctx, chat, "hello back", q); err != nil {
		t.Fatal(err)
	}
	if err := f.Send(ctx, chat, "plain"); err != nil {
		t.Fatal(err)
	}
	s := f.Sent()
	if len(s) != 2 || s[0].Quote == nil || s[0].Quote.MessageID != "A" || s[1].Quote != nil {
		t.Errorf("sent = %+v", s)
	}
	if err := f.MarkRead(ctx, "", "", []string{"x"}); err == nil {
		t.Error("bad chat should fail")
	}
}

func TestBuildMessage(t *testing.T) {
	q := &model.QuoteRef{MessageID: "3EB0ABC", SenderJID: "972501234567:12@s.whatsapp.net", Text: "pizza?"}
	// quote only
	m := buildMessage(model.OutMessage{Text: "sure!", Quote: q})
	ext := m.GetExtendedTextMessage()
	ci := ext.GetContextInfo()
	if ext.GetText() != "sure!" || ci.GetStanzaID() != "3EB0ABC" || ci.GetParticipant() != "972501234567@s.whatsapp.net" ||
		ci.GetQuotedMessage().GetConversation() != "pizza?" || m.GetConversation() != "" || len(ci.GetMentionedJID()) != 0 {
		t.Errorf("quoted = %+v", m)
	}
	if p := buildMessage(model.OutMessage{Text: "x", Quote: &model.QuoteRef{MessageID: "1"}}).GetExtendedTextMessage().GetContextInfo().Participant; p != nil {
		t.Errorf("no sender → no participant, got %v", *p)
	}
	// plain (an empty quote counts as none)
	if m := buildMessage(model.OutMessage{Text: "hi", Quote: &model.QuoteRef{}}); m.GetConversation() != "hi" || m.GetExtendedTextMessage() != nil {
		t.Errorf("plain = %+v", m)
	}
	// mentions only
	m = buildMessage(model.OutMessage{Text: "@200000000000001 hi", Mentions: []string{"200000000000001@lid"}})
	ci = m.GetExtendedTextMessage().GetContextInfo()
	if m.GetExtendedTextMessage().GetText() != "@200000000000001 hi" || ci.GetStanzaID() != "" || len(ci.GetMentionedJID()) != 1 || ci.GetMentionedJID()[0] != "200000000000001@lid" {
		t.Errorf("mentions = %+v", m)
	}
	// both
	m = buildMessage(model.OutMessage{Text: "@972501234567 yes", Quote: q, Mentions: []string{"972501234567@s.whatsapp.net"}})
	ci = m.GetExtendedTextMessage().GetContextInfo()
	if ci.GetStanzaID() != "3EB0ABC" || len(ci.GetMentionedJID()) != 1 {
		t.Errorf("both = %+v", m)
	}
}
