package server

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"whatsappdoppel/internal/behavior"
	"whatsappdoppel/internal/crossctx"
	"whatsappdoppel/internal/model"
)

// Cross-chat context: a persona may use what it learned in its other chats
// with the same people (see crossctx and docs/API.md "Cross-chat context").
// Settings live in settings.memory.cross, per chat in ChatAssignment.cross,
// per person in people[].cross and per memory in its scope.

func (s *Server) crossRules() crossctx.Rules {
	if s.d.Config == nil {
		return crossctx.Defaults()
	}
	return s.d.Config.Get().Memory.Cross.Rules()
}

// chatShares reports whether what is learned in c may reach other chats now:
// the feature and the chat's sharing are on, learning is on, it isn't
// paused or revealed, and the same persona has another chat.
func (s *Server) chatShares(c model.ChatAssignment, r crossctx.Rules) bool {
	if ok, _ := crossctx.ShareFor(r, c); !ok || !s.memoryEnabled(c) || c.Handoff != nil || c.RevealedAt != nil || c.PersonaID == "" {
		return false
	}
	for _, o := range s.d.Store.Chats() {
		if o.Key != c.Key && o.PersonaID == c.PersonaID {
			return true
		}
	}
	return false
}

// dmChatFor finds the same persona's private chat with a group member.
func (s *Server) dmChatFor(c model.ChatAssignment, ids ...string) (model.ChatAssignment, bool) {
	want := map[string]bool{}
	for _, id := range ids {
		if u := behavior.UserOf(id); u != "" {
			want[u] = true
		}
	}
	var best model.ChatAssignment
	found := false
	for _, o := range s.d.Store.Chats() {
		if o.Kind == "group" || o.PersonaID != c.PersonaID || o.PersonaID == "" {
			continue
		}
		cands := []string{o.JID, o.AltJID, strings.TrimPrefix(strings.TrimPrefix(o.Key, "dm:"), "lid:")}
		for _, id := range cands {
			if u := behavior.UserOf(id); u != "" && want[u] {
				if !found || (o.LastActivityAt != nil && (best.LastActivityAt == nil || o.LastActivityAt.After(*best.LastActivityAt))) {
					best, found = o, true
				}
				break
			}
		}
	}
	return best, found
}

// personCross fills a group member's cross-chat fields.
func (s *Server) personCross(c model.ChatAssignment, pv *personView, ids ...string) {
	pv.Cross, pv.CrossSource = true, crossctx.SourceDefault
	if i := behavior.FindPerson(c.People.People, ids...); i >= 0 && c.People.People[i].Cross != nil {
		pv.Cross, pv.CrossSource = *c.People.People[i].Cross, behavior.SourcePerson
	}
	if dm, ok := s.dmChatFor(c, ids...); ok {
		pv.DMChatKey = dm.Key
		pv.DMShares = s.chatShares(dm, s.crossRules())
	}
}

// parseCrossPatch reads PATCH /api/chats/{key} "cross": {mode?: string|null,
// share?: bool|null} (absent = keep, null = the app default).
func parseCrossPatch(raw json.RawMessage, cur model.CrossContext) (model.CrossContext, error) {
	if string(bytes.TrimSpace(raw)) == "null" {
		return model.CrossContext{}, nil
	}
	var top map[string]json.RawMessage
	if err := json.Unmarshal(raw, &top); err != nil || top == nil {
		return cur, errors.New("cross must be an object")
	}
	out := cur
	for k, v := range top {
		t := string(bytes.TrimSpace(v))
		switch k {
		case "mode":
			if t == "null" {
				out.Mode = ""
				continue
			}
			var m string
			if err := json.Unmarshal(v, &m); err != nil || (m != "" && !crossctx.ValidMode(m)) {
				return cur, fmt.Errorf("cross.mode must be %s or null", strings.Join(model.CrossModes, ", "))
			}
			out.Mode = m
		case "share":
			switch t {
			case "null":
				out.Share = nil
			case "true", "false":
				b := t == "true"
				out.Share = &b
			default:
				return cur, errors.New("cross.share must be true, false or null")
			}
		default:
			return cur, fmt.Errorf("cross: unknown field %q", k)
		}
	}
	return out, nil
}

// GET /api/chats/{key}/cross
func (s *Server) handleChatCross(w http.ResponseWriter, r *http.Request) {
	if s.d.Store == nil || s.d.Engine == nil {
		unavailable(w, "Engine")
		return
	}
	c, ok := s.requireChat(w, r.PathValue("key"))
	if !ok {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
	defer cancel()
	v, err := s.d.Engine.CrossView(ctx, c.Key)
	if writeNotImplemented(w, err, "CrossView") {
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "load_failed", err.Error())
		return
	}
	if v.Sources == nil {
		v.Sources = []model.CrossSource{}
	}
	writeJSON(w, http.StatusOK, v)
}

// POST /api/chats/{key}/cross/refresh
func (s *Server) handleRefreshBrief(w http.ResponseWriter, r *http.Request) {
	if s.d.Store == nil || s.d.Engine == nil {
		unavailable(w, "Engine")
		return
	}
	c, ok := s.requireChat(w, r.PathValue("key"))
	if !ok {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 90*time.Second)
	defer cancel()
	b, err := s.d.Engine.RefreshBrief(ctx, c.Key)
	if writeNotImplemented(w, err, "RefreshBrief") {
		return
	}
	if err != nil {
		writeError(w, http.StatusBadGateway, "brief_failed", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, b)
}
