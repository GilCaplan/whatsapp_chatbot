package engine

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"whatsappdoppel/internal/core/events"
	"whatsappdoppel/internal/core/model"
	"whatsappdoppel/internal/logic/behavior"
	"whatsappdoppel/internal/runtime/prompt"
)

// Connection ratings (model.RatingDay): at the daily recap time every
// enabled group with at least ratingMinMessages messages in the last 24
// hours gets one private call in which the persona scores, in character, how
// connected it felt to each person who wrote. On demand (RateNow) it rates
// whatever the last 24 hours hold. Nothing goes to WhatsApp.

const (
	ratingMinMessages = 20
	ratingMaxPeople   = 25
	ratingTimeout     = 90 * time.Second
	ratingRetry       = 2 * time.Minute // a reply was being written: try again
)

// ErrNothingToRate: nobody else wrote in the chat in the last 24 hours.
var ErrNothingToRate = errors.New("nobody wrote here in the last 24 hours")

// armRatings (re)arms the daily ratings timer: after d (> 0, a retry) or at
// the next recap.time. Off while no group chat has replies on (Reload re-arms).
func (e *Engine) armRatings(d time.Duration) {
	s := e.cfg.Get().Recap
	groups := slices.ContainsFunc(e.store.Chats(), func(c model.ChatAssignment) bool { return c.Enabled && isGroupChat(c) })
	e.ratings.mu.Lock()
	defer e.ratings.mu.Unlock()
	if e.ratings.timer != nil {
		e.ratings.timer.Stop()
		e.ratings.timer = nil
	}
	if e.ctx.Err() != nil || !groups {
		return
	}
	now := e.clock.Now()
	at := nextRecapAt(now, s.Time, time.Local)
	if d > 0 {
		at = now.Add(d)
	}
	e.ratings.next = at
	e.ratings.timer = e.clock.AfterFunc(at.Sub(now), func() { e.goBusy(e.ratingsTick) })
}

// ensureRatings arms the timer if a group now needs it and none is set, and
// stops it when no group has replies on (a pending retry is kept).
func (e *Engine) ensureRatings() {
	groups := slices.ContainsFunc(e.store.Chats(), func(c model.ChatAssignment) bool { return c.Enabled && isGroupChat(c) })
	e.ratings.mu.Lock()
	armed := e.ratings.timer != nil
	e.ratings.mu.Unlock()
	switch {
	case !groups && armed:
		e.stopRatings()
	case groups && !armed:
		e.armRatings(0)
	}
}

func (e *Engine) stopRatings() {
	e.ratings.mu.Lock()
	if e.ratings.timer != nil {
		e.ratings.timer.Stop()
		e.ratings.timer = nil
	}
	e.ratings.mu.Unlock()
}

// ratingsTick rates every eligible group, one at a time (own goroutine).
func (e *Engine) ratingsTick() {
	if e.ctx.Err() != nil {
		return
	}
	if e.anyGenerating() {
		e.armRatings(ratingRetry)
		return
	}
	for _, c := range e.store.Chats() {
		if !c.Enabled || !isGroupChat(c) || e.ctx.Err() != nil {
			continue
		}
		if e.yieldingSlot(e.ctx, func(ctx context.Context) {
			if _, err := e.rateChat(ctx, c, false); err != nil && !errors.Is(err, ErrNothingToRate) && ctx.Err() == nil {
				e.act(model.ActError, c, "", "Couldn't rate the people here: "+err.Error(), nil)
			}
		}) {
			e.armRatings(ratingRetry)
			return
		}
	}
	e.armRatings(0)
}

// RateNow rates the people of a chat from its last 24 hours now.
func (e *Engine) RateNow(ctx context.Context, chatKey string) (model.RatingDay, error) {
	c, ok := e.store.Chat(chatKey)
	if !ok {
		return model.RatingDay{}, fmt.Errorf("chat %q: %w", chatKey, ErrNotFound)
	}
	var d model.RatingDay
	var err error
	if !e.withJobSlot(ctx, func() { d, err = e.rateChat(ctx, c, true) }) {
		return d, ctx.Err()
	}
	return d, err
}

// rateChat writes and saves one day's ratings. Automatic runs skip quiet
// chats (fewer than ratingMinMessages) and chats already rated today.
func (e *Engine) rateChat(ctx context.Context, c model.ChatAssignment, onDemand bool) (model.RatingDay, error) {
	now := e.clock.Now()
	today := now.In(time.Local).Format("2006-01-02")
	prev, _ := e.store.Ratings(c.Key)
	if !onDemand && len(prev) > 0 && prev[0].Date == today {
		return model.RatingDay{}, ErrNothingToRate
	}
	p, ok := e.store.Persona(c.PersonaID)
	if !ok {
		return model.RatingDay{}, fmt.Errorf("persona %q not found", c.PersonaID)
	}
	all, err := e.store.History(c.Key, 0)
	if err != nil {
		return model.RatingDay{}, err
	}
	since := now.Add(-24 * time.Hour)
	var day []model.Message
	for _, m := range all {
		if !m.TS.Before(since) && m.Kind != model.MsgKindFix && m.Kind != model.MsgKindReveal {
			day = append(day, m)
		}
	}
	if !onDemand && len(day) < ratingMinMessages {
		return model.RatingDay{}, ErrNothingToRate
	}
	// Who wrote today, by person (phone/lid user part), most active first.
	type who struct {
		name, jid string
		n         int
	}
	var people []*who
	byID := map[string]*who{}
	for _, m := range day {
		if m.Speaker != "them" || strings.TrimSpace(m.Name) == "" {
			continue
		}
		id := behavior.UserOf(m.SenderJID)
		if id == "" {
			id = "name:" + strings.ToLower(m.Name)
		}
		w := byID[id]
		if w == nil {
			w = &who{jid: m.SenderJID}
			byID[id] = w
			people = append(people, w)
		}
		w.name, w.n = m.Name, w.n+1 // the latest name (nicknames applied)
	}
	if len(people) == 0 {
		return model.RatingDay{}, ErrNothingToRate
	}
	slices.SortStableFunc(people, func(a, b *who) int { return b.n - a.n })
	people = people[:min(len(people), ratingMaxPeople)]
	names := make([]string, len(people))
	for i, w := range people {
		names[i] = w.name
	}
	prov, mdl, err := e.llm.Resolve(p.LLM)
	if err != nil {
		return model.RatingDay{}, err
	}
	req := prompt.Rating(prompt.RatingInput{Persona: p, ChatName: c.Name, People: names, Messages: day, Zone: time.Local})
	req.Model = mdl
	cctx, cancel := context.WithTimeout(ctx, ratingTimeout)
	resp, err := prov.Chat(cctx, req)
	cancel()
	if err != nil {
		return model.RatingDay{}, err
	}
	rated, err := prompt.ParseRating(resp.Text)
	if err != nil {
		return model.RatingDay{}, errors.New("the model didn't write readable ratings")
	}
	d := model.RatingDay{ChatKey: c.Key, Date: today, At: now, Messages: len(day), OnDemand: onDemand, Scores: []model.PersonScore{}}
	seen := map[*who]bool{}
	for _, r := range rated {
		i := slices.IndexFunc(people, func(w *who) bool { return strings.EqualFold(w.name, r.Name) })
		if i < 0 || seen[people[i]] {
			continue // a name it made up, or twice
		}
		w := people[i]
		seen[w] = true
		ps := model.PersonScore{Name: w.name, JID: w.jid, Score: r.Score, Why: r.Why}
		if last, ok := lastScore(prev, today, w.jid, w.name); ok {
			ch := r.Score - last
			ps.Change = &ch
		}
		d.Scores = append(d.Scores, ps)
	}
	if len(d.Scores) == 0 {
		return model.RatingDay{}, errors.New("the model rated nobody from the chat")
	}
	slices.SortStableFunc(d.Scores, func(a, b model.PersonScore) int { return b.Score - a.Score })
	if err := e.store.SaveRating(d); err != nil {
		return model.RatingDay{}, err
	}
	top := d.Scores[0]
	e.act(model.ActRating, c, p.Name, fmt.Sprintf("Rated how connected %s felt to %d %s today — closest: %s (%d)",
		p.Name, len(d.Scores), plural(len(d.Scores), "person", "people"), top.Name, top.Score),
		map[string]any{"people": len(d.Scores), "onDemand": onDemand})
	if e.hub != nil {
		e.hub.Publish(events.TypeRatingsChanged, map[string]string{"chatKey": c.Key})
	}
	return d, nil
}

// lastScore is a person's score on the latest day before today.
func lastScore(prev []model.RatingDay, today, jid, name string) (int, bool) {
	u := behavior.UserOf(jid)
	for _, d := range prev { // newest first
		if d.Date == today {
			continue
		}
		for _, s := range d.Scores {
			if (u != "" && behavior.UserOf(s.JID) == u) || (u == "" && strings.EqualFold(s.Name, name)) {
				return s.Score, true
			}
		}
	}
	return 0, false
}

func plural(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}
