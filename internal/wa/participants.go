package wa

import (
	"context"
	"fmt"
	"sync"
	"time"

	"go.mau.fi/whatsmeow/types"

	"whatsappdoppel/internal/model"
)

// participantsTTL is how long a group's member list is cached.
const participantsTTL = 10 * time.Minute

type participantsEntry struct {
	at    time.Time
	items []model.Participant
}

// participantsCache caches group rosters per group JID. fetch serialises
// network lookups per group.
type participantsCache struct {
	mu      sync.Mutex
	entries map[string]participantsEntry
	fetch   map[string]*sync.Mutex
}

func (c *participantsCache) get(jid string) ([]model.Participant, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	e, ok := c.entries[jid]
	if !ok || time.Since(e.at) > participantsTTL {
		return nil, false
	}
	return e.items, true
}

func (c *participantsCache) put(jid string, items []model.Participant) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.entries == nil {
		c.entries = map[string]participantsEntry{}
	}
	c.entries[jid] = participantsEntry{at: time.Now(), items: items}
}

func (c *participantsCache) lock(jid string) *sync.Mutex {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.fetch == nil {
		c.fetch = map[string]*sync.Mutex{}
	}
	m := c.fetch[jid]
	if m == nil {
		m = &sync.Mutex{}
		c.fetch[jid] = m
	}
	return m
}

func (c *participantsCache) invalidate(jid string) {
	c.mu.Lock()
	delete(c.entries, jid)
	c.mu.Unlock()
}

func (c *participantsCache) clear() {
	c.mu.Lock()
	c.entries = nil
	c.mu.Unlock()
}

// GroupParticipants returns a group's members with names from the address
// book (full name > push name > business name). Cached for participantsTTL
// and dropped when WhatsApp reports a change to the group. (nil, nil) for
// anything that is not a group.
func (m *Manager) GroupParticipants(ctx context.Context, groupJID string) ([]model.Participant, error) {
	g, err := parseTarget(groupJID)
	if err != nil || g.Server != types.GroupServer {
		return nil, nil
	}
	key := g.String()
	if items, ok := m.participants.get(key); ok {
		return items, nil
	}
	l := m.participants.lock(key)
	l.Lock()
	defer l.Unlock()
	if items, ok := m.participants.get(key); ok {
		return items, nil // fetched by a concurrent caller
	}
	cli := m.currentClient()
	if !cli.IsLoggedIn() {
		return nil, ErrNotConnected
	}
	info, err := cli.GetGroupInfo(ctx, g)
	if err != nil {
		return nil, fmt.Errorf("load group members: %w", err)
	}
	names := func(j types.JID) types.ContactInfo {
		if j.User == "" || cli.Store.Contacts == nil {
			return types.ContactInfo{}
		}
		ci, err := cli.Store.Contacts.GetContact(ctx, j.ToNonAD())
		if err != nil {
			return types.ContactInfo{}
		}
		return ci
	}
	items := participantsFromGroup(info, names, m.selfIDs())
	m.participants.put(key, items)
	return items, nil
}

// participantsFromGroup maps whatsmeow's member list (pure; names looks up
// contact info by phone or lid JID).
func participantsFromGroup(g *types.GroupInfo, names func(types.JID) types.ContactInfo, self selfIDs) []model.Participant {
	if g == nil {
		return nil
	}
	out := make([]model.Participant, 0, len(g.Participants))
	for _, gp := range g.Participants {
		jid, ph, lid := gp.JID.ToNonAD(), gp.PhoneNumber.ToNonAD(), gp.LID.ToNonAD()
		switch {
		case ph.User == "" && jid.Server == types.DefaultUserServer:
			ph = jid
		case lid.User == "" && jid.Server == types.HiddenUserServer:
			lid = jid
		}
		var n nameParts
		n.fill(names(ph))
		n.fill(names(lid))
		p := model.Participant{
			JID:     jid.String(),
			Phone:   ph.User,
			LID:     jidString(lid),
			Name:    n.best(),
			IsAdmin: gp.IsAdmin || gp.IsSuperAdmin,
		}
		if self.linked() && ((ph.User != "" && ph.User == self.PN.User) || (lid.User != "" && lid.User == self.LID.User)) {
			p.IsSelf = true
		}
		out = append(out, p)
	}
	return out
}
