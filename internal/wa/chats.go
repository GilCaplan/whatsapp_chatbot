package wa

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/types"

	"whatsappdoppel/internal/model"
)

// contactCache is the merged address book (phone-keyed), built from ~14k
// whatsmeow contact rows on demand.
type contactCache struct {
	gen     int64
	at      time.Time
	items   []model.ChatItem
	byPhone map[string]string // phone → display name
}

func (m *Manager) invalidateContacts() { m.contactsGen.Add(1) }

// contactList returns the merged address book. Only the very first call
// builds it in the foreground: a stale cache (contacts changed or older than
// contactsTTL) is served while one rebuild runs in the background. Right after
// linking WhatsApp sends thousands of contact updates; rebuilding 14k rows per
// request used to stall the chat list for as long as that sync lasted.
func (m *Manager) contactList(ctx context.Context, cli *whatsmeow.Client) (*contactCache, error) {
	m.contactsMu.Lock()
	c := m.contacts
	m.contactsMu.Unlock()
	if c == nil {
		return m.buildContactsNow(ctx, cli)
	}
	if c.gen != m.contactsGen.Load() || time.Since(c.at) >= contactsTTL {
		m.rebuildContactsLater(cli)
	}
	return c, nil
}

// buildContactsNow (re)builds the cache unless a concurrent caller just did.
func (m *Manager) buildContactsNow(ctx context.Context, cli *whatsmeow.Client) (*contactCache, error) {
	m.contactsBuildMu.Lock()
	defer m.contactsBuildMu.Unlock()
	gen := m.contactsGen.Load()
	m.contactsMu.Lock()
	cur := m.contacts
	m.contactsMu.Unlock()
	if cur != nil && cur.gen == gen && time.Since(cur.at) < contactsTTL {
		return cur, nil
	}
	c, err := buildContacts(ctx, cli, m.selfIDs())
	if err != nil {
		return nil, err
	}
	c.gen = gen
	if m.isCurrent(cli) {
		m.contactsMu.Lock()
		m.contacts = c
		m.contactsMu.Unlock()
	}
	return c, nil
}

// rebuildContactsLater starts one background rebuild after a short pause, so
// a burst of contact updates costs a single rebuild.
func (m *Manager) rebuildContactsLater(cli *whatsmeow.Client) {
	if !m.contactsBuilding.CompareAndSwap(false, true) {
		return
	}
	go func() {
		defer m.contactsBuilding.Store(false)
		select {
		case <-time.After(contactsSettle):
		case <-m.baseCtx.Done():
			return
		}
		ctx, cancel := context.WithTimeout(m.baseCtx, time.Minute)
		defer cancel()
		if _, err := m.buildContactsNow(ctx, cli); err != nil {
			m.log.Warnf("refresh contacts: %v", err)
		}
	}()
}

type nameParts struct{ full, push, biz string }

func (n *nameParts) fill(ci types.ContactInfo) {
	n.full = cmpOr(n.full, ci.FullName)
	n.push = cmpOr(n.push, ci.PushName)
	n.biz = cmpOr(n.biz, ci.BusinessName)
}

func (n nameParts) best() string { return cmpOr(n.full, n.push, n.biz) }

func cmpOr(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

func buildContacts(ctx context.Context, cli *whatsmeow.Client, self selfIDs) (*contactCache, error) {
	c := &contactCache{at: time.Now(), byPhone: map[string]string{}}
	st := cli.Store
	if st.ID == nil || st.Contacts == nil {
		return c, nil
	}
	all, err := st.Contacts.GetAllContacts(ctx)
	if err != nil {
		return nil, fmt.Errorf("load contacts: %w", err)
	}
	if fc, ok := st.LIDs.(interface{ FillCache(context.Context) error }); ok {
		_ = fc.FillCache(ctx) // makes the per-contact LID lookups below map hits
	}
	names := make(map[string]*nameParts, len(all))
	lidOf := make(map[string]types.JID)
	part := func(phone string) *nameParts {
		n := names[phone]
		if n == nil {
			n = &nameParts{}
			names[phone] = n
		}
		return n
	}
	// Phone entries first so their names win; lid entries only fill gaps.
	for jid, ci := range all {
		if jid.Server == types.DefaultUserServer && jid.User != "" {
			part(jid.User).fill(ci)
		}
	}
	for jid, ci := range all {
		if jid.Server != types.HiddenUserServer || st.LIDs == nil {
			continue
		}
		pn, err := st.LIDs.GetPNForLID(ctx, jid)
		if err != nil || pn.User == "" {
			continue
		}
		part(pn.User).fill(ci)
		lidOf[pn.User] = jid.ToNonAD()
	}
	c.items = make([]model.ChatItem, 0, len(names))
	for phone, n := range names {
		pn := types.NewJID(phone, types.DefaultUserServer)
		lid, ok := lidOf[phone]
		if !ok && st.LIDs != nil {
			lid, _ = st.LIDs.GetLIDForPN(ctx, pn)
			lid = lid.ToNonAD()
		}
		name := n.best()
		it := dmItem(prefixDM+phone, pn, lid, name, self)
		c.items = append(c.items, it)
		if name != "" {
			c.byPhone[phone] = name
		}
	}
	sortByName(c.items)
	return c, nil
}

// contactName returns the address-book name for phone ("" when unknown).
func (m *Manager) contactName(ctx context.Context, cli *whatsmeow.Client, phone string) string {
	c, err := m.contactList(ctx, cli)
	if err != nil {
		return ""
	}
	return c.byPhone[phone]
}

// groupList returns joined groups. A cached list is served at once (when it
// is older than groupsTTL, one refresh runs in the background); force, or no
// list yet, waits for WhatsApp (at most groupsTimeout).
func (m *Manager) groupList(ctx context.Context, cli *whatsmeow.Client, force bool) ([]model.ChatItem, error) {
	m.groupsMu.RLock()
	cached, at := m.groups, m.groupsAt
	m.groupsMu.RUnlock()
	if !force && cached != nil {
		if time.Since(at) >= groupsTTL {
			m.refreshGroupsLater(cli)
		}
		return cached, nil
	}
	return m.fetchGroups(ctx, cli)
}

func (m *Manager) fetchGroups(ctx context.Context, cli *whatsmeow.Client) ([]model.ChatItem, error) {
	m.groupsFetchMu.Lock()
	defer m.groupsFetchMu.Unlock()
	m.groupsMu.RLock()
	cached, at := m.groups, m.groupsAt
	m.groupsMu.RUnlock()
	if cached != nil && time.Since(at) < time.Second*5 {
		return cached, nil // refreshed by a concurrent caller
	}
	if !cli.IsLoggedIn() {
		if cached != nil {
			return cached, nil
		}
		return nil, ErrNotConnected
	}
	ctx, cancel := context.WithTimeout(ctx, groupsTimeout)
	defer cancel()
	gs, err := cli.GetJoinedGroups(ctx)
	if err != nil {
		if cached != nil {
			m.log.Warnf("refresh groups (serving cache): %v", err)
			return cached, nil
		}
		return nil, fmt.Errorf("load groups: %w", err)
	}
	items := make([]model.ChatItem, 0, len(gs))
	for _, g := range gs {
		items = append(items, groupItem(g))
	}
	sortByName(items)
	if m.isCurrent(cli) {
		m.setGroups(items) // membership.go: reports groups you joined or left meanwhile
	}
	return items, nil
}

// refreshGroupsLater refreshes the group list once in the background.
func (m *Manager) refreshGroupsLater(cli *whatsmeow.Client) {
	if !m.groupsRefreshing.CompareAndSwap(false, true) {
		return
	}
	go func() {
		defer m.groupsRefreshing.Store(false)
		if _, err := m.fetchGroups(m.baseCtx, cli); err != nil {
			m.log.Warnf("refresh groups: %v", err)
		}
	}()
}

func groupItem(g *types.GroupInfo) model.ChatItem {
	n := g.ParticipantCount
	if n == 0 {
		n = len(g.Participants)
	}
	return model.ChatItem{
		Key:              prefixGroup + g.JID.User,
		Kind:             "group",
		JID:              g.JID.ToNonAD().String(),
		Name:             cmpOr(g.Name, "Unnamed group"),
		ParticipantCount: n,
	}
}

// cachedGroup looks a group up in the cache without fetching.
func (m *Manager) cachedGroup(jid string) (model.ChatItem, bool) {
	m.groupsMu.RLock()
	defer m.groupsMu.RUnlock()
	for _, g := range m.groups {
		if g.JID == jid {
			return g, true
		}
	}
	return model.ChatItem{}, false
}

func (m *Manager) cachedGroupName(jid string) string {
	g, _ := m.cachedGroup(jid)
	return g.Name
}

// ListChats lists chats for the picker (tabs: recent, contacts, groups).
func (m *Manager) ListChats(ctx context.Context, q model.ChatQuery) ([]model.ChatItem, int, error) {
	cli := m.currentClient()
	self := m.selfIDs()
	switch strings.ToLower(q.Tab) {
	case "", "recent":
		var pinned []model.ChatItem
		if self.linked() {
			pinned = append(pinned, selfItem(self))
		}
		items, total := filterChats(pinned, m.recentItems(ctx, cli, self, q.Q != ""), q)
		return items, total, nil
	case "contacts":
		c, err := m.contactList(ctx, cli)
		if err != nil {
			return nil, 0, err
		}
		items, total := filterChats(nil, c.items, q)
		return items, total, nil
	case "groups":
		gs, err := m.groupList(ctx, cli, false)
		if errors.Is(err, ErrNotConnected) {
			return []model.ChatItem{}, 0, nil
		} else if err != nil {
			return nil, 0, err
		}
		items, total := filterChats(nil, gs, q)
		return items, total, nil
	case "assigned": // handled by the server
		return []model.ChatItem{}, 0, nil
	}
	return nil, 0, fmt.Errorf("unknown chat tab %q", q.Tab)
}

// recentItems renders the recent-chats index (newest first). Archived chats
// are hidden unless includeArchived (i.e. when searching).
func (m *Manager) recentItems(ctx context.Context, cli *whatsmeow.Client, self selfIDs, includeArchived bool) []model.ChatItem {
	entries := m.recent.list()
	selfKey := ""
	if self.linked() {
		selfKey = prefixDM + self.PN.User
	}
	var names map[string]string
	if c, err := m.contactList(ctx, cli); err == nil {
		names = c.byPhone
	}
	items := make([]model.ChatItem, 0, len(entries))
	for _, e := range entries {
		if e.Key == selfKey || (e.Archived && !includeArchived) {
			continue
		}
		it := model.ChatItem{Key: e.Key, Kind: e.Kind, JID: e.JID, AltJID: e.AltJID, Name: e.Name}
		if e.TS > 0 {
			it.LastMessageAt = timePtr(time.Unix(e.TS, 0))
		}
		switch {
		case e.Kind == "group":
			if g, ok := m.cachedGroup(e.JID); ok {
				it.Name, it.ParticipantCount = g.Name, g.ParticipantCount
			}
			it.Name = cmpOr(it.Name, "Group")
		case strings.HasPrefix(e.Key, prefixDM):
			it.Phone = strings.TrimPrefix(e.Key, prefixDM)
			it.Name = cmpOr(names[it.Phone], it.Name, phoneName(it.Phone))
		default:
			it.Name = cmpOr(it.Name, "Unknown contact")
		}
		items = append(items, it)
	}
	sortByRecent(items)
	return items
}

// RefreshChats re-fetches joined groups and drops the contacts cache (the
// web app calls it on every page load). It also corrects a "connected" state
// whose connection is gone.
func (m *Manager) RefreshChats(ctx context.Context) error {
	cli := m.currentClient()
	if m.Status().State == model.WAConnected && !cli.IsConnected() {
		m.setState(model.WADisconnected, "")
	}
	m.invalidateContacts()
	_, err := m.groupList(ctx, cli, true)
	return err
}

// ResolveChat canonicalizes a JID, chat key or phone number into a ChatItem.
func (m *Manager) ResolveChat(ctx context.Context, jid string) (model.ChatItem, error) {
	target, err := parseTarget(jid)
	if err != nil {
		return model.ChatItem{}, err
	}
	cli := m.currentClient()
	self := m.selfIDs()
	var it model.ChatItem
	switch {
	case target.Server == types.GroupServer:
		if g, ok := m.cachedGroup(target.String()); ok {
			it = g
		} else {
			it = model.ChatItem{Key: prefixGroup + target.User, Kind: "group", JID: target.String()}
			if cli.IsLoggedIn() {
				if g, err := cli.GetGroupInfo(ctx, target); err == nil {
					it = groupItem(g)
				}
			}
		}
	case isUserServer(target):
		key, pn, lid := m.resolverFor(cli).dm(target, types.JID{})
		name := ""
		if pn.User != "" {
			name = m.contactName(ctx, cli, pn.User)
		}
		it = dmItem(key, pn, lid, name, self)
		if it.IsSelf {
			it.Name = SelfChatName
		}
	default:
		return model.ChatItem{}, fmt.Errorf("%w: %q", errBadJID, jid)
	}
	if e, ok := m.recent.get(it.Key); ok {
		if e.TS > 0 {
			it.LastMessageAt = timePtr(time.Unix(e.TS, 0))
		}
		if strings.HasPrefix(it.Name, "+") || it.Name == "" || it.Name == "Unknown contact" {
			it.Name = cmpOr(e.Name, it.Name)
		}
	}
	if it.Kind == "group" {
		it.Name = cmpOr(it.Name, "Group")
	}
	return it, nil
}

// ─── avatars ─────────────────────────────────────────────────

// avatarKey canonicalizes a chat address for the avatar cache (phone JID when known).
func (m *Manager) avatarKey(cli *whatsmeow.Client, jid types.JID) types.JID {
	jid = jid.ToNonAD()
	if !isUserServer(jid) {
		return jid
	}
	_, pn, lid := m.resolverFor(cli).dm(jid, types.JID{})
	if pn.User != "" {
		return pn
	}
	return lid
}

func (m *Manager) avatarKeys(cli *whatsmeow.Client, jid types.JID) []string {
	return []string{jid.ToNonAD().String(), m.avatarKey(cli, jid).String()}
}

// Avatar returns profile picture bytes; (nil, "", nil) when there is none or it's hidden.
func (m *Manager) Avatar(ctx context.Context, jid string) ([]byte, string, error) {
	target, err := parseTarget(jid)
	if err != nil {
		return nil, "", err
	}
	cli := m.currentClient()
	target = m.avatarKey(cli, target)
	key := target.String()

	stale, fresh, negative := m.avatars.lookup(key)
	switch {
	case negative:
		return nil, "", nil
	case fresh:
		return stale, imageContentType(stale), nil
	}
	fallback := func(err error) ([]byte, string, error) {
		if stale != nil {
			return stale, imageContentType(stale), nil
		}
		return nil, "", err
	}
	if !cli.IsLoggedIn() {
		return fallback(nil)
	}
	if err := m.avatars.acquire(ctx); err != nil {
		return fallback(err)
	}
	defer m.avatars.release()
	if data, fresh, negative := m.avatars.lookup(key); negative {
		return nil, "", nil
	} else if fresh {
		return data, imageContentType(data), nil // fetched while we waited
	}

	info, err := cli.GetProfilePictureInfo(ctx, target, &whatsmeow.GetProfilePictureParams{Preview: true})
	switch {
	case errors.Is(err, whatsmeow.ErrProfilePictureNotSet), errors.Is(err, whatsmeow.ErrProfilePictureUnauthorized):
		m.avatars.storeNegative(key)
		return nil, "", nil
	case err != nil:
		return fallback(err)
	case info == nil || info.URL == "":
		m.avatars.storeNegative(key)
		return nil, "", nil
	}
	data, err := m.avatars.download(ctx, info.URL)
	if errors.Is(err, errNoAvatar) {
		m.avatars.storeNegative(key)
		return nil, "", nil
	} else if err != nil {
		return fallback(err)
	}
	m.avatars.storeImage(key, data)
	return data, imageContentType(data), nil
}
