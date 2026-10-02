package server

import (
	"context"
	"net/http"
	"slices"
	"strings"
	"time"

	"whatsappdoppel/internal/model"
)

func (s *Server) handleWAStatus(w http.ResponseWriter, r *http.Request) {
	if s.d.WA == nil {
		unavailable(w, "WhatsApp")
		return
	}
	writeJSON(w, http.StatusOK, s.d.WA.Status())
}

func (s *Server) handleWAPair(w http.ResponseWriter, r *http.Request) {
	if s.d.WA == nil {
		unavailable(w, "WhatsApp")
		return
	}
	if err := s.d.WA.Pair(); err != nil {
		writeError(w, http.StatusConflict, "pair_failed", err.Error())
		return
	}
	writeOK(w)
}

func (s *Server) handleWAQR(w http.ResponseWriter, r *http.Request) {
	if s.d.WA == nil {
		unavailable(w, "WhatsApp")
		return
	}
	png := s.d.WA.CurrentQR()
	if len(png) == 0 {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("Cache-Control", "no-store")
	w.Write(png)
}

func (s *Server) handleWAReconnect(w http.ResponseWriter, r *http.Request) {
	if s.d.WA == nil {
		unavailable(w, "WhatsApp")
		return
	}
	if err := s.d.WA.Reconnect(); err != nil {
		writeError(w, http.StatusConflict, "reconnect_failed", err.Error())
		return
	}
	writeOK(w)
}

func (s *Server) handleWADisconnect(w http.ResponseWriter, r *http.Request) {
	if s.d.WA == nil {
		unavailable(w, "WhatsApp")
		return
	}
	s.d.WA.Disconnect()
	writeOK(w)
}

func (s *Server) handleWALogout(w http.ResponseWriter, r *http.Request) {
	if s.d.WA == nil {
		unavailable(w, "WhatsApp")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	if err := s.d.WA.Logout(ctx); err != nil {
		writeError(w, http.StatusBadGateway, "logout_failed", err.Error())
		return
	}
	writeOK(w)
}

func (s *Server) handleWARefresh(w http.ResponseWriter, r *http.Request) {
	if s.d.WA == nil {
		unavailable(w, "WhatsApp")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 60*time.Second)
	defer cancel()
	if err := s.d.WA.RefreshChats(ctx); err != nil {
		writeError(w, http.StatusBadGateway, "refresh_failed", err.Error())
		return
	}
	writeOK(w)
}

func (s *Server) handleWAAvatar(w http.ResponseWriter, r *http.Request) {
	if s.d.WA == nil {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	jid := strings.TrimSpace(r.URL.Query().Get("jid"))
	if jid == "" {
		writeError(w, http.StatusBadRequest, "missing_jid", "jid is required")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
	defer cancel()
	data, ct, err := s.d.WA.Avatar(ctx, jid)
	if err != nil || len(data) == 0 {
		w.Header().Set("Cache-Control", "private, max-age=300")
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if ct == "" {
		ct = http.DetectContentType(data)
	}
	w.Header().Set("Content-Type", ct)
	w.Header().Set("Cache-Control", "private, max-age=3600")
	w.Write(data)
}

// handleWAChats lists pickable chats. tab=assigned is served from the store;
// the other tabs come from WhatsApp with `assigned` filled in.
func (s *Server) handleWAChats(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	cq := model.ChatQuery{
		Q:      strings.TrimSpace(q.Get("q")),
		Kind:   q.Get("kind"),
		Tab:    q.Get("tab"),
		Limit:  clamp(queryInt(r, "limit", 50), 1, 500),
		Offset: max(queryInt(r, "offset", 0), 0),
	}
	if cq.Kind != "dm" && cq.Kind != "group" {
		cq.Kind = "all"
	}
	if cq.Tab == "" {
		cq.Tab = "recent"
	}
	if cq.Tab == "assigned" {
		items := s.assignedChatItems(cq)
		total := len(items)
		writeJSON(w, http.StatusOK, map[string]any{"items": page(items, cq.Offset, cq.Limit), "total": total})
		return
	}
	if s.d.WA == nil {
		unavailable(w, "WhatsApp")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	items, total, err := s.d.WA.ListChats(ctx, cq)
	if err != nil {
		writeError(w, http.StatusBadGateway, "list_failed", err.Error())
		return
	}
	keys, jids := s.assignedSets()
	for i := range items {
		it := &items[i]
		it.Assigned = keys[it.Key] || jids[it.JID] || (it.AltJID != "" && jids[it.AltJID])
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": nonNilSlice(items), "total": total})
}

func (s *Server) assignedSets() (keys, jids map[string]bool) {
	keys, jids = map[string]bool{}, map[string]bool{}
	if s.d.Store == nil {
		return
	}
	for _, c := range s.d.Store.Chats() {
		keys[c.Key] = true
		if c.JID != "" {
			jids[c.JID] = true
		}
		if c.AltJID != "" {
			jids[c.AltJID] = true
		}
	}
	return
}

func (s *Server) ownJIDs() map[string]bool {
	own := map[string]bool{}
	if s.d.WA != nil {
		for _, j := range s.d.WA.OwnJIDs() {
			own[j] = true
		}
	}
	return own
}

func (s *Server) assignedChatItems(cq model.ChatQuery) []model.ChatItem {
	items := []model.ChatItem{}
	if s.d.Store == nil {
		return items
	}
	own := s.ownJIDs()
	needle := strings.ToLower(cq.Q)
	for _, c := range s.d.Store.Chats() {
		if cq.Kind != "all" && c.Kind != cq.Kind {
			continue
		}
		phone := ""
		if c.Kind == "dm" && strings.HasPrefix(c.Key, "dm:") {
			phone = strings.TrimPrefix(c.Key, "dm:")
		}
		if needle != "" {
			hay := strings.ToLower(c.Name + "\x00" + c.JID + "\x00" + c.AltJID + "\x00" + c.Key)
			if !strings.Contains(hay, needle) {
				continue
			}
		}
		items = append(items, model.ChatItem{
			Key:           c.Key,
			Kind:          c.Kind,
			JID:           c.JID,
			AltJID:        c.AltJID,
			Name:          c.Name,
			Phone:         phone,
			LastMessageAt: c.LastActivityAt,
			IsSelf:        own[c.JID] || (c.AltJID != "" && own[c.AltJID]),
			Assigned:      true,
		})
	}
	// Most recently active first, never-active last (by creation).
	slices.SortStableFunc(items, func(a, b model.ChatItem) int {
		switch {
		case a.LastMessageAt == nil && b.LastMessageAt == nil:
			return 0
		case a.LastMessageAt == nil:
			return 1
		case b.LastMessageAt == nil:
			return -1
		}
		return b.LastMessageAt.Compare(*a.LastMessageAt)
	})
	return items
}

func page[T any](items []T, offset, limit int) []T {
	if offset >= len(items) {
		return []T{}
	}
	end := min(offset+limit, len(items))
	return items[offset:end]
}
