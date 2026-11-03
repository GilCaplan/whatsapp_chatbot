package wa

import (
	"context"
	"slices"
	"time"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/types"
	waEvents "go.mau.fi/whatsmeow/types/events"

	"whatsappdoppel/internal/events"
	"whatsappdoppel/internal/model"
)

// Group membership: the joined-groups cache is the source of truth. It is
// refreshed on connect, on every page load (RefreshChats) and every
// groupsTTL, and patched live when WhatsApp reports that you were added to or
// removed from a group. Every change is reported to the group handler (the
// engine), as a chats.changed event and in the activity feed.

// SetGroupHandler registers the callback for groups you join or leave.
func (m *Manager) SetGroupHandler(fn func(model.GroupChange)) {
	m.mu.Lock()
	m.groupHandler = fn
	m.mu.Unlock()
}

// InGroup reports whether you are a member of the group with this JID;
// known is false until the group list has been loaded once. Never blocks.
func (m *Manager) InGroup(jid string) (member, known bool) {
	m.groupsMu.RLock()
	defer m.groupsMu.RUnlock()
	if m.groups == nil {
		return false, false
	}
	_, ok := findGroup(m.groups, jid)
	return ok, true
}

func findGroup(gs []model.ChatItem, jid string) (int, bool) {
	i := slices.IndexFunc(gs, func(g model.ChatItem) bool { return g.JID == jid })
	return i, i >= 0
}

// setGroups replaces the group list and reports what changed since the last
// one (nothing on the first load: there is nothing to compare with).
func (m *Manager) setGroups(items []model.ChatItem) {
	m.groupsMu.Lock()
	old := m.groups
	m.groups, m.groupsAt = items, time.Now()
	m.groupsMu.Unlock()
	if old == nil {
		return
	}
	var changes []model.GroupChange
	for _, g := range old {
		if _, ok := findGroup(items, g.JID); !ok {
			changes = append(changes, groupChange(g, false))
		}
	}
	for _, g := range items {
		if _, ok := findGroup(old, g.JID); !ok {
			changes = append(changes, groupChange(g, true))
		}
	}
	m.reportGroups(changes)
}

// onGroupInfo handles membership changes that include you: removed (or the
// group was deleted) drops it from the list; added back refetches the list.
func (m *Manager) onGroupInfo(cli *whatsmeow.Client, v *waEvents.GroupInfo) {
	self := m.selfIDs()
	jid := v.JID.ToNonAD().String()
	switch {
	case v.Delete != nil || containsSelf(v.Leave, self):
		m.groupsMu.Lock()
		var gone []model.GroupChange
		if i, ok := findGroup(m.groups, jid); ok {
			gone = append(gone, groupChange(m.groups[i], false))
			m.groups = slices.Delete(slices.Clone(m.groups), i, i+1)
		}
		m.groupsMu.Unlock()
		m.reportGroups(gone)
	case containsSelf(v.Join, self):
		go func() {
			ctx, cancel := context.WithTimeout(m.baseCtx, groupsTimeout)
			defer cancel()
			m.groupsMu.Lock()
			m.groupsAt = time.Time{} // make fetchGroups ask WhatsApp
			m.groupsMu.Unlock()
			if _, err := m.fetchGroups(ctx, cli); err != nil {
				m.log.Warnf("refresh groups after being added: %v", err)
			}
		}()
	}
}

// onJoinedGroup adds a group you just joined (or were added to).
func (m *Manager) onJoinedGroup(v *waEvents.JoinedGroup) {
	g := groupItem(&v.GroupInfo)
	m.groupsMu.Lock()
	if m.groups == nil {
		m.groupsMu.Unlock()
		return // the first full load will include it
	}
	if _, ok := findGroup(m.groups, g.JID); ok {
		m.groupsMu.Unlock()
		return
	}
	gs := append(slices.Clone(m.groups), g)
	sortByName(gs)
	m.groups = gs
	m.groupsMu.Unlock()
	m.reportGroups([]model.GroupChange{groupChange(g, true)})
}

func groupChange(g model.ChatItem, joined bool) model.GroupChange {
	return model.GroupChange{Key: g.Key, JID: g.JID, Name: g.Name, Joined: joined}
}

func containsSelf(jids []types.JID, self selfIDs) bool {
	for _, j := range jids {
		if u := j.User; u != "" && (u == self.PN.User || u == self.LID.User) {
			return true
		}
	}
	return false
}

func (m *Manager) reportGroups(changes []model.GroupChange) {
	if len(changes) == 0 {
		return
	}
	m.mu.Lock()
	h := m.groupHandler
	m.mu.Unlock()
	for _, c := range changes {
		text := "You are no longer in the group " + c.Name
		if c.Joined {
			text = "You joined the group " + c.Name
		}
		m.activity(model.ActSystem, text, map[string]any{"group": c.Key, "joined": c.Joined})
		if h != nil {
			h(c)
		}
	}
	m.publish(events.TypeChatsChanged, map[string]any{})
}
