package wa

import (
	"context"
	"encoding/base64"
	"fmt"
	"strings"
	"sync"
	"time"

	"go.mau.fi/whatsmeow/types"
	waLog "go.mau.fi/whatsmeow/util/log"

	"whatsappdoppel/internal/contract"
	"whatsappdoppel/internal/events"
	"whatsappdoppel/internal/model"
)

// Fake identities.
const (
	FakeSelfPhone = "15550000000"
	FakeSelfLID   = "100000000000000"
	fakePairDelay = 3 * time.Second
)

var _ contract.WhatsApp = (*Fake)(nil)

// FakeSent is a message recorded by Fake.SendMessage (and its Send /
// SendQuoted wrappers).
type FakeSent struct {
	ID       string // the message id returned in SendResult
	JID      string
	Text     string
	At       time.Time
	Quote    *model.QuoteRef
	Mentions []string
}

// FakeEdit is one Fake.EditMessage call.
type FakeEdit struct {
	JID, ID, Text string
	Mentions      []string
}

// FakeRead is one Fake.MarkRead call.
type FakeRead struct {
	ChatJID, SenderJID string
	IDs                []string
}

// FakeReaction is one Fake.React call.
type FakeReaction struct {
	ChatJID, SenderJID, MessageID, Emoji string
}

// Fake implements contract.WhatsApp without a network (--fake-wa): a linked
// "me", 30 synthetic contacts and 3 groups with deterministic names.
type Fake struct {
	hub  *events.Hub
	log  waLog.Logger
	self selfIDs

	contacts []model.ChatItem
	groups   []model.ChatItem
	recent   []model.ChatItem
	rosters  map[string][]model.Participant // group JID → members

	mu      sync.Mutex
	status  model.WAStatus
	handler func(model.Incoming)
	sent    []FakeSent
	edits   []FakeEdit
	seq     int
	reads   []FakeRead
	reacts  []FakeReaction
	qrPNG   []byte
	timer   *time.Timer
	closed  bool
}

var fakeFirst = []string{"Dana", "Noam", "Maya", "Eitan", "Shira", "Omer", "Tamar", "Yoni", "Lior", "Gal",
	"Avi", "Roni", "Tal", "Ido", "Neta", "Amit", "Hila", "Uri", "Michal", "Ben",
	"Alice", "Bob", "Carol", "Dave", "Erin", "Frank", "Grace", "Heidi", "Ivan", "Judy"}

var fakeLast = []string{"Cohen", "Levi", "Mizrahi", "Peretz", "Biton", "Friedman", "Shapiro", "Katz", "Adler", "Stone"}

var fakeGroups = []struct {
	name    string
	members int
}{{"Weekend Crew", 6}, {"Family", 9}, {"Book Club", 12}}

// NewFake returns a Fake that is already "connected".
func NewFake(hub *events.Hub) *Fake {
	f := &Fake{
		hub: hub,
		log: newLogger("fake-wa", levelInfo),
		self: selfIDs{
			PN:  types.NewJID(FakeSelfPhone, types.DefaultUserServer),
			LID: types.NewJID(FakeSelfLID, types.HiddenUserServer),
		},
		status: model.WAStatus{State: model.WAConnected, Since: time.Now()},
	}
	base := time.Now().Truncate(time.Minute)
	for i, first := range fakeFirst {
		phone := fmt.Sprintf("1555010%04d", i+1)
		pn := types.NewJID(phone, types.DefaultUserServer)
		lid := types.NewJID(fmt.Sprintf("2000000000%05d", i+1), types.HiddenUserServer)
		f.contacts = append(f.contacts, dmItem(prefixDM+phone, pn, lid, first+" "+fakeLast[i%len(fakeLast)], f.self))
	}
	f.rosters = map[string][]model.Participant{}
	for i, g := range fakeGroups {
		id := fmt.Sprintf("12036300000000%04d", i+1)
		jid := id + "@" + types.GroupServer
		f.groups = append(f.groups, model.ChatItem{
			Key: prefixGroup + id, Kind: "group", JID: jid,
			Name: g.name, ParticipantCount: g.members,
		})
		f.rosters[jid] = f.fakeRoster(i, g.name, g.members)
	}
	sortByName(f.contacts)
	sortByName(f.groups)
	// Recent: every group plus every third contact, spaced an hour apart.
	var recent []model.ChatItem
	recent = append(recent, f.groups...)
	for i := 0; i < len(f.contacts); i += 3 {
		recent = append(recent, f.contacts[i])
	}
	for i := range recent {
		recent[i].LastMessageAt = timePtr(base.Add(-time.Duration(i) * time.Hour))
	}
	f.recent = recent
	return f
}

// fakeRoster builds group i's members from the (unsorted) contacts: you plus
// contacts[(i*4+k) % 30]. "Book Club" is lid-addressed and has two members
// called Lior, so name disambiguation shows up in --fake-wa.
func (f *Fake) fakeRoster(i int, name string, members int) []model.Participant {
	lidAddressed := name == "Book Club"
	self := model.Participant{JID: f.self.PN.String(), Phone: FakeSelfPhone, LID: f.self.LID.String(), Name: "You", IsSelf: true, IsAdmin: i == 1}
	if lidAddressed {
		self.JID = f.self.LID.String()
	}
	out := []model.Participant{self}
	for k := 0; k < members-1; k++ {
		c := f.contacts[(i*4+k)%len(f.contacts)]
		p := model.Participant{JID: c.JID, Phone: c.Phone, LID: c.AltJID, Name: c.Name, IsAdmin: k == 0}
		if lidAddressed && k == members-2 {
			p = model.Participant{JID: "15550200001@" + types.DefaultUserServer, Phone: "15550200001", LID: "300000000000001@" + types.HiddenUserServer, Name: "Lior Stone"}
		}
		if lidAddressed {
			p.JID = p.LID
		}
		out = append(out, p)
	}
	return out
}

// Start publishes the (connected) status.
func (f *Fake) Start(ctx context.Context) error {
	f.publish(events.TypeWAStatus, f.Status())
	return nil
}

func (f *Fake) Status() model.WAStatus {
	f.mu.Lock()
	defer f.mu.Unlock()
	st := f.status
	if st.State == model.WAConnected || st.State == model.WADisconnected {
		st.Me = &model.WAMe{
			JID: f.self.PN.String(), LID: f.self.LID.String(), PushName: "You", Phone: FakeSelfPhone,
			AvatarURL: "/api/wa/avatar?jid=" + f.self.PN.String(),
		}
	}
	return st
}

func (f *Fake) setState(state, lastErr string) {
	f.mu.Lock()
	if f.closed {
		f.mu.Unlock()
		return
	}
	prev := f.status.State
	f.status = model.WAStatus{State: state, LastError: lastErr, Since: time.Now()}
	f.mu.Unlock()
	st := f.Status()
	f.publish(events.TypeWAStatus, st)
	if prev != state {
		if text := stateActivityText(st); text != "" && f.hub != nil {
			f.hub.Activity(model.ActivityEvent{Type: model.ActWAStatus, Text: text, Meta: map[string]any{"state": state}})
		}
	}
}

func (f *Fake) publish(typ string, data any) {
	if f.hub != nil {
		f.hub.Publish(typ, data)
	}
}

// Pair simulates a QR flow: publish a QR frame, then connect after ~3s.
func (f *Fake) Pair() error {
	png, err := qrPNG("doppel-fake-pairing-" + time.Now().Format(time.RFC3339Nano))
	if err != nil {
		return err
	}
	f.mu.Lock()
	if f.closed {
		f.mu.Unlock()
		return errClosed
	}
	if f.timer != nil {
		f.timer.Stop()
	}
	f.qrPNG = png
	f.timer = time.AfterFunc(fakePairDelay, func() {
		f.mu.Lock()
		f.qrPNG, f.timer = nil, nil
		f.mu.Unlock()
		f.setState(model.WAPairing, "")
		f.setState(model.WAConnected, "")
	})
	f.mu.Unlock()
	f.setState(model.WAAwaitingQR, "")
	f.publish(events.TypeWAQR, model.QRFrame{
		PNG:          "data:image/png;base64," + base64.StdEncoding.EncodeToString(png),
		ExpiresInSec: 60,
	})
	return nil
}

func (f *Fake) stopPairing() {
	f.mu.Lock()
	if f.timer != nil {
		f.timer.Stop()
		f.timer = nil
	}
	f.qrPNG = nil
	f.mu.Unlock()
}

func (f *Fake) Reconnect() error {
	f.stopPairing()
	f.setState(model.WAConnected, "")
	return nil
}

func (f *Fake) Disconnect() {
	f.stopPairing()
	f.setState(model.WADisconnected, "")
}

// Logout goes to logged_out and immediately starts a simulated pairing.
func (f *Fake) Logout(ctx context.Context) error {
	f.stopPairing()
	f.setState(model.WALoggedOut, "")
	return f.Pair()
}

func (f *Fake) CurrentQR() []byte {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.qrPNG == nil {
		return nil
	}
	return append([]byte(nil), f.qrPNG...)
}

func (f *Fake) ListChats(ctx context.Context, q model.ChatQuery) ([]model.ChatItem, int, error) {
	switch strings.ToLower(q.Tab) {
	case "", "recent":
		items, total := filterChats([]model.ChatItem{selfItem(f.self)}, f.recent, q)
		return items, total, nil
	case "contacts":
		items, total := filterChats(nil, f.contacts, q)
		return items, total, nil
	case "groups":
		items, total := filterChats(nil, f.groups, q)
		return items, total, nil
	case "assigned":
		return []model.ChatItem{}, 0, nil
	}
	return nil, 0, fmt.Errorf("unknown chat tab %q", q.Tab)
}

func (f *Fake) RefreshChats(ctx context.Context) error { return nil }

// fakeLIDs maps synthetic lids back to phones.
func (f *Fake) resolver() resolver {
	return resolver{
		self: f.self,
		pnForLID: func(lid types.JID) types.JID {
			for _, c := range f.contacts {
				if c.AltJID == lid.String() {
					return types.NewJID(c.Phone, types.DefaultUserServer)
				}
			}
			return types.JID{}
		},
		lidForPN: func(pn types.JID) types.JID {
			for _, c := range f.contacts {
				if c.Phone == pn.User {
					j, _ := types.ParseJID(c.AltJID)
					return j
				}
			}
			return types.JID{}
		},
	}
}

func (f *Fake) ResolveChat(ctx context.Context, jid string) (model.ChatItem, error) {
	target, err := parseTarget(jid)
	if err != nil {
		return model.ChatItem{}, err
	}
	var it model.ChatItem
	switch {
	case target.Server == types.GroupServer:
		it = model.ChatItem{Key: prefixGroup + target.User, Kind: "group", JID: target.String(), Name: "Group"}
		for _, g := range f.groups {
			if g.Key == it.Key {
				it = g
			}
		}
	case isUserServer(target):
		key, pn, lid := f.resolver().dm(target, types.JID{})
		it = dmItem(key, pn, lid, "", f.self)
		for _, c := range f.contacts {
			if c.Key == key {
				it = c
			}
		}
		if it.IsSelf {
			it.Name = SelfChatName
		}
	default:
		return model.ChatItem{}, fmt.Errorf("%w: %q", errBadJID, jid)
	}
	for _, r := range f.recent {
		if r.Key == it.Key {
			it.LastMessageAt = r.LastMessageAt
		}
	}
	return it, nil
}

func (f *Fake) Avatar(ctx context.Context, jid string) ([]byte, string, error) { return nil, "", nil }

// SendMessage records the message (with its quote and mentions) and logs it.
// The result has a synthetic id ("FAKE00000001", …) usable with EditMessage.
func (f *Fake) SendMessage(ctx context.Context, jid string, msg model.OutMessage) (model.SendResult, error) {
	if _, err := parseTarget(jid); err != nil {
		return model.SendResult{}, err
	}
	f.mu.Lock()
	connected := f.status.State == model.WAConnected
	var res model.SendResult
	if connected {
		f.seq++
		res = model.SendResult{ID: fmt.Sprintf("FAKE%08d", f.seq), At: time.Now()}
		fs := FakeSent{ID: res.ID, JID: jid, Text: msg.Text, At: res.At, Mentions: append([]string(nil), msg.Mentions...)}
		if msg.Quote != nil && msg.Quote.MessageID != "" {
			q := *msg.Quote
			fs.Quote = &q
		}
		f.sent = append(f.sent, fs)
	}
	f.mu.Unlock()
	if !connected {
		return model.SendResult{}, ErrNotConnected
	}
	switch {
	case msg.Quote != nil && msg.Quote.MessageID != "":
		f.log.Infof("send to %s (quoting %s): %q", jid, msg.Quote.MessageID, msg.Text)
	case len(msg.Mentions) > 0:
		f.log.Infof("send to %s (mentioning %v): %q", jid, msg.Mentions, msg.Text)
	default:
		f.log.Infof("send to %s: %q", jid, msg.Text)
	}
	return res, nil
}

// EditMessage records the edit of a message sent earlier by this fake.
func (f *Fake) EditMessage(ctx context.Context, jid, id string, msg model.OutMessage) error {
	if _, err := parseTarget(jid); err != nil {
		return err
	}
	if id == "" {
		return fmt.Errorf("message id required")
	}
	f.mu.Lock()
	connected := f.status.State == model.WAConnected
	if connected {
		f.edits = append(f.edits, FakeEdit{JID: jid, ID: id, Text: msg.Text, Mentions: append([]string(nil), msg.Mentions...)})
	}
	f.mu.Unlock()
	if !connected {
		return ErrNotConnected
	}
	f.log.Infof("edit %s in %s: %q", id, jid, msg.Text)
	return nil
}

// Edits returns a copy of the recorded edits.
func (f *Fake) Edits() []FakeEdit {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]FakeEdit(nil), f.edits...)
}

// Send records a plain message.
func (f *Fake) Send(ctx context.Context, jid string, text string) error {
	_, err := f.SendMessage(ctx, jid, model.OutMessage{Text: text})
	return err
}

// SendQuoted records the message with its quote.
func (f *Fake) SendQuoted(ctx context.Context, jid, text string, q model.QuoteRef) error {
	_, err := f.SendMessage(ctx, jid, model.OutMessage{Text: text, Quote: &q})
	return err
}

// GroupParticipants returns the synthetic roster of a fake group.
func (f *Fake) GroupParticipants(ctx context.Context, groupJID string) ([]model.Participant, error) {
	g, err := parseTarget(groupJID)
	if err != nil || g.Server != types.GroupServer {
		return nil, nil
	}
	r, ok := f.rosters[g.String()]
	if !ok {
		return nil, nil
	}
	return append([]model.Participant(nil), r...), nil
}

// React records a reaction.
func (f *Fake) React(ctx context.Context, chatJID, senderJID, messageID, emoji string) error {
	if _, err := parseTarget(chatJID); err != nil {
		return err
	}
	f.mu.Lock()
	f.reacts = append(f.reacts, FakeReaction{ChatJID: chatJID, SenderJID: senderJID, MessageID: messageID, Emoji: emoji})
	f.mu.Unlock()
	f.log.Infof("react %s on %s in %s", emoji, messageID, chatJID)
	return nil
}

// MarkRead records read receipts.
func (f *Fake) MarkRead(ctx context.Context, chatJID, senderJID string, ids []string) error {
	if len(ids) == 0 {
		return nil
	}
	if _, err := parseTarget(chatJID); err != nil {
		return err
	}
	f.mu.Lock()
	f.reads = append(f.reads, FakeRead{ChatJID: chatJID, SenderJID: senderJID, IDs: append([]string(nil), ids...)})
	f.mu.Unlock()
	return nil
}

// Reads returns every MarkRead call.
func (f *Fake) Reads() []FakeRead {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]FakeRead(nil), f.reads...)
}

// Reactions returns every React call.
func (f *Fake) Reactions() []FakeReaction {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]FakeReaction(nil), f.reacts...)
}

// Sent returns a copy of every message passed to Send.
func (f *Fake) Sent() []FakeSent {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]FakeSent(nil), f.sent...)
}

func (f *Fake) Typing(ctx context.Context, jid string, on bool) {}

func (f *Fake) OwnJIDs() []string { return []string{f.self.PN.String(), f.self.LID.String()} }

func (f *Fake) SetMessageHandler(fn func(model.Incoming)) {
	f.mu.Lock()
	f.handler = fn
	f.mu.Unlock()
}

// Inject delivers in to the message handler as if it arrived from WhatsApp.
// Missing ChatJID/Timestamp/MessageID are filled in from ChatKey.
func (f *Fake) Inject(in model.Incoming) {
	if in.ChatJID == "" && in.ChatKey != "" {
		if j, err := parseTarget(in.ChatKey); err == nil {
			in.ChatJID = j.String()
		}
	}
	if strings.HasPrefix(in.ChatKey, prefixGroup) {
		in.IsGroup = true
	}
	if in.Timestamp.IsZero() {
		in.Timestamp = time.Now()
	}
	if in.MessageID == "" {
		in.MessageID = fmt.Sprintf("FAKE%d", time.Now().UnixNano())
	}
	f.mu.Lock()
	h := f.handler
	f.mu.Unlock()
	if h != nil {
		h(in)
	}
}

func (f *Fake) Close() {
	f.stopPairing()
	f.mu.Lock()
	f.closed = true
	f.mu.Unlock()
}
