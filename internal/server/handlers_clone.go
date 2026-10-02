package server

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"whatsappdoppel/internal/behavior"
	"whatsappdoppel/internal/model"
)

// Clone yourself (wave 3, owner: Engineer A). With your consent (settings
// clone.collectSamples) Doppel keeps redacted samples of messages you type;
// the builder drafts a persona that texts like you (not saved — the UI opens
// it in the persona editor). See WAVE3_PLAN §3.11.

// clonePreview is how many recent samples GET /api/clone/samples shows.
const clonePreview = 15

func (s *Server) handleCloneSamples(w http.ResponseWriter, r *http.Request) {
	if s.d.Store == nil || s.d.Config == nil {
		unavailable(w, "Store")
		return
	}
	count, since := s.d.Store.SelfSampleStats()
	v := model.CloneSamplesView{Enabled: s.d.Config.Get().SelfClone.CollectSamples, Count: count, Since: since, Preview: []string{}}
	if samples, err := s.d.Store.SelfSamples(clonePreview); err == nil {
		for _, x := range samples {
			v.Preview = append(v.Preview, x.Text)
		}
	}
	writeJSON(w, http.StatusOK, v)
}

func (s *Server) handleDeleteCloneSamples(w http.ResponseWriter, r *http.Request) {
	if s.d.Store == nil {
		unavailable(w, "Store")
		return
	}
	if err := s.d.Store.ClearSelfSamples(); err != nil {
		writeError(w, http.StatusInternalServerError, "delete_failed", err.Error())
		return
	}
	writeOK(w)
}

func (s *Server) handleCloneDraft(w http.ResponseWriter, r *http.Request) {
	var body model.CloneRequest
	if !decodeJSON(w, r, &body) {
		return
	}
	if s.d.Store != nil {
		if n, _ := s.d.Store.SelfSampleStats(); n < model.MinCloneSamples {
			writeError(w, http.StatusBadRequest, "too_few_samples",
				fmt.Sprintf("Doppel has %d of your messages so far and needs at least %d. Keep sample collection on and chat as usual for a while.", n, model.MinCloneSamples))
			return
		}
	}
	if s.d.Builder == nil {
		unavailable(w, "AI builder")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 150*time.Second)
	defer cancel()
	out, err := s.d.Builder.CloneDraft(ctx, body)
	if err != nil {
		writeError(w, http.StatusBadGateway, "draft_failed", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, out)
}

// withPersonaZone resolves the "persona" active-hours zone marker with the
// chat persona's time zone (Effective is reported with the marker as set).
func (s *Server) withPersonaZone(eff behavior.Effective, c model.ChatAssignment) behavior.Effective {
	if !behavior.FollowsPersona(eff.Profile.Availability) || s.d.Store == nil {
		return eff
	}
	tz := ""
	if p, ok := s.d.Store.Persona(c.PersonaID); ok {
		tz = p.World.Timezone
	}
	return behavior.WithZone(eff, tz)
}
