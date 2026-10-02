package wa

import (
	"errors"
	"fmt"
	"testing"
	"time"

	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/proto/waHistorySync"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
	"google.golang.org/protobuf/proto"

	"whatsappdoppel/internal/model"
)

var (
	pnAlice  = types.NewJID("972501112222", types.DefaultUserServer)
	lidAlice = types.NewJID("111111111111111", types.HiddenUserServer)
	pnMe     = types.NewJID("15550000000", types.DefaultUserServer)
	lidMe    = types.NewJID("999999999999999", types.HiddenUserServer)
	groupJID = types.NewJID("120363001234567890", types.GroupServer)
	lidBob   = types.NewJID("222222222222222", types.HiddenUserServer) // no known phone
)

func testResolver() resolver {
	return resolver{
		self: selfIDs{PN: pnMe, LID: lidMe},
		pnForLID: func(j types.JID) types.JID {
			if j.User == lidAlice.User {
				return pnAlice
			}
			return types.JID{}
		},
		lidForPN: func(j types.JID) types.JID {
			if j.User == pnAlice.User {
				return lidAlice
			}
			return types.JID{}
		},
	}
}

func textMsg(s string) *waE2E.Message { return &waE2E.Message{Conversation: proto.String(s)} }

func msgEvent(chat, sender types.JID, fromMe bool, m *waE2E.Message) *events.Message {
	return &events.Message{
		Info: types.MessageInfo{
			MessageSource: types.MessageSource{Chat: chat, Sender: sender, IsFromMe: fromMe, IsGroup: chat.Server == types.GroupServer},
			ID:            "MSG1",
			PushName:      "Alice",
			Timestamp:     time.Unix(1700000000, 0),
		},
		Message: m,
	}
}

func TestNormalizeDMs(t *testing.T) {
	r := testResolver()
	adSender := pnAlice
	adSender.Device = 3

	tests := []struct {
		name              string
		evt               *events.Message
		key, chatJID, alt string
		fromMe            bool
	}{
		{
			name: "phone chat, lid from store",
			evt:  msgEvent(pnAlice, adSender, false, textMsg("hi")),
			key:  "dm:972501112222", chatJID: pnAlice.String(), alt: lidAlice.String(),
		},
		{
			name: "lid chat resolved via SenderAlt",
			evt: func() *events.Message {
				e := msgEvent(types.NewJID("333", types.HiddenUserServer), types.NewJID("333", types.HiddenUserServer), false, textMsg("hi"))
				e.Info.SenderAlt = types.NewJID("972509999999", types.DefaultUserServer)
				return e
			}(),
			key: "dm:972509999999", chatJID: "333@lid", alt: "972509999999@s.whatsapp.net",
		},
		{
			name: "lid chat from me resolved via RecipientAlt",
			evt: func() *events.Message {
				e := msgEvent(types.NewJID("444", types.HiddenUserServer), lidMe, true, textMsg("yo"))
				e.Info.RecipientAlt = types.NewJID("972508888888", types.DefaultUserServer)
				e.Info.SenderAlt = pnMe // must be ignored for messages from me
				return e
			}(),
			key: "dm:972508888888", chatJID: "444@lid", alt: "972508888888@s.whatsapp.net", fromMe: true,
		},
		{
			name: "lid chat resolved via LID store",
			evt:  msgEvent(lidAlice, lidAlice, false, textMsg("hi")),
			key:  "dm:972501112222", chatJID: lidAlice.String(), alt: pnAlice.String(),
		},
		{
			name: "unresolved lid",
			evt:  msgEvent(lidBob, lidBob, false, textMsg("hi")),
			key:  "lid:222222222222222", chatJID: lidBob.String(), alt: "",
		},
		{
			name: "self chat on lid",
			evt:  msgEvent(lidMe, lidMe, true, textMsg("1 hey")),
			key:  "dm:15550000000", chatJID: lidMe.String(), alt: pnMe.String(), fromMe: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			in, ok := normalizeMessage(tt.evt, r)
			if !ok {
				t.Fatal("message unexpectedly skipped")
			}
			if in.ChatKey != tt.key || in.ChatJID != tt.chatJID || in.AltJID != tt.alt {
				t.Errorf("got key=%q chat=%q alt=%q, want %q %q %q", in.ChatKey, in.ChatJID, in.AltJID, tt.key, tt.chatJID, tt.alt)
			}
			if in.IsGroup || in.IsFromMe != tt.fromMe {
				t.Errorf("IsGroup=%v IsFromMe=%v", in.IsGroup, in.IsFromMe)
			}
		})
	}
}

func TestNormalizeGroupAndFields(t *testing.T) {
	sender := pnAlice
	sender.Device = 7
	m := &waE2E.Message{ExtendedTextMessage: &waE2E.ExtendedTextMessage{
		Text:        proto.String("hey @bob"),
		ContextInfo: &waE2E.ContextInfo{MentionedJID: []string{"222222222222222@lid"}},
	}}
	in, ok := normalizeMessage(msgEvent(groupJID, sender, false, m), testResolver())
	if !ok {
		t.Fatal("skipped")
	}
	want := model.Incoming{
		ChatKey: "group:120363001234567890", ChatJID: groupJID.String(), IsGroup: true,
		SenderJID: pnAlice.String(), PushName: "Alice", Text: "hey @bob",
		Timestamp: time.Unix(1700000000, 0), MessageID: "MSG1",
	}
	if in.ChatKey != want.ChatKey || in.ChatJID != want.ChatJID || in.AltJID != "" || !in.IsGroup ||
		in.SenderJID != want.SenderJID || in.PushName != want.PushName || in.Text != want.Text ||
		!in.Timestamp.Equal(want.Timestamp) || in.MessageID != want.MessageID {
		t.Errorf("got %+v", in)
	}
	if len(in.MentionedJIDs) != 1 || in.MentionedJIDs[0] != "222222222222222@lid" {
		t.Errorf("mentions = %v", in.MentionedJIDs)
	}
}

func TestMessageText(t *testing.T) {
	tests := []struct {
		m    *waE2E.Message
		want string
	}{
		{textMsg("plain"), "plain"},
		{&waE2E.Message{ExtendedTextMessage: &waE2E.ExtendedTextMessage{Text: proto.String("ext")}}, "ext"},
		{&waE2E.Message{ImageMessage: &waE2E.ImageMessage{Caption: proto.String("look")}}, "[photo] look"},
		{&waE2E.Message{VideoMessage: &waE2E.VideoMessage{Caption: proto.String("watch")}}, "[video] watch"},
		{&waE2E.Message{ImageMessage: &waE2E.ImageMessage{}}, ""},
		{textMsg("   "), ""},
	}
	for _, tt := range tests {
		if got := messageText(tt.m); got != tt.want {
			t.Errorf("messageText = %q, want %q", got, tt.want)
		}
	}
}

func TestNormalizeSkips(t *testing.T) {
	r := testResolver()
	edit := msgEvent(pnAlice, pnAlice, false, textMsg("edited"))
	edit.IsEdit = true
	tests := map[string]*events.Message{
		"status":     msgEvent(types.StatusBroadcastJID, pnAlice, false, textMsg("story")),
		"broadcast":  msgEvent(types.NewJID("12345", types.BroadcastServer), pnAlice, false, textMsg("bc")),
		"newsletter": msgEvent(types.NewJID("12345", types.NewsletterServer), pnAlice, false, textMsg("news")),
		"reaction":   msgEvent(pnAlice, pnAlice, false, &waE2E.Message{ReactionMessage: &waE2E.ReactionMessage{Text: proto.String("👍")}}),
		"protocol":   msgEvent(pnAlice, pnAlice, false, &waE2E.Message{ProtocolMessage: &waE2E.ProtocolMessage{}}),
		"edit":       edit,
		"no text":    msgEvent(pnAlice, pnAlice, false, &waE2E.Message{}), // (a captionless photo is "[photo]" since wave 3)
		"nil msg":    msgEvent(pnAlice, pnAlice, false, nil),
	}
	for name, evt := range tests {
		if _, ok := normalizeMessage(evt, r); ok {
			t.Errorf("%s: not skipped", name)
		}
	}
}

func TestParseTarget(t *testing.T) {
	tests := []struct {
		in, want string
	}{
		{"972501112222@s.whatsapp.net", "972501112222@s.whatsapp.net"},
		{"972501112222:12@s.whatsapp.net", "972501112222@s.whatsapp.net"},
		{"111111111111111@lid", "111111111111111@lid"},
		{"120363001234567890@g.us", "120363001234567890@g.us"},
		{"dm:972501112222", "972501112222@s.whatsapp.net"},
		{"group:120363001234567890", "120363001234567890@g.us"},
		{"lid:111111111111111", "111111111111111@lid"},
		{"+972 50-111-2222", "972501112222@s.whatsapp.net"},
		{" 972501112222 ", "972501112222@s.whatsapp.net"},
	}
	for _, tt := range tests {
		got, err := parseTarget(tt.in)
		if err != nil || got.String() != tt.want {
			t.Errorf("parseTarget(%q) = %v, %v; want %s", tt.in, got, err, tt.want)
		}
	}
	for _, bad := range []string{"", "dana", "dm:", "group:", "123", "@s.whatsapp.net"} {
		if _, err := parseTarget(bad); !errors.Is(err, errBadJID) {
			t.Errorf("parseTarget(%q) err = %v, want errBadJID", bad, err)
		}
	}
}

func TestEntryFromConversation(t *testing.T) {
	r := testResolver()
	conv := func(id, pn, lid, name string, ts uint64, archived bool) *waHistorySync.Conversation {
		c := &waHistorySync.Conversation{ID: proto.String(id), ConversationTimestamp: proto.Uint64(ts), Archived: proto.Bool(archived)}
		if pn != "" {
			c.PnJID = proto.String(pn)
		}
		if lid != "" {
			c.LidJID = proto.String(lid)
		}
		if name != "" {
			c.Name = proto.String(name)
		}
		return c
	}
	tests := []struct {
		conv *waHistorySync.Conversation
		want recentEntry
		ok   bool
	}{
		{conv("333@lid", "972507777777@s.whatsapp.net", "", "", 10, false),
			recentEntry{Key: "dm:972507777777", Kind: "dm", JID: "972507777777@s.whatsapp.net", AltJID: "333@lid", TS: 10}, true},
		{conv(pnAlice.String(), "", "", "", 20, true),
			recentEntry{Key: "dm:972501112222", Kind: "dm", JID: pnAlice.String(), AltJID: lidAlice.String(), TS: 20, Archived: true}, true},
		{conv(groupJID.String(), "", "", "Friends", 30, false),
			recentEntry{Key: "group:120363001234567890", Kind: "group", JID: groupJID.String(), Name: "Friends", TS: 30}, true},
		{conv(lidBob.String(), "", "", "", 5, false),
			recentEntry{Key: "lid:222222222222222", Kind: "dm", JID: lidBob.String(), TS: 5}, true},
		{conv("status@broadcast", "", "", "", 1, false), recentEntry{}, false},
		{conv("1234@newsletter", "", "", "", 1, false), recentEntry{}, false},
		{conv("garbage", "", "", "", 1, false), recentEntry{}, false},
	}
	for _, tt := range tests {
		got, ok := entryFromConversation(tt.conv, r)
		if ok != tt.ok || got != tt.want {
			t.Errorf("%s: got %+v,%v want %+v,%v", tt.conv.GetID(), got, ok, tt.want, tt.ok)
		}
	}
}

func TestFilterChats(t *testing.T) {
	self := selfItem(selfIDs{PN: pnMe, LID: lidMe})
	var items []model.ChatItem
	for i := 0; i < 20; i++ {
		items = append(items, model.ChatItem{Key: fmt.Sprintf("dm:97250000%04d", i), Kind: "dm", Name: fmt.Sprintf("Person %02d", i), Phone: fmt.Sprintf("97250000%04d", i)})
	}
	items = append(items, model.ChatItem{Key: "group:1", Kind: "group", Name: "Book Club"}, model.ChatItem{Key: "group:2", Kind: "group", Name: "שבת שלום"})

	got, total := filterChats([]model.ChatItem{self}, items, model.ChatQuery{})
	if total != 23 || len(got) != 23 || !got[0].IsSelf {
		t.Fatalf("no filter: total=%d len=%d first=%+v", total, len(got), got[0])
	}
	got, total = filterChats([]model.ChatItem{self}, items, model.ChatQuery{Q: "BOOK"})
	if total != 2 || got[1].Name != "Book Club" {
		t.Errorf("q=BOOK: total=%d %+v", total, got)
	}
	got, total = filterChats(nil, items, model.ChatQuery{Q: "שבת"})
	if total != 1 || got[0].Key != "group:2" {
		t.Errorf("hebrew: total=%d", total)
	}
	got, total = filterChats(nil, items, model.ChatQuery{Q: "+972 50 000 0013"})
	if total != 1 || got[0].Name != "Person 13" {
		t.Errorf("phone search: total=%d %+v", total, got)
	}
	got, total = filterChats([]model.ChatItem{self}, items, model.ChatQuery{Kind: "group"})
	if total != 2 || got[0].Kind != "group" {
		t.Errorf("kind=group: total=%d", total)
	}
	got, total = filterChats(nil, items, model.ChatQuery{Kind: "dm", Limit: 5, Offset: 18})
	if total != 20 || len(got) != 2 || got[0].Name != "Person 18" {
		t.Errorf("paging: total=%d len=%d", total, len(got))
	}
	got, total = filterChats(nil, items, model.ChatQuery{Offset: 100})
	if total != 22 || got == nil || len(got) != 0 {
		t.Errorf("offset past end: total=%d got=%v", total, got)
	}
}

func TestSorting(t *testing.T) {
	items := []model.ChatItem{{Key: "a", Name: "+972"}, {Key: "b", Name: "zoe"}, {Key: "c", Name: "Adam"}}
	sortByName(items)
	if items[0].Name != "Adam" || items[1].Name != "zoe" || items[2].Name != "+972" {
		t.Errorf("sortByName: %+v", items)
	}
	t1, t2 := time.Unix(100, 0), time.Unix(200, 0)
	rec := []model.ChatItem{{Key: "old", LastMessageAt: &t1}, {Key: "none"}, {Key: "new", LastMessageAt: &t2}}
	sortByRecent(rec)
	if rec[0].Key != "new" || rec[1].Key != "old" || rec[2].Key != "none" {
		t.Errorf("sortByRecent: %v %v %v", rec[0].Key, rec[1].Key, rec[2].Key)
	}
}

func TestMessageContentMedia(t *testing.T) {
	tests := []struct {
		m           *waE2E.Message
		text, media string
	}{
		{textMsg("plain"), "plain", ""},
		{&waE2E.Message{ImageMessage: &waE2E.ImageMessage{}}, "[photo]", model.MediaImage},
		{&waE2E.Message{ImageMessage: &waE2E.ImageMessage{Caption: proto.String("look")}}, "[photo] look", model.MediaImage},
		{&waE2E.Message{VideoMessage: &waE2E.VideoMessage{}}, "[video]", model.MediaVideo},
		{&waE2E.Message{AudioMessage: &waE2E.AudioMessage{PTT: proto.Bool(true)}}, "[voice note]", model.MediaAudio},
		{&waE2E.Message{AudioMessage: &waE2E.AudioMessage{}}, "[audio]", model.MediaAudio},
		{&waE2E.Message{StickerMessage: &waE2E.StickerMessage{}}, "[sticker]", model.MediaSticker},
		{&waE2E.Message{}, "", ""},
	}
	for _, tt := range tests {
		if text, media := messageContent(tt.m); text != tt.text || media != tt.media {
			t.Errorf("messageContent = %q %q, want %q %q", text, media, tt.text, tt.media)
		}
	}
	in, ok := normalizeMessage(msgEvent(pnAlice, pnAlice, false, &waE2E.Message{ImageMessage: &waE2E.ImageMessage{}}), testResolver())
	if !ok || in.Text != "[photo]" || in.Media != model.MediaImage {
		t.Errorf("normalize image = %+v %v", in, ok)
	}
}
