package server

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	"whatsappdoppel/internal/behavior"
	"whatsappdoppel/internal/model"
)

// personView is one row of GET /api/chats/{key}/people.
type personView struct {
	JID           string     `json:"jid"`
	Name          string     `json:"name"`
	Phone         string     `json:"phone"`
	LID           string     `json:"lid"`
	IsAdmin       bool       `json:"isAdmin"`
	IsSelf        bool       `json:"isSelf"`
	Respond       bool       `json:"respond"`       // effective
	RespondSource string     `json:"respondSource"` // person|mode
	Priority      bool       `json:"priority"`
	Notes         string     `json:"notes"`
	LastSpokeAt   *time.Time `json:"lastSpokeAt"`
	Left          bool       `json:"left"` // has preferences but is no longer in the group
	// Cross-chat context (groups): use the same persona's private chat with
	// this person here (effective; source person|default), that chat's key
	// ("" = none) and whether it shares.
	Cross       bool   `json:"cross"`
	CrossSource string `json:"crossSource"`
	DMChatKey   string `json:"dmChatKey"`
	DMShares    bool   `json:"dmShares"`
}

type peopleView struct {
	Kind          string       `json:"kind"`
	Mode          string       `json:"mode"`          // auto|everyone|selected
	EffectiveMode string       `json:"effectiveMode"` // everyone|selected
	MemberCount   int          `json:"memberCount"`
	Threshold     int          `json:"threshold"` // respondToAllMaxMembers
	AnswerAnyone  bool         `json:"answerAnyoneWhoAddressesIt"`
	Members       []personView `json:"members"`
	MembersError  string       `json:"membersError,omitempty"` // the member list couldn't be loaded
}

// GET /api/chats/{key}/people
func (s *Server) handleChatPeople(w http.ResponseWriter, r *http.Request) {
	if s.d.Store == nil || s.d.Config == nil {
		unavailable(w, "Store")
		return
	}
	c, ok := s.requireChat(w, r.PathValue("key"))
	if !ok {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	writeJSON(w, http.StatusOK, s.chatPeopleView(ctx, c))
}

func (s *Server) chatPeopleView(ctx context.Context, c model.ChatAssignment) peopleView {
	kind := behavior.KindOf(c)
	bp := behavior.Resolve(s.d.Config.Get().Behavior.For(kind), c).Profile
	mode := c.People.Mode
	if mode == "" {
		mode = model.PeopleAuto
	}
	v := peopleView{Kind: kind, Mode: mode, Threshold: bp.RespondToAllMaxMembers, AnswerAnyone: bp.AnswerAnyoneWhoAddressesIt, Members: []personView{}}
	prefOf := func(ids ...string) model.PersonPrefs {
		if i := behavior.FindPerson(c.People.People, ids...); i >= 0 {
			return c.People.People[i]
		}
		return model.PersonPrefs{}
	}

	if kind != behavior.KindGroup {
		pp := prefOf(c.JID, c.AltJID)
		phone := ""
		if strings.HasSuffix(c.JID, "@s.whatsapp.net") {
			phone = behavior.UserOf(c.JID)
		}
		v.EffectiveMode = model.PeopleEveryone
		v.Members = append(v.Members, personView{JID: c.JID, Name: c.Name, Phone: phone, LID: lidOf(c.AltJID),
			Respond: true, RespondSource: behavior.SourcePeopleBy, Notes: pp.Notes})
		return v
	}

	var roster []model.Participant
	if s.d.WA != nil {
		jid := c.JID
		if jid == "" {
			jid = c.Key
		}
		ps, err := s.d.WA.GroupParticipants(ctx, jid)
		if err != nil {
			v.MembersError = "Couldn't load the group's members: " + err.Error()
		}
		roster = ps
	}
	v.MemberCount = len(roster)
	v.EffectiveMode = behavior.EffectivePeopleMode(c.People, v.MemberCount, v.Threshold)

	spoke := s.lastSpoke(c.Key)
	seen := map[int]bool{}
	for _, p := range roster {
		ids := []string{p.JID, p.LID}
		if p.Phone != "" {
			ids = append(ids, p.Phone+"@s.whatsapp.net")
		}
		pv := personView{JID: p.JID, Name: p.Name, Phone: p.Phone, LID: p.LID, IsAdmin: p.IsAdmin, IsSelf: p.IsSelf}
		if i := behavior.FindPerson(c.People.People, ids...); i >= 0 {
			seen[i] = true
			pp := c.People.People[i]
			pv.Priority, pv.Notes = pp.Priority, pp.Notes
			if pv.Name == "" {
				pv.Name = pp.Name
			}
		}
		pv.Respond, pv.RespondSource = behavior.Answerable(c.People, v.MemberCount, v.Threshold, ids...)
		if !p.IsSelf {
			s.personCross(c, &pv, ids...)
		}
		for _, id := range ids {
			if t, ok := spoke[behavior.UserOf(id)]; ok && (pv.LastSpokeAt == nil || t.After(*pv.LastSpokeAt)) {
				t := t
				pv.LastSpokeAt = &t
			}
		}
		v.Members = append(v.Members, pv)
	}
	for i, pp := range c.People.People {
		if seen[i] {
			continue
		}
		pv := personView{JID: pp.JID, Name: pp.Name, LID: lidOf(pp.JID), Priority: pp.Priority, Notes: pp.Notes, Left: len(roster) > 0}
		if strings.HasSuffix(pp.JID, "@s.whatsapp.net") {
			pv.Phone = behavior.UserOf(pp.JID)
		}
		pv.Respond, pv.RespondSource = behavior.Answerable(c.People, v.MemberCount, v.Threshold, pp.JID)
		s.personCross(c, &pv, pp.JID)
		if t, ok := spoke[behavior.UserOf(pp.JID)]; ok {
			pv.LastSpokeAt = &t
		}
		v.Members = append(v.Members, pv)
	}
	sort.SliceStable(v.Members, func(i, j int) bool {
		a, b := v.Members[i], v.Members[j]
		if a.Left != b.Left {
			return b.Left // people who left last
		}
		if a.IsSelf != b.IsSelf {
			return b.IsSelf // then you
		}
		if (a.LastSpokeAt != nil) != (b.LastSpokeAt != nil) {
			return a.LastSpokeAt != nil
		}
		if a.LastSpokeAt != nil && !a.LastSpokeAt.Equal(*b.LastSpokeAt) {
			return a.LastSpokeAt.After(*b.LastSpokeAt)
		}
		an, bn := strings.ToLower(a.Name), strings.ToLower(b.Name)
		if (an == "") != (bn == "") {
			return an != "" // named first
		}
		if an != bn {
			return an < bn
		}
		return a.JID < b.JID
	})
	return v
}

func lidOf(jid string) string {
	if strings.HasSuffix(jid, "@lid") {
		return jid
	}
	return ""
}

// lastSpoke maps a sender's user part to when they last wrote in the chat
// (from the remembered history).
func (s *Server) lastSpoke(key string) map[string]time.Time {
	out := map[string]time.Time{}
	hist, err := s.d.Store.History(key, 500)
	if err != nil {
		return out
	}
	for _, m := range hist {
		if m.Speaker != "them" || m.SenderJID == "" {
			continue
		}
		u := behavior.UserOf(m.SenderJID)
		if t, ok := out[u]; !ok || m.TS.After(t) {
			out[u] = m.TS
		}
	}
	return out
}

// peoplePatch is a parsed PATCH "people" object.
type peoplePatch struct {
	reset   bool // "people": null — forget every preference
	mode    *string
	entries []personPatch
}

type personPatch struct {
	jid      string
	name     *string
	respond  *json.RawMessage // present: null clears, bool sets
	priority *bool
	notes    *string
	cross    *json.RawMessage // present: null clears, bool sets
}

// parsePeoplePatch decodes and validates `people` of PATCH /api/chats/{key}:
// {mode?, people?: [{jid, name?, respond?: bool|null, priority?, notes?, cross?: bool|null}]}.
func parsePeoplePatch(raw json.RawMessage) (peoplePatch, error) {
	var pp peoplePatch
	var top map[string]json.RawMessage
	if err := json.Unmarshal(raw, &top); err != nil || top == nil {
		return pp, fmt.Errorf("people must be an object")
	}
	for k := range top {
		if k != "mode" && k != "people" {
			return pp, fmt.Errorf("people: unknown field %q", k)
		}
	}
	if m, ok := top["mode"]; ok {
		var mode string
		if err := json.Unmarshal(m, &mode); err != nil {
			return pp, fmt.Errorf("people.mode must be a string")
		}
		if err := behavior.ValidatePeople(model.PeopleConfig{Mode: mode}); err != nil {
			return pp, err
		}
		pp.mode = &mode
	}
	if list, ok := top["people"]; ok {
		var items []map[string]json.RawMessage
		if err := json.Unmarshal(list, &items); err != nil {
			return pp, fmt.Errorf("people.people must be a list")
		}
		seen := map[string]bool{}
		for _, it := range items {
			var e personPatch
			for k, v := range it {
				var err error
				switch k {
				case "jid":
					err = json.Unmarshal(v, &e.jid)
				case "name":
					e.name = new(string)
					err = json.Unmarshal(v, e.name)
				case "respond":
					if t := string(bytes.TrimSpace(v)); t != "null" && t != "true" && t != "false" {
						err = fmt.Errorf("respond must be true, false or null")
					}
					vv := v
					e.respond = &vv
				case "cross":
					if t := string(bytes.TrimSpace(v)); t != "null" && t != "true" && t != "false" {
						err = fmt.Errorf("cross must be true, false or null")
					}
					vv := v
					e.cross = &vv
				case "priority":
					e.priority = new(bool)
					err = json.Unmarshal(v, e.priority)
				case "notes":
					e.notes = new(string)
					err = json.Unmarshal(v, e.notes)
				default:
					err = fmt.Errorf("unknown field %q", k)
				}
				if err != nil {
					return pp, fmt.Errorf("people: %v", err)
				}
			}
			probe := model.PersonPrefs{JID: strings.TrimSpace(e.jid)}
			if e.name != nil {
				probe.Name = strings.TrimSpace(*e.name)
			}
			if e.notes != nil {
				probe.Notes = strings.TrimSpace(*e.notes)
			}
			if err := behavior.ValidatePeople(model.PeopleConfig{People: []model.PersonPrefs{probe}}); err != nil {
				return pp, err
			}
			if u := behavior.UserOf(probe.JID); seen[u] {
				return pp, fmt.Errorf("people lists %s twice", probe.JID)
			} else {
				seen[u] = true
			}
			e.jid = probe.JID
			pp.entries = append(pp.entries, e)
		}
	}
	return pp, nil
}

// apply merges the patch into cur (upsert by JID user part); the result is a
// fresh slice, normalised.
func (pp peoplePatch) apply(cur model.PeopleConfig) model.PeopleConfig {
	if pp.reset {
		return model.PeopleConfig{People: []model.PersonPrefs{}}
	}
	out := model.PeopleConfig{Mode: cur.Mode, People: append([]model.PersonPrefs(nil), cur.People...)}
	if pp.mode != nil {
		out.Mode = *pp.mode
	}
	for _, e := range pp.entries {
		i := behavior.FindPerson(out.People, e.jid)
		if i < 0 {
			out.People = append(out.People, model.PersonPrefs{JID: e.jid})
			i = len(out.People) - 1
		}
		p := out.People[i]
		if e.name != nil {
			p.Name = *e.name
		}
		if e.respond != nil {
			switch string(bytes.TrimSpace(*e.respond)) {
			case "true":
				p.Respond = boolPtr(true)
			case "false":
				p.Respond = boolPtr(false)
			default:
				p.Respond = nil
			}
		}
		if e.priority != nil {
			p.Priority = *e.priority
		}
		if e.cross != nil {
			switch string(bytes.TrimSpace(*e.cross)) {
			case "true":
				p.Cross = boolPtr(true)
			case "false":
				p.Cross = boolPtr(false)
			default:
				p.Cross = nil
			}
		}
		if e.notes != nil {
			p.Notes = *e.notes
		}
		out.People[i] = p
	}
	return behavior.NormalizePeople(out)
}

func boolPtr(b bool) *bool { return &b }
