package server

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"whatsappdoppel/internal/behavior"
	"whatsappdoppel/internal/model"
)

// GET /api/behavior/presets
func (s *Server) handleBehaviorPresets(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"presets": behavior.Presets(),
		"ranges":  behavior.Ranges(),
		"enums":   behavior.Enums(),
		// Vibe dials: 5 levels each, field values per kind (behavior/dials.go).
		"dials":     behavior.DialTable(),
		"dialOrder": behavior.DialIDs,
		"defaults": map[string]model.BehaviorProfile{
			"private": behavior.Default(behavior.KindDM),
			"group":   behavior.Default(behavior.KindGroup),
		},
	})
}

// sampleTexts are the reply lengths the preview assumes, by lengthBias.
var sampleTexts = map[string]string{
	"shorter": "Haha yes, I saw that too. Saturday works for me!",
	"normal":  "Haha yes, I saw that too. Honestly I think Saturday is better, the weather looks way nicer. Let me know what works for you!",
	"longer": "Haha yes, I saw that too and I could not stop laughing. Honestly I think Saturday is the better day, the weather looks way nicer and the market is open. " +
		"We could grab lunch first and then walk over. Let me know what works for you and I will book something!",
	"match": "Haha yes, I saw that too. Saturday works for me, the weather looks nicer!",
}

type samplePhase struct {
	Name     string  `json:"name"` // notice|seen|wait|think|distracted|typing|gap|send
	Sec      float64 `json:"sec"`
	StartSec float64 `json:"startSec"`
}

type sampleBubble struct {
	Text      string  `json:"text"`
	TypingSec float64 `json:"typingSec"`
}

type sampleRun struct {
	TotalSec float64        `json:"totalSec"`
	Phases   []samplePhase  `json:"phases"`
	Bubbles  []sampleBubble `json:"bubbles"`
	Quoted   bool           `json:"quoted"`
}

func secs(d time.Duration) float64 { return math.Round(d.Seconds()*10) / 10 }

// normKind maps "dm"/"private"/"" → "dm" and "group" → "group".
func normKind(k string) (string, bool) {
	switch strings.ToLower(strings.TrimSpace(k)) {
	case "", "dm", "private":
		return behavior.KindDM, true
	case "group":
		return behavior.KindGroup, true
	}
	return "", false
}

// POST /api/behavior/sample — previews how a profile would deliver one reply
// to one incoming message, using the engine's own planner.
func (s *Server) handleBehaviorSample(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Kind     string          `json:"kind"`
		Profile  json.RawMessage `json:"profile"`
		Text     string          `json:"text"`
		Samples  int             `json:"samples"`
		Approval bool            `json:"approval"`
	}
	if !decodeJSON(w, r, &body) {
		return
	}
	kind, ok := normKind(body.Kind)
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid_kind", `kind must be "dm" or "group"`)
		return
	}
	// Missing fields fall back to the defaults for the kind; the profile is
	// clamped, never rejected (the preview must work while a form is mid-edit).
	p := behavior.Default(kind)
	if len(body.Profile) > 0 && string(body.Profile) != "null" {
		if err := json.Unmarshal(body.Profile, &p); err != nil {
			writeError(w, http.StatusBadRequest, "bad_json", "Invalid profile: "+err.Error())
			return
		}
	}
	behavior.Clamp(&p)
	text := strings.TrimSpace(body.Text)
	if text == "" {
		text = sampleTexts[p.LengthBias]
	}
	if utf8.RuneCountInString(text) > 2000 {
		text = string([]rune(text)[:2000])
	}
	n := body.Samples
	if n == 0 {
		n = 5
	}
	n = min(max(n, 1), 10)

	eff := behavior.Effective{Kind: kind, Profile: p}
	rng := behavior.NewSampler(nil)
	quote := &model.QuoteRef{MessageID: "sample", Text: "the message being answered"}
	runs := make([]sampleRun, 0, n)
	var totals []float64
	split := 0
	for range n {
		plan := behavior.PlanDelivery(eff, rng, behavior.PlanInput{
			Text: text, Group: kind == behavior.KindGroup, Approval: body.Approval, Quote: quote,
		})
		run := sampleRun{Phases: []samplePhase{}, Bubbles: []sampleBubble{}}
		var at time.Duration
		add := func(name string, d time.Duration, always bool) {
			if d <= 0 && !always {
				return
			}
			run.Phases = append(run.Phases, samplePhase{Name: name, Sec: secs(d), StartSec: secs(at)})
			at += d
		}
		add("notice", plan.Notice, false)
		if p.MarkRead {
			add("seen", 0, true)
		}
		add("wait", plan.Wait, false)
		add("think", plan.Think, false)
		add("distracted", plan.Distracted, false)
		for _, b := range plan.Bubbles {
			add("gap", b.GapBefore, false)
			add("typing", b.Typing, false)
			add("send", 0, true)
			run.Bubbles = append(run.Bubbles, sampleBubble{Text: b.Text, TypingSec: secs(b.Typing)})
			if b.Quote != nil {
				run.Quoted = true
			}
		}
		run.TotalSec = secs(at)
		if len(plan.Bubbles) > 1 {
			split++
		}
		totals = append(totals, run.TotalSec)
		runs = append(runs, run)
	}
	slices.Sort(totals)
	median := totals[len(totals)/2]
	if len(totals)%2 == 0 {
		median = math.Round((totals[len(totals)/2-1]+totals[len(totals)/2])*5) / 10
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"samples": runs,
		"summary": map[string]any{
			"minTotalSec":    totals[0],
			"medianTotalSec": median,
			"maxTotalSec":    totals[len(totals)-1],
			"splitShare":     math.Round(float64(split)/float64(n)*100) / 100,
		},
	})
}

// GET /api/chats/{key}/behavior — the chat's effective behaviour and where each value comes from.
func (s *Server) handleChatBehavior(w http.ResponseWriter, r *http.Request) {
	if s.d.Store == nil || s.d.Config == nil {
		unavailable(w, "Store")
		return
	}
	c, ok := s.requireChat(w, r.PathValue("key"))
	if !ok {
		return
	}
	writeJSON(w, http.StatusOK, s.chatBehaviorView(c, time.Now()))
}

type chatBehaviorView struct {
	Kind         string                  `json:"kind"`
	Effective    model.BehaviorProfile   `json:"effective"`
	Sources      map[string]string       `json:"sources"`
	Overrides    model.BehaviorOverrides `json:"overrides"`
	Defaults     model.BehaviorProfile   `json:"defaults"`
	SnoozedUntil *time.Time              `json:"snoozedUntil"`
	Available    bool                    `json:"available"`
	NextChangeAt *time.Time              `json:"nextChangeAt"`
	// Goal is the chat's resolved goal settings and progress (handlers_goal.go).
	Goal model.ChatGoal `json:"goal"`
	// Dials places the effective profile on the vibe dials.
	Dials map[string]behavior.DialPos `json:"dials"`
}

func (s *Server) chatBehaviorView(c model.ChatAssignment, now time.Time) chatBehaviorView {
	kind := behavior.KindOf(c)
	base := s.d.Config.Get().Behavior.For(kind)
	eff := behavior.Resolve(base, c)
	open, next := s.withPersonaZone(eff, c).Available(now) // "persona" active-hours zone (wave 3)
	v := chatBehaviorView{
		Kind:         kind,
		Effective:    eff.Profile,
		Sources:      eff.Sources,
		Overrides:    c.Behavior,
		Defaults:     base,
		SnoozedUntil: eff.SnoozedUntil,
		Available:    open,
		Dials:        behavior.ReadDials(eff.Profile, kind),
	}
	if !next.IsZero() {
		v.NextChangeAt = &next
	}
	if s.d.Store != nil {
		v.Goal = s.chatGoal(c, nil)
	}
	return v
}

// parseOverridesPatch decodes a PATCH "behavior" object (unknown fields are
// rejected) and validates its non-null values.
func parseOverridesPatch(raw json.RawMessage) (map[string]json.RawMessage, error) {
	var patch map[string]json.RawMessage
	if err := json.Unmarshal(raw, &patch); err != nil || patch == nil {
		return nil, fmt.Errorf("behavior must be an object")
	}
	var probe model.BehaviorOverrides
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&probe); err != nil {
		return nil, fmt.Errorf("invalid behavior: %v", err)
	}
	if err := behavior.ValidateOverrides(probe); err != nil {
		return nil, err
	}
	return patch, nil
}

// applyOverridesPatch merges patch into cur at the top level: absent keys
// keep their value, null inherits again, and the availability/proactive
// blocks are replaced as a whole.
func applyOverridesPatch(cur model.BehaviorOverrides, patch map[string]json.RawMessage) model.BehaviorOverrides {
	b, _ := json.Marshal(cur)
	var m map[string]json.RawMessage
	_ = json.Unmarshal(b, &m)
	if m == nil {
		m = map[string]json.RawMessage{}
	}
	for k, v := range patch {
		if string(bytes.TrimSpace(v)) == "null" {
			delete(m, k)
		} else {
			m[k] = v
		}
	}
	merged, _ := json.Marshal(m)
	var out model.BehaviorOverrides
	_ = json.Unmarshal(merged, &out)
	return out
}

// parseSnooze decodes "snoozedUntil": null (or a time not in the future) clears it.
func parseSnooze(raw json.RawMessage, now time.Time) (*time.Time, error) {
	if string(bytes.TrimSpace(raw)) == "null" {
		return nil, nil
	}
	var t time.Time
	if err := json.Unmarshal(raw, &t); err != nil {
		return nil, fmt.Errorf("snoozedUntil must be an RFC 3339 time or null")
	}
	if !t.After(now) {
		return nil, nil
	}
	return &t, nil
}
