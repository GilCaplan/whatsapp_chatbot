package engine

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"time"

	"whatsappdoppel/internal/behavior"
	"whatsappdoppel/internal/crossctx"
	"whatsappdoppel/internal/mention"
	"whatsappdoppel/internal/model"
	"whatsappdoppel/internal/prompt"
)

// Cross-chat context: a persona draws on what it learned in its OTHER chats
// with the people in the conversation — in a group, what someone told it
// one-to-one; in a private chat, what happened in groups the contact shares
// with it. Only memories and the per-chat brief (brief.go) cross, filtered by
// crossctx (sensitive topics, locks, freshness), and every reply is checked
// with crossctx.Leak (guardedReply) so Discreet stays discreet.

// crossActiveMessages is how far back "people in the conversation" reaches.
const crossActiveMessages = 12

// crossHistoryScan is how many messages of a group are read to tell whether
// a contact is in it when WhatsApp can't list the members.
const crossHistoryScan = 60

// crossUse is the cross-chat context of one reply.
type crossUse struct {
	Mode    string
	IsGroup bool
	Input   prompt.CrossInput
	Items   []crossctx.Item
	People  []string        // display names (here) whose context is used
	Sources []string        // source chat names
	Present map[string]bool // users and first names active in the last messages
	Here    []string        // texts of this chat (for the leak guard)
}

// meta is the crossContext activity meta: never item texts.
func (u *crossUse) meta() map[string]any {
	if u == nil {
		return nil
	}
	return map[string]any{"mode": u.Mode, "people": u.People, "items": len(u.Items), "sources": u.Sources}
}

// leaks checks a reply against the items shown.
func (u *crossUse) leaks(reply string) []string {
	if u == nil || len(u.Items) == 0 {
		return nil
	}
	return crossctx.Leak(crossctx.LeakInput{Items: u.Items, Reply: reply, Here: u.Here, Mode: u.Mode, IsGroup: u.IsGroup, Present: u.Present})
}

// active reports whether the context changes the prompt.
func (u *crossUse) active() bool { return u != nil && len(u.Items) > 0 }

func (e *Engine) crossRules() crossctx.Rules { return e.cfg.Get().Memory.Cross.Rules() }

// crossKeyAllowed: playground and preview chats never draw on real chats.
func crossKeyAllowed(key string) bool {
	return key != "" && !strings.HasPrefix(key, "playground:") && !strings.HasPrefix(key, "preview:")
}

// chatIDs lists every address of a private chat's contact.
func chatIDs(c model.ChatAssignment) []string {
	var ids []string
	for _, j := range []string{c.JID, c.AltJID} {
		if j != "" {
			ids = append(ids, j)
		}
	}
	switch {
	case strings.HasPrefix(c.Key, "dm:"):
		ids = append(ids, strings.TrimPrefix(c.Key, "dm:")+"@s.whatsapp.net")
	case strings.HasPrefix(c.Key, "lid:"):
		ids = append(ids, strings.TrimPrefix(c.Key, "lid:")+"@lid")
	}
	return ids
}

// sameUser reports whether any id of a matches any id of b.
func sameUser(a, b []string) bool {
	for _, x := range a {
		ux := crossctx.UserOf(x)
		if ux == "" {
			continue
		}
		for _, y := range b {
			if crossctx.UserOf(y) == ux {
				return true
			}
		}
	}
	return false
}

// crossBlocked is why chat s can't be a source right now ("" = it can).
func (e *Engine) crossBlocked(s model.ChatAssignment, r crossctx.Rules) string {
	switch {
	case s.Handoff != nil:
		return model.CrossBlockedHandoff
	case s.RevealedAt != nil:
		return model.CrossBlockedRevealed
	case !e.memoryOn(s):
		return model.CrossBlockedMemory
	}
	if ok, _ := crossctx.ShareFor(r, s); !ok {
		return model.CrossBlockedShareOff
	}
	return ""
}

// crossCandidate is a possible source chat for one person.
type crossCandidate struct {
	chat    model.ChatAssignment
	person  crossctx.Person
	blocked string
}

// activePeople are the people in the conversation, most relevant first:
// the person answered now, then recent speakers, then people tagged lately.
func (e *Engine) activePeople(c model.ChatAssignment, d *mention.Directory, hist []model.Message) []crossctx.Person {
	if !isGroupChat(c) {
		name := c.Name
		if i := behavior.FindPerson(c.People.People, chatIDs(c)...); i >= 0 && name == "" {
			name = c.People.People[i].Name
		}
		return []crossctx.Person{{Name: mention.CleanName(name), JID: c.JID, IDs: chatIDs(c)}}
	}
	recent := hist[max(0, len(hist)-crossActiveMessages):]
	var roster []model.Participant
	if d != nil {
		for _, en := range d.Entries() {
			roster = append(roster, en.Participant)
		}
	}
	var out []crossctx.Person
	seen := map[string]bool{}
	add := func(jid, name string) {
		u := mention.User(jid)
		if u == "" || seen[u] || (d != nil && d.IsOwn(jid)) {
			return
		}
		seen[u] = true
		ids := personIDs(jid, roster)
		if d != nil {
			if en, ok := d.ByJID(jid); ok {
				name = en.Display
			}
		}
		out = append(out, crossctx.Person{Name: mention.CleanName(name), JID: jid, IDs: ids, Rank: len(out)})
	}
	for _, jid := range recentSpeakers(recent) {
		name := ""
		for i := len(recent) - 1; i >= 0; i-- {
			if mention.User(recent[i].SenderJID) == mention.User(jid) {
				name = recent[i].Name
				break
			}
		}
		add(jid, name)
	}
	if d != nil {
		for _, m := range hist[max(0, len(hist)-3):] {
			for _, n := range m.Mentions {
				for _, en := range d.Entries() {
					if strings.EqualFold(en.Display, n) {
						add(en.JID, en.Display)
					}
				}
			}
		}
	}
	return out
}

// crossCandidates finds, for each active person, the other chats of the
// same persona that may inform this one (blocked ones included, with why).
func (e *Engine) crossCandidates(ctx context.Context, c model.ChatAssignment, people []crossctx.Person, r crossctx.Rules) []crossCandidate {
	var out []crossCandidate
	chats := e.store.Chats()
	if isGroupChat(c) {
		for _, p := range people {
			var best *crossCandidate
			for _, s := range chats {
				if s.Key == c.Key || s.PersonaID != c.PersonaID || isGroupChat(s) || !sameUser(chatIDs(s), append([]string{p.JID}, p.IDs...)) {
					continue
				}
				cand := crossCandidate{chat: s, person: p, blocked: e.crossBlocked(s, r)}
				if i := behavior.FindPerson(c.People.People, append([]string{p.JID}, p.IDs...)...); i >= 0 {
					if pc := c.People.People[i].Cross; pc != nil && !*pc && cand.blocked == "" {
						cand.blocked = model.CrossBlockedPerson
					}
				}
				// Phone and lid chats of one person: the usable, most recent one.
				if best == nil || (best.blocked != "" && cand.blocked == "") ||
					((best.blocked == "") == (cand.blocked == "") && later(s.LastActivityAt, best.chat.LastActivityAt)) {
					cc := cand
					best = &cc
				}
			}
			if best != nil {
				out = append(out, *best)
			}
		}
		return out
	}
	if len(people) == 0 {
		return nil
	}
	p := people[0]
	ids := append([]string{p.JID}, p.IDs...)
	for _, s := range chats {
		if s.Key == c.Key || s.PersonaID != c.PersonaID || !isGroupChat(s) {
			continue
		}
		if !e.inGroup(ctx, s, ids) {
			continue
		}
		out = append(out, crossCandidate{chat: s, person: p, blocked: e.crossBlocked(s, r)})
	}
	return out
}

func later(a, b *time.Time) bool {
	if a == nil {
		return false
	}
	return b == nil || a.After(*b)
}

// inGroup reports whether a contact (ids) is a member of group s: per its
// roster, else because they wrote there lately.
func (e *Engine) inGroup(ctx context.Context, s model.ChatAssignment, ids []string) bool {
	for _, p := range e.roster(ctx, s) {
		if !p.IsSelf && sameUser([]string{p.JID, p.LID, p.Phone + "@s.whatsapp.net"}, ids) {
			return true
		}
	}
	hist, _ := e.store.History(s.Key, crossHistoryScan)
	for _, m := range hist {
		if m.Speaker == "them" && m.SenderJID != "" && sameUser([]string{m.SenderJID}, ids) {
			return true
		}
	}
	return false
}

// crossItems collects what may cross from one source chat about its person.
func (e *Engine) crossItems(src model.ChatAssignment, p crossctx.Person, targetGroup bool, r crossctx.Rules, now time.Time) []crossctx.Item {
	var out []crossctx.Item
	base := crossctx.Item{Person: p.Name, PersonJID: p.JID, Source: src.Key, SourceName: src.Name, SourceKind: src.Kind}
	if base.PersonJID == "" && len(p.IDs) > 0 {
		base.PersonJID = p.IDs[0]
	}
	mems, _ := e.store.Memories(src.Key)
	for _, m := range mems {
		if isGroupChat(src) {
			// Only what the contact said there, never other members' things.
			if !crossctx.Matches(crossctx.Item{Person: m.Person, PersonJID: m.PersonJID}, p) {
				continue
			}
		}
		if ok, _ := crossctx.Carries(m, r, now); !ok {
			continue
		}
		it := base
		it.Kind, it.Text, it.Evidence, it.At, it.Pinned = crossctx.KindMemory, m.Text, m.Evidence, m.UpdatedAt, m.Pinned
		out = append(out, it)
	}
	b, ok, _ := e.store.Brief(src.Key)
	if !ok || (r.FreshDays > 0 && now.Sub(b.GeneratedAt) > time.Duration(r.FreshDays)*24*time.Hour) {
		return out
	}
	touched := false
	for _, cat := range b.Sensitive {
		if crossctx.SensitiveOn(r, cat) {
			touched = true
		}
	}
	clean := func(s string) bool {
		cat := crossctx.Classify(s)
		return cat == "" || !crossctx.SensitiveOn(r, cat)
	}
	// Promises are the persona's own words: each one is checked on its own.
	for _, cm := range b.Commitments {
		if clean(cm) {
			it := base
			it.Kind, it.Text, it.At = crossctx.KindCommitment, cm, b.GeneratedAt
			out = append(out, it)
		}
	}
	if touched {
		return out // the window touched something sensitive: no topics or notes from it
	}
	var topics []string
	for _, t := range b.Topics {
		if clean(t) {
			topics = append(topics, t)
		}
	}
	if len(topics) > 0 {
		it := base
		label := "lately talking about: "
		if isGroupChat(src) {
			label = "recent topics: "
		}
		it.Kind, it.Text, it.At = crossctx.KindTopic, label+strings.Join(topics, ", "), b.GeneratedAt
		out = append(out, it)
	}
	if isGroupChat(src) {
		for _, bp := range b.People {
			if crossctx.Matches(crossctx.Item{Person: bp.Name, PersonJID: bp.JID}, p) && clean(bp.Note) {
				it := base
				it.Kind, it.Text, it.At = crossctx.KindGroupNote, bp.Note, b.GeneratedAt
				out = append(out, it)
				break
			}
		}
	}
	return out
}

// crossContext builds a reply's cross-chat context (nil when off or empty).
func (e *Engine) crossContext(c model.ChatAssignment, d *mention.Directory, hist []model.Message, now time.Time) *crossUse {
	r := e.crossRules()
	mode, _ := crossctx.ModeFor(r, c)
	if mode == model.CrossOff || !crossKeyAllowed(c.Key) || !e.memoryOn(c) || c.PersonaID == "" {
		return nil
	}
	return e.crossFor(e.ctx, c, d, hist, now, r, mode)
}

func (e *Engine) crossFor(ctx context.Context, c model.ChatAssignment, d *mention.Directory, hist []model.Message, now time.Time, r crossctx.Rules, mode string) *crossUse {
	isGroup := isGroupChat(c)
	people := e.activePeople(c, d, hist)
	if len(people) == 0 {
		return nil
	}
	if r.MaxPeople > 0 && len(people) > r.MaxPeople {
		people = people[:r.MaxPeople]
	}
	var items []crossctx.Item
	for _, cand := range e.crossCandidates(ctx, c, people, r) {
		if cand.blocked == "" {
			items = append(items, e.crossItems(cand.chat, cand.person, isGroup, r, now)...)
		}
	}
	if len(items) == 0 {
		return nil
	}
	var recent []string
	for i := len(hist) - 1; i >= 0 && len(recent) < 3; i-- {
		recent = append(recent, hist[i].Text)
	}
	sel := crossctx.Select(items, people, recent, now, r)
	if len(sel) == 0 {
		return nil
	}
	u := &crossUse{Mode: mode, IsGroup: isGroup, Items: sel, Present: map[string]bool{}}
	// Who is "part of the conversation right now" (Open mode).
	if isGroup {
		for _, m := range hist[max(0, len(hist)-crossActiveMessages):] {
			if m.Speaker == "them" && m.SenderJID != "" {
				u.Present[mention.User(m.SenderJID)] = true
				if n := strings.ToLower(firstWord(m.Name)); n != "" {
					u.Present[n] = true
				}
			}
		}
	} else {
		for _, id := range people[0].IDs {
			u.Present[crossctx.UserOf(id)] = true
		}
		if n := strings.ToLower(firstWord(people[0].Name)); n != "" {
			u.Present[n] = true
		}
	}
	// What this chat already knows (said here, remembered here, your notes).
	for _, m := range hist {
		u.Here = append(u.Here, m.Text)
	}
	if mems, err := e.store.Memories(c.Key); err == nil {
		for _, m := range mems {
			u.Here = append(u.Here, m.Text)
		}
	}
	for _, pp := range c.People.People {
		u.Here = append(u.Here, pp.Notes)
	}
	u.Input = crossInput(u, people, c)
	return u
}

// crossInput turns the selected items into the prompt section's data.
func crossInput(u *crossUse, people []crossctx.Person, c model.ChatAssignment) prompt.CrossInput {
	in := prompt.CrossInput{Mode: u.Mode, IsGroup: u.IsGroup}
	addName := func(n string) {
		if n != "" && !slices.Contains(u.People, n) {
			u.People = append(u.People, n)
		}
	}
	addSource := func(n string) {
		if n != "" && !slices.Contains(u.Sources, n) {
			u.Sources = append(u.Sources, n)
		}
	}
	if u.IsGroup {
		for _, p := range people {
			cp := prompt.CrossPerson{Name: p.Name}
			for _, it := range u.Items {
				if !crossctx.Matches(it, p) {
					continue
				}
				addSource(it.SourceName)
				if it.Kind == crossctx.KindCommitment {
					cp.Commitments = append(cp.Commitments, it.Text)
				} else {
					cp.Items = append(cp.Items, it.Text)
				}
			}
			if len(cp.Items)+len(cp.Commitments) > 0 && cp.Name != "" {
				in.People = append(in.People, cp)
				addName(cp.Name)
			}
		}
		return in
	}
	in.Contact = people[0].Name
	if in.Contact == "" {
		in.Contact = c.Name
	}
	bySource := map[string]*prompt.CrossPerson{}
	var order []string
	for _, it := range u.Items {
		cp, ok := bySource[it.Source]
		if !ok {
			cp = &prompt.CrossPerson{Name: in.Contact, Source: it.SourceName}
			bySource[it.Source] = cp
			order = append(order, it.Source)
		}
		switch it.Kind {
		case crossctx.KindCommitment:
			cp.Commitments = append(cp.Commitments, it.Text)
		case crossctx.KindGroupNote:
			cp.Note = it.Text
		default:
			cp.Items = append(cp.Items, it.Text)
		}
	}
	for _, k := range order {
		in.People = append(in.People, *bySource[k])
		addSource(bySource[k].Source)
	}
	addName(in.Contact)
	return in
}

// crossActivityText describes the context for the feed, never its contents:
// "Keeping in mind 2 things from your private chat with Dana — discreet".
func crossActivityText(u *crossUse) string {
	n := len(u.Items)
	things := fmt.Sprintf("%d things", n)
	if n == 1 {
		things = "one thing"
	}
	how := map[string]string{model.CrossDiscreet: "discreet", model.CrossOpen: "open"}[u.Mode]
	if u.IsGroup {
		who := joinNames(u.People)
		chats := "your private chat with " + who
		if len(u.People) > 1 {
			chats = "your private chats with " + who
		}
		return "Keeping in mind " + things + " from " + chats + " — " + how
	}
	return "Keeping in mind what happened in " + joinNames(u.Sources) + " — " + how
}

// CrossView reports a chat's cross-chat settings, the chats it may draw on
// (and why some are not used) and its own brief (GET /api/chats/{key}/cross).
func (e *Engine) CrossView(ctx context.Context, chatKey string) (model.CrossView, error) {
	c, ok := e.store.Chat(chatKey)
	if !ok {
		return model.CrossView{}, fmt.Errorf("chat %q: %w", chatKey, ErrNotFound)
	}
	r := e.crossRules()
	v := model.CrossView{Enabled: r.Enabled, Kind: c.Kind, Sources: []model.CrossSource{}}
	v.Mode, v.ModeSource = crossctx.ModeFor(r, c)
	v.Share, v.ShareSource = crossctx.ShareFor(r, c)
	if b, ok, _ := e.store.Brief(c.Key); ok {
		v.Brief = &b
	}
	v.BriefPending = e.store.RunnerState(c.Key).BriefPending
	if c.PersonaID == "" {
		return v, nil
	}
	hist := e.chatHistory(e.runner(c.Key), c)
	var people []crossctx.Person
	if isGroupChat(c) {
		// Every member who has a private chat with this persona, not only
		// the people talking right now.
		d := e.directory(ctx, c, hist)
		if d != nil {
			var roster []model.Participant
			for _, en := range d.Entries() {
				roster = append(roster, en.Participant)
			}
			for _, en := range d.Entries() {
				people = append(people, crossctx.Person{Name: en.Display, JID: en.JID, IDs: personIDs(en.JID, roster), Rank: len(people)})
			}
		}
	} else {
		people = e.activePeople(c, nil, hist)
	}
	now := e.clock.Now()
	for _, cand := range e.crossCandidates(ctx, c, people, r) {
		src := model.CrossSource{ChatKey: cand.chat.Key, Name: cand.chat.Name, Kind: cand.chat.Kind, Person: cand.person.Name, PersonJID: cand.person.JID, Blocked: cand.blocked}
		if v.Mode == model.CrossOff && src.Blocked == "" {
			src.Blocked = "mode_off"
		}
		for _, it := range e.crossItems(cand.chat, cand.person, isGroupChat(c), r, now) {
			if it.Kind == crossctx.KindMemory {
				src.Items++
			} else {
				src.HasBrief = true
			}
		}
		v.Sources = append(v.Sources, src)
	}
	return v, nil
}
