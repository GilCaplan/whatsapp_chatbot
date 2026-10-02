// Package events is the in-process pub/sub that feeds the SSE stream
// (/api/events) and keeps a ring buffer of activity for /api/activity.
package events

import (
	"encoding/json"
	"os"
	"sync"
	"time"

	"whatsappdoppel/internal/model"
)

// SSE event types.
const (
	TypeWAStatus        = "wa.status"
	TypeWAQR            = "wa.qr"
	TypeActivity        = "activity"
	TypeApproval        = "approval" // data: {action:new|updated|removed, pending}
	TypeChatsChanged    = "chats.changed"
	TypePersonasChanged = "personas.changed"
	TypeSettingsChanged = "settings.changed"
	TypeOllamaPull      = "ollama.pull"
	TypeSystem          = "system" // data: {kind:"port_changed", url} | {kind:"quitting"}

	// Wave 3.
	TypeMemoriesChanged = "memories.changed" // data: {chatKey}
	TypeRecapsChanged   = "recaps.changed"   // data: {}
	TypeMissionsChanged = "missions.changed" // data: {}
)

type Event struct {
	ID   int64           `json:"id"`
	Type string          `json:"type"`
	Data json.RawMessage `json:"data"`
}

// ApprovalEvent is the payload of TypeApproval.
type ApprovalEvent struct {
	Action  string             `json:"action"` // new|updated|removed
	Pending model.PendingReply `json:"pending"`
}

const (
	replaySize   = 500
	activitySize = 1000
	subBuffer    = 256
)

type Hub struct {
	mu       sync.Mutex
	seq      int64
	replay   []Event // ring of recent events for Last-Event-ID
	activity []model.ActivityEvent
	actSeq   int64
	subs     map[chan Event]struct{}
	sink     func(model.ActivityEvent) // optional persistence (activity log file)
}

func NewHub() *Hub {
	return &Hub{subs: make(map[chan Event]struct{})}
}

// SetActivitySink installs fn to be called for every activity event (e.g. append to a log file).
func (h *Hub) SetActivitySink(fn func(model.ActivityEvent)) {
	h.mu.Lock()
	h.sink = fn
	h.mu.Unlock()
}

// Publish sends data (marshaled to JSON) to all subscribers.
// Events of type wa.qr are not kept for replay (they expire quickly).
func (h *Hub) Publish(typ string, data any) {
	raw, err := json.Marshal(data)
	if err != nil {
		return
	}
	h.mu.Lock()
	h.seq++
	ev := Event{ID: h.seq, Type: typ, Data: raw}
	if typ != TypeWAQR {
		h.replay = append(h.replay, ev)
		if len(h.replay) > replaySize {
			h.replay = h.replay[len(h.replay)-replaySize:]
		}
	}
	for ch := range h.subs {
		select {
		case ch <- ev:
		default: // slow subscriber: drop rather than block the publisher
		}
	}
	h.mu.Unlock()
}

// Activity records an activity event (assigning ID and timestamp) and publishes it.
func (h *Hub) Activity(a model.ActivityEvent) model.ActivityEvent {
	h.mu.Lock()
	h.actSeq++
	a.ID = h.actSeq
	if a.TS.IsZero() {
		a.TS = time.Now()
	}
	h.activity = append(h.activity, a)
	if len(h.activity) > activitySize {
		h.activity = h.activity[len(h.activity)-activitySize:]
	}
	sink := h.sink
	h.mu.Unlock()
	if sink != nil {
		sink(a)
	}
	h.Publish(TypeActivity, a)
	return a
}

// ActivitySince returns activity with ID > since, optionally filtered, newest last, capped at limit.
func (h *Hub) ActivitySince(since int64, chatKey string, types map[string]bool, limit int) ([]model.ActivityEvent, int64) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if limit <= 0 || limit > activitySize {
		limit = 200
	}
	out := make([]model.ActivityEvent, 0, limit)
	for _, a := range h.activity {
		if a.ID <= since {
			continue
		}
		if chatKey != "" && a.ChatKey != chatKey {
			continue
		}
		if len(types) > 0 && !types[a.Type] {
			continue
		}
		out = append(out, a)
	}
	if len(out) > limit {
		out = out[len(out)-limit:]
	}
	return out, h.actSeq
}

// ClearActivity empties the activity ring buffer.
func (h *Hub) ClearActivity() {
	h.mu.Lock()
	h.activity = nil
	h.mu.Unlock()
}

// Subscribe returns a channel of events plus the events after lastID still in the
// replay buffer (lastID <= 0 means no replay). Call cancel when done.
func (h *Hub) Subscribe(lastID int64) (ch <-chan Event, backlog []Event, cancel func()) {
	c := make(chan Event, subBuffer)
	h.mu.Lock()
	if lastID > 0 {
		for _, ev := range h.replay {
			if ev.ID > lastID {
				backlog = append(backlog, ev)
			}
		}
	}
	h.subs[c] = struct{}{}
	h.mu.Unlock()
	var once sync.Once
	return c, backlog, func() {
		once.Do(func() {
			h.mu.Lock()
			delete(h.subs, c)
			h.mu.Unlock()
		})
	}
}

// FileSink returns an activity sink that appends JSON lines to logs/activity-YYYY-MM-DD.jsonl.
func FileSink(logsDir string) func(model.ActivityEvent) {
	var mu sync.Mutex
	return func(a model.ActivityEvent) {
		b, err := json.Marshal(a)
		if err != nil {
			return
		}
		mu.Lock()
		defer mu.Unlock()
		name := logsDir + "/activity-" + a.TS.Format("2006-01-02") + ".jsonl"
		f, err := os.OpenFile(name, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
		if err != nil {
			return
		}
		f.Write(append(b, '\n'))
		f.Close()
	}
}
