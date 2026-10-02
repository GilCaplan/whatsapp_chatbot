// Package wa is the WhatsApp layer: it owns the whatsmeow client (pairing,
// connection state, reconnects, logout), normalizes incoming messages for the
// engine, and serves chat listings, avatars and sends to the rest of the app.
package wa

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"net/url"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/store"
	"go.mau.fi/whatsmeow/store/sqlstore"
	"go.mau.fi/whatsmeow/types"
	waEvents "go.mau.fi/whatsmeow/types/events"
	waLog "go.mau.fi/whatsmeow/util/log"
	"google.golang.org/protobuf/proto"
	"rsc.io/qr"

	"whatsappdoppel/internal/config"
	"whatsappdoppel/internal/contract"
	"whatsappdoppel/internal/events"
	"whatsappdoppel/internal/model"
)

// Options configures New.
type Options struct {
	Paths config.Paths
	Hub   *events.Hub
	// LegacyDBCandidates are old bot session DBs; the first existing one is
	// copied to Paths.WhatsAppDB() when that file doesn't exist yet.
	LegacyDBCandidates []string
	// Clone yourself (selfsamples.go): CollectSelf reports your consent;
	// OnSelfMessage gets redacted samples of messages you typed yourself.
	CollectSelf   func() bool
	OnSelfMessage func([]model.SelfSample)
}

var (
	ErrNotConnected  = errors.New("WhatsApp is not connected")
	ErrAlreadyLinked = errors.New("a WhatsApp account is already linked — log out first to link another one")
	ErrNotLinked     = errors.New("no WhatsApp account is linked")
	errClosed        = errors.New("WhatsApp manager is closed")
)

const (
	msgQRExpired  = "QR expired — tap to get a new one"
	msgReplaced   = "Another program is using this WhatsApp session — close the old bot, then press Reconnect"
	msgOutdated   = "WhatsApp rejected this app version as outdated — the app needs an update"
	msgRemoteGone = "This device was unlinked from your phone — scan the QR code to link again"
	groupsTTL     = 10 * time.Minute
	contactsTTL   = 10 * time.Minute
	lookupTimeout = 5 * time.Second
)

var _ contract.WhatsApp = (*Manager)(nil)

// Manager is the real WhatsApp implementation of contract.WhatsApp.
type Manager struct {
	paths     config.Paths
	hub       *events.Hub
	log       waLog.Logger // our own messages
	libLog    waLog.Logger // whatsmeow internals (WARN+)
	container *sqlstore.Container
	baseCtx   context.Context
	cancel    context.CancelFunc
	avatars   *avatarCache
	recent    *recentIndex

	opMu sync.Mutex // serializes lifecycle operations (Start/Pair/Reconnect/Disconnect/Logout/Close)

	mu         sync.Mutex // guards the fields below; never held while calling into whatsmeow
	client     *whatsmeow.Client
	status     model.WAStatus
	self       selfIDs
	pushName   string
	qrPNG      []byte
	qrCancel   context.CancelFunc
	handler    func(model.Incoming)
	migrateErr string
	started    bool
	closed     bool

	contactsGen atomic.Int64 // bumped to invalidate the contacts cache
	contactsMu  sync.Mutex   // held while (re)building contacts
	contacts    *contactCache

	groupsFetchMu sync.Mutex // serializes GetJoinedGroups
	groupsMu      sync.RWMutex
	groups        []model.ChatItem
	groupsAt      time.Time

	participants participantsCache // group rosters

	collectSelf func() bool              // Clone yourself consent (selfsamples.go)
	onSelf      func([]model.SelfSample) // where samples go
	sent        sentIDs                  // ids this app sent (never samples)
}

// New migrates a legacy session if needed, opens the session store and
// creates the client. It does not connect until Start.
func New(opts Options) (*Manager, error) {
	if opts.Paths.Dir == "" {
		return nil, errors.New("wa: Options.Paths is required")
	}
	m := &Manager{
		paths:   opts.Paths,
		hub:     opts.Hub,
		log:     newLogger("wa", levelInfo),
		libLog:  newLogger("whatsmeow", levelWarn),
		avatars: newAvatarCache(opts.Paths.AvatarCacheDir()),
		recent:  newRecentIndex(opts.Paths.RecentChatsFile()),
		status:  model.WAStatus{State: model.WADisconnected, Since: time.Now()},

		collectSelf: opts.CollectSelf,
		onSelf:      opts.OnSelfMessage,
	}
	m.baseCtx, m.cancel = context.WithCancel(context.Background())

	dbPath := opts.Paths.WhatsAppDB()
	imported, err := migrateLegacy(dbPath, opts.LegacyDBCandidates)
	if err != nil {
		m.migrateErr = err.Error()
		m.status.LastError = m.migrateErr
		m.log.Warnf("%v", err)
	}

	store.DeviceProps.Os = proto.String("WhatsApp Doppel") // name shown under "Linked devices" on the phone
	container, err := sqlstore.New(m.baseCtx, sqlDriver, sessionDSN(dbPath), m.libLog.Sub("DB"))
	if err != nil {
		m.cancel()
		return nil, fmt.Errorf("open WhatsApp session store: %w", err)
	}
	dev, err := container.GetFirstDevice(m.baseCtx)
	if err != nil {
		container.Close()
		m.cancel()
		return nil, fmt.Errorf("load WhatsApp device: %w", err)
	}
	m.container = container
	m.client = m.newClient(dev)
	m.updateSelf(m.client)

	if imported != "" {
		m.log.Infof("Imported WhatsApp session from %s", imported)
		m.activity(model.ActSystem, "Imported your existing WhatsApp session", map[string]any{"from": filepath.Base(imported)})
	}
	return m, nil
}

func (m *Manager) newClient(dev *store.Device) *whatsmeow.Client {
	cli := whatsmeow.NewClient(dev, m.libLog.Sub("Client"))
	cli.EnableAutoReconnect = true
	cli.InitialAutoReconnect = true
	cli.AddEventHandler(func(evt any) { m.handleEvent(cli, evt) })
	return cli
}

// ─── lifecycle ───────────────────────────────────────────────

// Start connects with the stored session, or begins QR pairing when there is
// none. It returns immediately; progress is reported via wa.status events.
func (m *Manager) Start(ctx context.Context) error {
	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		return errClosed
	}
	if m.started {
		m.mu.Unlock()
		return nil
	}
	m.started = true
	m.mu.Unlock()
	go func() {
		m.opMu.Lock()
		defer m.opMu.Unlock()
		if m.isClosed() {
			return
		}
		cli := m.currentClient()
		var err error
		if cli.Store.ID == nil {
			err = m.beginPairing()
		} else {
			err = m.connect(cli)
		}
		if err != nil {
			m.log.Warnf("start: %v", err)
		}
	}()
	return nil
}

// connect connects an already-linked client. Caller holds opMu.
func (m *Manager) connect(cli *whatsmeow.Client) error {
	m.prepareVersion()
	m.setState(model.WAConnecting, "")
	if err := cli.Connect(); err != nil && !errors.Is(err, whatsmeow.ErrAlreadyConnected) {
		m.setState(model.WAError, "Couldn't connect to WhatsApp: "+err.Error())
		return err
	}
	return nil
}

// beginPairing starts a fresh QR flow on a new client for the (unlinked)
// device, so stale QR channels and handlers can't interfere. Caller holds opMu.
func (m *Manager) beginPairing() error {
	old := m.currentClient()
	if old.Store.ID != nil {
		return ErrAlreadyLinked
	}
	m.stopQR()
	old.RemoveEventHandlers()
	old.Disconnect()

	m.prepareVersion()
	cli := m.newClient(old.Store)
	ctx, cancel := context.WithCancel(m.baseCtx)
	ch, err := cli.GetQRChannel(ctx)
	if err != nil {
		cancel()
		m.setState(model.WAError, "Couldn't start linking: "+err.Error())
		return err
	}
	m.mu.Lock()
	m.client, m.qrCancel, m.qrPNG = cli, cancel, nil
	state := m.status.State
	m.mu.Unlock()
	if state != model.WALoggedOut {
		m.setState(model.WAConnecting, "")
	}
	go m.qrLoop(ctx, cli, ch)
	if err := cli.Connect(); err != nil {
		cancel()
		m.setState(model.WAError, "Couldn't reach WhatsApp to get a QR code: "+err.Error())
		return err
	}
	return nil
}

func (m *Manager) qrLoop(ctx context.Context, cli *whatsmeow.Client, ch <-chan whatsmeow.QRChannelItem) {
	for {
		var item whatsmeow.QRChannelItem
		select {
		case <-ctx.Done():
			return
		case it, ok := <-ch:
			if !ok {
				return
			}
			item = it
		}
		if !m.isCurrent(cli) {
			continue
		}
		switch {
		case item.Event == whatsmeow.QRChannelEventCode:
			png, err := qrPNG(item.Code)
			if err != nil {
				m.log.Errorf("encode QR: %v", err)
				continue
			}
			m.mu.Lock()
			m.qrPNG = png
			m.mu.Unlock()
			m.setState(model.WAAwaitingQR, "")
			m.publish(events.TypeWAQR, model.QRFrame{
				PNG:          "data:image/png;base64," + base64.StdEncoding.EncodeToString(png),
				ExpiresInSec: int(item.Timeout / time.Second),
			})
		case item == whatsmeow.QRChannelSuccess:
			m.clearQR()
			m.setState(model.WAPairing, "")
		case item == whatsmeow.QRChannelTimeout:
			m.clearQR()
			cli.Disconnect()
			m.setState(model.WAAwaitingQR, msgQRExpired)
		case item == whatsmeow.QRChannelClientOutdated:
			m.clearQR()
			m.setState(model.WAOutdated, msgOutdated)
		case item == whatsmeow.QRChannelScannedWithoutMultidevice:
			m.setState(model.WAAwaitingQR, "Your phone's WhatsApp needs multi-device support — update WhatsApp and scan again")
		case item.Event == whatsmeow.QRChannelEventError:
			m.clearQR()
			m.setState(model.WAError, fmt.Sprintf("Linking failed: %v", item.Error))
		}
	}
}

// prepareVersion refreshes the advertised WhatsApp Web version (forced after
// an "outdated" rejection). Caller holds opMu.
func (m *Manager) prepareVersion() {
	refreshWAVersion(m.baseCtx, m.avatars.http, m.log, m.Status().State == model.WAOutdated)
}

func qrPNG(code string) ([]byte, error) {
	c, err := qr.Encode(code, qr.M)
	if err != nil {
		return nil, err
	}
	c.Scale = 8
	return c.PNG(), nil
}

// Pair (re)starts QR pairing; only valid when no device is linked.
func (m *Manager) Pair() error {
	m.opMu.Lock()
	defer m.opMu.Unlock()
	if m.isClosed() {
		return errClosed
	}
	m.mu.Lock()
	m.started = true
	m.mu.Unlock()
	return m.beginPairing()
}

// Reconnect drops and re-opens the connection (or restarts pairing when unlinked).
func (m *Manager) Reconnect() error {
	m.opMu.Lock()
	defer m.opMu.Unlock()
	if m.isClosed() {
		return errClosed
	}
	m.mu.Lock()
	m.started = true
	m.mu.Unlock()
	cli := m.currentClient()
	if cli.Store.ID == nil {
		return m.beginPairing()
	}
	m.stopQR()
	cli.Disconnect()
	return m.connect(cli)
}

// Disconnect closes the connection; the session stays linked.
func (m *Manager) Disconnect() {
	m.opMu.Lock()
	defer m.opMu.Unlock()
	m.stopQR()
	m.currentClient().Disconnect()
	m.setState(model.WADisconnected, "")
}

// Logout unlinks this device from the phone, resets to a fresh device and
// begins pairing again so the UI can show a QR code.
func (m *Manager) Logout(ctx context.Context) error {
	m.opMu.Lock()
	defer m.opMu.Unlock()
	if m.isClosed() {
		return errClosed
	}
	cli := m.currentClient()
	if cli.Store.ID == nil {
		return ErrNotLinked
	}
	note := ""
	if err := cli.Logout(ctx); err != nil {
		if cli.IsLoggedIn() {
			return err
		}
		// Offline: forget the session locally; the phone keeps a stale entry.
		cli.Disconnect()
		if derr := cli.Store.Delete(ctx); derr != nil {
			return fmt.Errorf("%w (and deleting the local session failed: %v)", err, derr)
		}
		note = "Logged out locally — also remove this device under Linked devices on your phone"
	}
	m.resetDevice(note)
	return nil
}

// resetDevice swaps in a fresh, unpaired device + client, clears per-account
// caches, reports logged_out and starts pairing. Caller holds opMu.
func (m *Manager) resetDevice(note string) {
	old := m.currentClient()
	old.RemoveEventHandlers()
	old.Disconnect()
	m.stopQR()
	cli := m.newClient(m.container.NewDevice())
	m.mu.Lock()
	m.client, m.self, m.pushName = cli, selfIDs{}, ""
	m.mu.Unlock()
	m.invalidateContacts()
	m.groupsMu.Lock()
	m.groups, m.groupsAt = nil, time.Time{}
	m.groupsMu.Unlock()
	m.participants.clear()
	m.recent.clear()
	m.setState(model.WALoggedOut, note)
	if err := m.beginPairing(); err != nil {
		m.log.Warnf("pairing after logout: %v", err)
	}
}

// Close disconnects cleanly and releases the session store.
func (m *Manager) Close() {
	m.opMu.Lock()
	defer m.opMu.Unlock()
	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		return
	}
	m.closed = true
	m.status.State = model.WADisconnected
	cli := m.client
	m.mu.Unlock()
	m.stopQR()
	cli.RemoveEventHandlers()
	cli.Disconnect()
	m.cancel()
	m.recent.close()
	if err := m.container.Close(); err != nil {
		m.log.Warnf("close session store: %v", err)
	}
}

// ─── state ───────────────────────────────────────────────────

func (m *Manager) currentClient() *whatsmeow.Client {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.client
}

func (m *Manager) isCurrent(cli *whatsmeow.Client) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.client == cli && !m.closed
}

func (m *Manager) isClosed() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.closed
}

func (m *Manager) selfIDs() selfIDs {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.self
}

func (m *Manager) stopQR() {
	m.mu.Lock()
	cancel := m.qrCancel
	m.qrCancel, m.qrPNG = nil, nil
	m.mu.Unlock()
	if cancel != nil {
		cancel()
	}
}

func (m *Manager) clearQR() {
	m.mu.Lock()
	m.qrPNG = nil
	m.mu.Unlock()
}

// updateSelf caches the linked account's identity from the device store.
func (m *Manager) updateSelf(cli *whatsmeow.Client) {
	var s selfIDs
	if id := cli.Store.ID; id != nil {
		s.PN, s.LID = id.ToNonAD(), cli.Store.LID.ToNonAD()
	}
	name := cli.Store.PushName
	m.mu.Lock()
	m.self, m.pushName = s, name
	m.mu.Unlock()
}

// Status returns the current connection state.
func (m *Manager) Status() model.WAStatus {
	m.mu.Lock()
	defer m.mu.Unlock()
	st := m.status
	if m.self.linked() {
		jid := m.self.PN.String()
		st.Me = &model.WAMe{
			JID:       jid,
			LID:       jidString(m.self.LID),
			PushName:  m.pushName,
			Phone:     m.self.PN.User,
			AvatarURL: "/api/wa/avatar?jid=" + url.QueryEscape(jid),
		}
	}
	return st
}

// setState records a state change, publishes wa.status and, for meaningful
// transitions, a wa.status activity.
func (m *Manager) setState(state, lastErr string) {
	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		return
	}
	if lastErr == "" && m.migrateErr != "" && (state == model.WAConnecting || state == model.WAAwaitingQR) {
		lastErr = m.migrateErr // keep the "didn't import" reason visible while pairing
	}
	prev := m.status
	if prev.State == state && prev.LastError == lastErr {
		m.mu.Unlock()
		return
	}
	m.status.State, m.status.LastError = state, lastErr
	if prev.State != state {
		m.status.Since = time.Now()
	}
	m.mu.Unlock()

	st := m.Status()
	m.publish(events.TypeWAStatus, st)
	if prev.State != state {
		if text := stateActivityText(st); text != "" {
			m.activity(model.ActWAStatus, text, map[string]any{"state": state})
		}
	}
}

func stateActivityText(st model.WAStatus) string {
	switch st.State {
	case model.WAConnected:
		if st.Me != nil && st.Me.PushName != "" {
			return "WhatsApp connected as " + st.Me.PushName
		}
		return "WhatsApp connected"
	case model.WADisconnected:
		return "WhatsApp disconnected"
	case model.WAAwaitingQR:
		return "Waiting for you to scan the QR code"
	case model.WAPairing:
		return "QR code scanned — finishing the link"
	case model.WALoggedOut:
		if st.LastError != "" {
			return "WhatsApp logged out: " + st.LastError
		}
		return "WhatsApp logged out"
	case model.WAReplaced, model.WABanned, model.WAOutdated, model.WAError:
		if st.LastError != "" {
			return st.LastError
		}
		return "WhatsApp " + st.State
	}
	return ""
}

func (m *Manager) publish(typ string, data any) {
	if m.hub != nil {
		m.hub.Publish(typ, data)
	}
}

func (m *Manager) activity(typ, text string, meta map[string]any) {
	if m.hub != nil {
		m.hub.Activity(model.ActivityEvent{Type: typ, Text: text, Meta: meta})
	}
}

// CurrentQR returns the latest QR PNG, or nil when not pairing.
func (m *Manager) CurrentQR() []byte {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.qrPNG == nil {
		return nil
	}
	return append([]byte(nil), m.qrPNG...)
}

// OwnJIDs returns the linked account's phone and lid JIDs.
func (m *Manager) OwnJIDs() []string {
	s := m.selfIDs()
	var out []string
	if s.PN.User != "" {
		out = append(out, s.PN.String())
	}
	if s.LID.User != "" {
		out = append(out, s.LID.String())
	}
	return out
}

// SetMessageHandler installs the receiver of normalized incoming messages.
func (m *Manager) SetMessageHandler(fn func(model.Incoming)) {
	m.mu.Lock()
	m.handler = fn
	m.mu.Unlock()
}

// ─── events ──────────────────────────────────────────────────

func (m *Manager) handleEvent(cli *whatsmeow.Client, evt any) {
	if !m.isCurrent(cli) {
		return
	}
	switch v := evt.(type) {
	case *waEvents.Message:
		m.onMessage(cli, v)
	case *waEvents.Connected:
		m.onConnected(cli)
	case *waEvents.Disconnected:
		switch m.Status().State {
		case model.WAConnected, model.WAConnecting, model.WAPairing:
			if cli.Store.ID != nil {
				m.setState(model.WADisconnected, "")
			}
		}
	case *waEvents.StreamReplaced:
		m.setState(model.WAReplaced, msgReplaced)
	case *waEvents.TemporaryBan:
		m.setState(model.WABanned, v.String())
	case *waEvents.ClientOutdated:
		m.setState(model.WAOutdated, msgOutdated)
	case *waEvents.ConnectFailure:
		m.setState(model.WAError, fmt.Sprintf("WhatsApp refused the connection: %s %s", v.Reason, v.Message))
	case *waEvents.LoggedOut:
		go m.onRemoteLogout(cli, v)
	case *waEvents.PairSuccess:
		m.updateSelf(cli)
		m.setState(model.WAPairing, "")
	case *waEvents.PairError:
		m.setState(model.WAError, fmt.Sprintf("Linking failed: %v", v.Error))
	case *waEvents.Picture:
		m.avatars.invalidate(m.avatarKeys(cli, v.JID)...)
	case *waEvents.PushName, *waEvents.Contact, *waEvents.BusinessName:
		m.invalidateContacts()
	case *waEvents.PushNameSetting:
		m.updateSelf(cli)
		m.publish(events.TypeWAStatus, m.Status())
	case *waEvents.HistorySync:
		m.onHistorySync(cli, v)
	case *waEvents.GroupInfo:
		m.participants.invalidate(v.JID.ToNonAD().String())
	case *waEvents.JoinedGroup:
		m.participants.invalidate(v.JID.ToNonAD().String())
	}
}

func (m *Manager) onConnected(cli *whatsmeow.Client) {
	m.updateSelf(cli)
	m.mu.Lock()
	m.migrateErr = ""
	m.qrPNG = nil
	m.mu.Unlock()
	m.setState(model.WAConnected, "")
	go func() {
		ctx, cancel := context.WithTimeout(m.baseCtx, 30*time.Second)
		defer cancel()
		m.mu.Lock()
		hasName := m.pushName != ""
		m.mu.Unlock()
		if hasName {
			if err := cli.SendPresence(ctx, types.PresenceAvailable); err != nil {
				m.log.Debugf("send presence: %v", err)
			}
		}
		m.invalidateContacts()
		if _, err := m.groupList(ctx, cli, true); err != nil {
			m.log.Warnf("refresh groups: %v", err)
		}
	}()
}

func (m *Manager) onRemoteLogout(cli *whatsmeow.Client, v *waEvents.LoggedOut) {
	m.opMu.Lock()
	defer m.opMu.Unlock()
	if !m.isCurrent(cli) {
		return
	}
	note := msgRemoteGone
	if v.OnConnect && v.Reason != waEvents.ConnectFailureLoggedOut {
		note = fmt.Sprintf("WhatsApp ended this session (%s) — scan the QR code to link again", v.Reason)
	}
	m.resetDevice(note)
}

func (m *Manager) onMessage(cli *whatsmeow.Client, v *waEvents.Message) {
	in, ok := normalizeMessage(v, m.resolverFor(cli))
	if !ok {
		return
	}
	m.touchRecent(in)
	m.sampleLive(in) // Clone yourself, with consent (selfsamples.go)
	m.mu.Lock()
	h := m.handler
	m.mu.Unlock()
	if h != nil {
		h(in)
	}
}

func (m *Manager) onHistorySync(cli *whatsmeow.Client, v *waEvents.HistorySync) {
	r := m.resolverFor(cli)
	n := 0
	for _, conv := range v.Data.GetConversations() {
		if e, ok := entryFromConversation(conv, r); ok {
			m.recent.upsert(e, true)
			n++
		}
	}
	if n > 0 || len(v.Data.GetPushnames()) > 0 {
		m.invalidateContacts()
	}
	m.sampleHistory(cli, v.Data.GetConversations(), r) // Clone yourself, with consent
}

// touchRecent records a live message in the recent-chats index.
func (m *Manager) touchRecent(in model.Incoming) {
	e := recentEntry{Key: in.ChatKey, TS: in.Timestamp.Unix()}
	switch {
	case in.IsGroup:
		e.Kind, e.JID = "group", in.ChatJID
		e.Name = m.cachedGroupName(in.ChatJID)
	case strings.HasPrefix(in.ChatKey, prefixDM):
		e.Kind = "dm"
		e.JID = types.NewJID(strings.TrimPrefix(in.ChatKey, prefixDM), types.DefaultUserServer).String()
		for _, j := range []string{in.ChatJID, in.AltJID} {
			if pj, err := types.ParseJID(j); err == nil && pj.Server == types.HiddenUserServer {
				e.AltJID = j
			}
		}
	default:
		e.Kind, e.JID = "dm", in.ChatJID
	}
	if !in.IsGroup && !in.IsFromMe {
		e.Name = in.PushName
	}
	m.recent.upsert(e, false)
}

// resolverFor builds a resolver backed by the client's LID map.
func (m *Manager) resolverFor(cli *whatsmeow.Client) resolver {
	r := resolver{self: m.selfIDs()}
	lids := cli.Store.LIDs
	if lids == nil {
		return r
	}
	r.pnForLID = func(j types.JID) types.JID {
		ctx, cancel := context.WithTimeout(m.baseCtx, lookupTimeout)
		defer cancel()
		pn, err := lids.GetPNForLID(ctx, j)
		if err != nil {
			return types.JID{}
		}
		return pn
	}
	r.lidForPN = func(j types.JID) types.JID {
		ctx, cancel := context.WithTimeout(m.baseCtx, lookupTimeout)
		defer cancel()
		lid, err := lids.GetLIDForPN(ctx, j)
		if err != nil {
			return types.JID{}
		}
		return lid
	}
	return r
}

// ─── messaging ───────────────────────────────────────────────

// buildMessage turns an OutMessage into a WhatsApp message: a plain
// Conversation, or an ExtendedTextMessage whose ContextInfo carries the quote
// and/or the mentioned JIDs.
func buildMessage(msg model.OutMessage) *waE2E.Message {
	quote := msg.Quote != nil && msg.Quote.MessageID != ""
	if !quote && len(msg.Mentions) == 0 {
		return &waE2E.Message{Conversation: proto.String(msg.Text)}
	}
	ci := &waE2E.ContextInfo{}
	if quote {
		q := msg.Quote
		ci.StanzaID = proto.String(q.MessageID)
		ci.QuotedMessage = &waE2E.Message{Conversation: proto.String(q.Text)}
		if q.SenderJID != "" {
			if s, err := types.ParseJID(q.SenderJID); err == nil {
				ci.Participant = proto.String(s.ToNonAD().String())
			}
		}
	}
	if len(msg.Mentions) > 0 {
		ci.MentionedJID = append([]string(nil), msg.Mentions...)
	}
	return &waE2E.Message{ExtendedTextMessage: &waE2E.ExtendedTextMessage{Text: proto.String(msg.Text), ContextInfo: ci}}
}

// SendMessage sends a text message (quote and mentions optional). jid may be
// a JID, chat key or phone number. The result has WhatsApp's message id.
func (m *Manager) SendMessage(ctx context.Context, jid string, msg model.OutMessage) (model.SendResult, error) {
	to, err := parseTarget(jid)
	if err != nil {
		return model.SendResult{}, err
	}
	cli := m.currentClient()
	if !cli.IsLoggedIn() {
		return model.SendResult{}, ErrNotConnected
	}
	resp, err := cli.SendMessage(ctx, to, buildMessage(msg))
	if err != nil {
		return model.SendResult{}, err
	}
	m.sent.add(string(resp.ID))
	return model.SendResult{ID: string(resp.ID), At: resp.Timestamp}, nil
}

// EditMessage replaces the text of one of our own messages (WhatsApp allows
// it for 20 minutes). Mentions are kept; a quote is not (edits can't change it).
func (m *Manager) EditMessage(ctx context.Context, jid, id string, msg model.OutMessage) error {
	chat, err := parseTarget(jid)
	if err != nil {
		return err
	}
	if id == "" {
		return errors.New("message id required")
	}
	cli := m.currentClient()
	if !cli.IsLoggedIn() {
		return ErrNotConnected
	}
	msg.Quote = nil
	resp, err := cli.SendMessage(ctx, chat, cli.BuildEdit(chat, types.MessageID(id), buildMessage(msg)))
	if err == nil {
		m.sent.add(string(resp.ID))
	}
	return err
}

// Send sends a plain text message.
func (m *Manager) Send(ctx context.Context, jid string, text string) error {
	_, err := m.SendMessage(ctx, jid, model.OutMessage{Text: text})
	return err
}

// SendQuoted sends text as a reply quoting q. Without a message id it is a plain Send.
func (m *Manager) SendQuoted(ctx context.Context, jid, text string, q model.QuoteRef) error {
	_, err := m.SendMessage(ctx, jid, model.OutMessage{Text: text, Quote: &q})
	return err
}

// optionalJID parses a sender JID ("" → empty JID).
func optionalJID(s string) (types.JID, error) {
	if strings.TrimSpace(s) == "" {
		return types.EmptyJID, nil
	}
	j, err := types.ParseJID(s)
	if err != nil {
		return types.EmptyJID, fmt.Errorf("%w: %q", errBadJID, s)
	}
	return j.ToNonAD(), nil
}

// React sets (or with emoji "" removes) a reaction on a message.
func (m *Manager) React(ctx context.Context, chatJID, senderJID, messageID, emoji string) error {
	chat, err := parseTarget(chatJID)
	if err != nil {
		return err
	}
	sender, err := optionalJID(senderJID)
	if err != nil {
		return err
	}
	if messageID == "" {
		return errors.New("message id required")
	}
	cli := m.currentClient()
	if !cli.IsLoggedIn() {
		return ErrNotConnected
	}
	_, err = cli.SendMessage(ctx, chat, cli.BuildReaction(chat, sender, types.MessageID(messageID), emoji))
	return err
}

// MarkRead sends read receipts (blue ticks; whatsmeow downgrades to a private
// "read-self" receipt when the account has read receipts turned off).
func (m *Manager) MarkRead(ctx context.Context, chatJID, senderJID string, ids []string) error {
	if len(ids) == 0 {
		return nil
	}
	chat, err := parseTarget(chatJID)
	if err != nil {
		return err
	}
	sender, err := optionalJID(senderJID)
	if err != nil {
		return err
	}
	cli := m.currentClient()
	if !cli.IsLoggedIn() {
		return ErrNotConnected
	}
	mids := make([]types.MessageID, len(ids))
	for i, id := range ids {
		mids[i] = types.MessageID(id)
	}
	return cli.MarkRead(ctx, mids, time.Now(), chat, sender)
}

// Typing shows or clears the "typing…" indicator; errors are only logged.
func (m *Manager) Typing(ctx context.Context, jid string, on bool) {
	to, err := parseTarget(jid)
	if err != nil {
		return
	}
	cli := m.currentClient()
	if !cli.IsLoggedIn() {
		return
	}
	state := types.ChatPresencePaused
	if on {
		state = types.ChatPresenceComposing
	}
	if err := cli.SendChatPresence(ctx, to, state, types.ChatPresenceMediaText); err != nil {
		m.log.Debugf("chat presence %s: %v", state, err)
	}
}
