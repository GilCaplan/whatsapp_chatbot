package engine

import (
	"sync"
	"time"

	"whatsappdoppel/internal/guard"
	"whatsappdoppel/internal/model"
)

// Decision log (store/decisions.go): one entry per message someone sends in
// an assigned chat, kept up to date from the chat's activity — arrived →
// decided (and why) → the steps on the way → replied (and how long it took),
// or stayed quiet, reacted, paused… Activity about particular messages
// carries their ids in meta (msgId, msgIds, messageId); the reply cycle's
// steps go to every entry still pending. Openers the persona writes first
// get an entry of their own (Kind "opener").

const (
	logTextMax  = 300
	logReplyMax = 500
	logStepsMax = 24
)

// openerWhy remembers why the persona is about to write first, per chat
// (the proactive activity comes before the message goes out).
type openerWhy struct {
	mu  sync.Mutex
	why map[string]string
}

func (o *openerWhy) set(key, why string) {
	o.mu.Lock()
	if o.why == nil {
		o.why = map[string]string{}
	}
	o.why[key] = why
	o.mu.Unlock()
}

func (o *openerWhy) take(key string) string {
	o.mu.Lock()
	defer o.mu.Unlock()
	w := o.why[key]
	delete(o.why, key)
	return w
}

// withMsg adds the ids of the messages an activity is about (empty ids are
// left out; nil meta is fine).
func withMsg(meta map[string]any, ids ...string) map[string]any {
	var keep []string
	for _, id := range ids {
		if id != "" {
			keep = append(keep, id)
		}
	}
	if len(keep) == 0 {
		return meta
	}
	if meta == nil {
		meta = map[string]any{}
	}
	if len(keep) == 1 {
		meta["msgId"] = keep[0]
	} else {
		meta["msgIds"] = keep
	}
	return meta
}

// msgIDs are the ids of the messages a cycle answers.
func (c *cycle) msgIDs() []string {
	ids := make([]string, 0, len(c.pending))
	for _, p := range c.pending {
		ids = append(ids, p.ID)
	}
	return ids
}

// logIDs reads the message ids out of an activity's meta.
func logIDs(meta map[string]any) []string {
	var ids []string
	for _, k := range []string{"msgId", "messageId"} {
		if s, ok := meta[k].(string); ok && s != "" {
			ids = append(ids, s)
		}
	}
	if s, ok := meta["msgIds"].([]string); ok {
		ids = append(ids, s...)
	}
	return ids
}

// logActivity updates the chat's decision log from one activity event.
func (e *Engine) logActivity(typ string, c model.ChatAssignment, text string, meta map[string]any) {
	if c.Key == "" || e.store == nil {
		return
	}
	now := e.clock.Now()
	ids := logIDs(meta)
	reason, _ := meta["reason"].(string)
	switch typ {
	case model.ActIncoming:
		if meta["fromMe"] == true && meta["trigger"] != true {
			return
		}
		sender, _ := meta["sender"].(string)
		if meta["fromMe"] == true {
			sender = "You"
		}
		at := now
		if t, ok := meta["sentAt"].(time.Time); ok && !t.IsZero() && !t.After(now) {
			at = t
		}
		id := ""
		if len(ids) > 0 {
			id = ids[0]
		}
		e.logArrived(c.Key, id, at, sender, text)
	case model.ActDecisionSkip:
		outcome := model.DecisionSkipped
		switch {
		case meta["react"] == true:
			outcome = model.DecisionReacted
		case reason == "handoff":
			outcome = model.DecisionPaused
		}
		e.logDecided(c.Key, ids, outcome, reason, text, now)
	case model.ActDecisionReply:
		e.logDecided(c.Key, ids, model.DecisionPending, reason, text, now)
	case model.ActBlockedInjection:
		e.logDecided(c.Key, ids, model.DecisionBlocked, "injection", "Ignored a likely prompt-injection attempt", now)
	case model.ActHandoff:
		if meta["autoContinued"] == true {
			e.logStep(c.Key, ids, typ, text, now) // flagged, but it carries on
			return
		}
		e.logDecided(c.Key, ids, model.DecisionPaused, "handoff", text, now)
	case model.ActReacted:
		e.logUpdate(c.Key, ids, now, func(d *model.LogEntry) {
			d.Outcome = model.DecisionReacted
			addStep(d, now, typ, text)
		})
	case model.ActError:
		if len(ids) > 0 {
			e.logDecided(c.Key, ids, model.DecisionFailed, "error", text, now)
		}
	case model.ActProactive:
		if meta["stage"] == "starting" {
			e.openers.set(c.Key, text)
		}
	case model.ActSent:
		if meta["manual"] == true || meta["fix"] == true || meta["edited"] == true {
			return
		}
		if len(ids) == 0 && meta["proactive"] == true {
			e.logOpener(c.Key, text, meta["part"], now)
			return
		}
		e.logReplied(c.Key, ids, text, now)
	case model.ActApprovalSent:
		e.logApprovalDone(c.Key, true, text, now)
	case model.ActApprovalDiscarded:
		e.logApprovalDone(c.Key, false, text, now)
	case model.ActNoticing, model.ActSeen, model.ActWaiting, model.ActThinking, model.ActGenerating, model.ActTyping, model.ActDeferred:
		e.logStep(c.Key, ids, typ, text, now)
	}
}

// logArrived adds the entry of a new message (id "" = a generated one).
func (e *Engine) logArrived(chatKey, id string, at time.Time, sender, text string) {
	_ = e.store.UpdateDecisions(chatKey, func(all []model.LogEntry) ([]model.LogEntry, bool) {
		if id != "" {
			for i := range all {
				if all[i].ID == id {
					return all, false // redelivered
				}
			}
		} else {
			id = "local-" + at.Format("20060102T150405.000000000")
		}
		return append(all, model.LogEntry{
			ID: id, ChatKey: chatKey, At: at, Sender: sender,
			Text: guard.Truncate(text, logTextMax), Outcome: model.DecisionPending,
		}), true
	})
}

// logUpdate runs fn on the entries with ids, or — when the activity names
// no message — on the newest pending one (the message being handled).
func (e *Engine) logUpdate(chatKey string, ids []string, now time.Time, fn func(*model.LogEntry)) {
	_ = e.store.UpdateDecisions(chatKey, func(all []model.LogEntry) ([]model.LogEntry, bool) {
		changed := false
		if len(ids) == 0 {
			for i := len(all) - 1; i >= 0; i-- {
				if all[i].Outcome == model.DecisionPending && all[i].Kind == "" {
					fn(&all[i])
					return all, true
				}
			}
			return all, false
		}
		want := map[string]bool{}
		for _, id := range ids {
			want[id] = true
		}
		for i := range all {
			if want[all[i].ID] {
				fn(&all[i])
				changed = true
			}
		}
		return all, changed
	})
}

// logDecided records what the persona decided about messages (outcome
// pending = it decided to answer and is on it).
func (e *Engine) logDecided(chatKey string, ids []string, outcome, reason, why string, now time.Time) {
	e.logUpdate(chatKey, ids, now, func(d *model.LogEntry) {
		if d.Outcome == model.DecisionReplied {
			return // already answered (e.g. "stood down" for a later message)
		}
		d.Outcome, d.Reason, d.Why = outcome, reason, why
		if d.DecidedAt == nil {
			t := now
			d.DecidedAt = &t
		}
		addStep(d, now, "decision", why)
	})
}

// logStep adds a step of the reply cycle to the messages it is about, or to
// every message still pending.
func (e *Engine) logStep(chatKey string, ids []string, typ, text string, now time.Time) {
	_ = e.store.UpdateDecisions(chatKey, func(all []model.LogEntry) ([]model.LogEntry, bool) {
		want := map[string]bool{}
		for _, id := range ids {
			want[id] = true
		}
		changed := false
		for i := range all {
			d := &all[i]
			if d.Kind != "" || (len(want) > 0 && !want[d.ID]) || (len(want) == 0 && d.Outcome != model.DecisionPending) {
				continue
			}
			addStep(d, now, typ, text)
			changed = true
		}
		return all, changed
	})
}

// logReplied marks messages answered by a reply message (the first one sets
// the time; later parts add their text).
func (e *Engine) logReplied(chatKey string, ids []string, text string, now time.Time) {
	e.logUpdate(chatKey, ids, now, func(d *model.LogEntry) {
		if d.RepliedAt == nil {
			t := now
			d.Outcome, d.RepliedAt = model.DecisionReplied, &t
			d.DelaySeconds = now.Sub(d.At).Seconds()
			if d.Why == "" {
				d.Why = "Answered"
			}
			if d.DecidedAt == nil {
				d.DecidedAt = &t
			}
			d.Reply = guard.Truncate(text, logReplyMax)
		} else {
			d.Reply = guard.Truncate(d.Reply+"\n"+text, logReplyMax)
		}
		addStep(d, now, model.ActSent, text)
	})
}

// logOpener records a message the persona wrote first (part 1 starts an
// entry, later parts add to it).
func (e *Engine) logOpener(chatKey, text string, part any, now time.Time) {
	first := true
	if n, ok := part.(int); ok && n > 1 {
		first = false
	}
	why := ""
	if first {
		why = e.openers.take(chatKey)
	}
	_ = e.store.UpdateDecisions(chatKey, func(all []model.LogEntry) ([]model.LogEntry, bool) {
		if !first {
			for i := len(all) - 1; i >= 0; i-- {
				if all[i].Kind == "opener" {
					all[i].Reply = guard.Truncate(all[i].Reply+"\n"+text, logReplyMax)
					return all, true
				}
			}
		}
		if why == "" {
			why = "Wrote first"
		}
		t := now
		return append(all, model.LogEntry{
			ID: "opener-" + now.Format("20060102T150405.000000000"), ChatKey: chatKey, Kind: "opener",
			At: now, Outcome: model.DecisionReplied, Reason: "opener", Why: why,
			DecidedAt: &t, RepliedAt: &t, Reply: guard.Truncate(text, logReplyMax),
		}), true
	})
}

// logApprovalDone closes the messages whose reply waited for your approval.
func (e *Engine) logApprovalDone(chatKey string, sent bool, text string, now time.Time) {
	_ = e.store.UpdateDecisions(chatKey, func(all []model.LogEntry) ([]model.LogEntry, bool) {
		changed := false
		for i := range all {
			d := &all[i]
			if d.Outcome != model.DecisionApproval {
				continue
			}
			changed = true
			if !sent {
				d.Outcome, d.Reason, d.Why = model.DecisionSkipped, "discarded", "You discarded the reply"
				addStep(d, now, model.ActApprovalDiscarded, d.Why)
				continue
			}
			t := now
			d.Outcome, d.RepliedAt, d.DelaySeconds = model.DecisionReplied, &t, now.Sub(d.At).Seconds()
			d.Why = "You approved the reply"
			d.Reply = guard.Truncate(text, logReplyMax)
			addStep(d, now, model.ActApprovalSent, text)
		}
		return all, changed
	})
}

func addStep(d *model.LogEntry, at time.Time, typ, text string) {
	if len(d.Steps) >= logStepsMax {
		return
	}
	d.Steps = append(d.Steps, model.LogStep{At: at, Type: typ, Text: guard.Truncate(text, 200)})
}
