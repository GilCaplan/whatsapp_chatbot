package llm

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"time"

	"whatsappdoppel/internal/model"
)

// Fake is a scripted provider for tests and --fake-llm. Replies come from, in
// order: Func (if set), the queued replies, then a canned default. Every request
// is recorded.
type Fake struct {
	mu       sync.Mutex
	queue    []fakeReply
	fn       func(Request) (Response, error)
	requests []Request
	delay    time.Duration
	n        int
}

type fakeReply struct {
	text string
	err  error
}

func NewFake() *Fake { return &Fake{} }

func (f *Fake) Name() string { return "fake" }

// Push queues replies returned by the next Chat calls.
func (f *Fake) Push(replies ...string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, r := range replies {
		f.queue = append(f.queue, fakeReply{text: r})
	}
}

// PushError queues an error for the next Chat call.
func (f *Fake) PushError(err error) {
	f.mu.Lock()
	f.queue = append(f.queue, fakeReply{err: err})
	f.mu.Unlock()
}

// SetFunc makes fn answer every request (takes precedence over the queue).
func (f *Fake) SetFunc(fn func(Request) (Response, error)) {
	f.mu.Lock()
	f.fn = fn
	f.mu.Unlock()
}

// SetDelay simulates latency (honours ctx cancellation).
func (f *Fake) SetDelay(d time.Duration) {
	f.mu.Lock()
	f.delay = d
	f.mu.Unlock()
}

// Requests returns a copy of every request seen so far.
func (f *Fake) Requests() []Request {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]Request(nil), f.requests...)
}

// Reset clears recorded requests and queued replies.
func (f *Fake) Reset() {
	f.mu.Lock()
	f.requests, f.queue, f.n = nil, nil, 0
	f.mu.Unlock()
}

func (f *Fake) Chat(ctx context.Context, req Request) (Response, error) {
	f.mu.Lock()
	f.requests = append(f.requests, req)
	fn, delay := f.fn, f.delay
	var next *fakeReply
	if fn == nil && len(f.queue) > 0 {
		next = &f.queue[0]
		f.queue = f.queue[1:]
	}
	f.n++
	n := f.n
	f.mu.Unlock()

	if delay > 0 {
		select {
		case <-time.After(delay):
		case <-ctx.Done():
			return Response{}, ctx.Err()
		}
	}
	if err := ctx.Err(); err != nil {
		return Response{}, err
	}
	if fn != nil {
		return fn(req)
	}
	if next != nil {
		if next.err != nil {
			return Response{}, next.err
		}
		return Response{Text: next.text, Model: req.Model}, nil
	}
	return Response{Text: fakeDefault(req, n), Model: req.Model}, nil
}

var fakeLines = []string{
	"haha totally — tell me more",
	"wait, really? that's wild",
	"ok but how was the rest of your week?",
	"honestly same. what are you up to later?",
	"love that for you. details please",
}

func fakeDefault(req Request, n int) string {
	all := req.System
	for _, m := range req.Messages {
		all += "\n" + m.Content
	}
	switch {
	case strings.Contains(all, "YES or NO"):
		return "YES"
	case req.JSON && strings.Contains(req.System, "CO-PILOT"):
		// Co-pilot drafts (prompt.Drafts).
		b, _ := json.Marshal(map[string]any{"drafts": []map[string]string{
			{"tone": "brief", "text": "sounds good"},
			{"tone": "warm", "text": "aw I love that, tell me everything. how did it go?"},
			{"tone": "playful", "text": "ok but only if there are snacks involved"},
		}})
		return string(b)
	case req.JSON && strings.Contains(req.System, "needs a real person"):
		// Hand-off check (prompt.HandoffCheck): the fake never escalates weak hits.
		return `{"category":"none","serious":false}`
	case req.JSON && strings.Contains(req.System, "daily recaps"):
		// Daily recap (prompt.Recap).
		b, _ := json.Marshal(map[string]any{
			"headline": "A relaxed catch-up about the week and weekend plans", "topics": []string{"the week", "weekend plans"},
			"goalProgress": "", "toKnow": []string{"They asked about Saturday: nothing is agreed yet"}, "mood": "upbeat",
		})
		return string(b)
	case req.JSON && strings.Contains(req.System, "private notebook"):
		// Memory extraction (prompt.ExtractMemories): remember the last
		// thing someone else said, so the Memory tab has something to show.
		return fakeMemories(req)
	case req.JSON && strings.Contains(req.System, `"next"`):
		// Plan-ahead request (prompt.Plan).
		b, _ := json.Marshal(map[string]any{"situation": "Catching up", "achieved": false, "evidence": "", "next": "Ask a light follow-up question about their week"})
		return string(b)
	case req.JSON:
		b, _ := json.Marshal(map[string]any{
			"name":          "Sam",
			"tagline":       "Easygoing friend who always has a plan",
			"avatar":        map[string]any{"glyph": "wave"},
			"bio":           "Grew up by the sea, works in design, loves weekend trips.",
			"personality":   "Warm, curious and a little sarcastic.",
			"style":         "Casual, short messages, lowercase most of the time.",
			"vocabulary":    "\"honestly\", \"lowkey\", \"no way\"",
			"rules":         "- Ask follow-up questions.\n- Never lecture.",
			"language":      "English",
			"emoji":         map[string]any{"usage": "rare", "favorites": []string{"🙂", "🌊"}},
			"messageLength": "short",
			"goal":          "Catch up and see how their week is going.",
			"decisionHint":  "friendly, joins in when plans or jokes come up",
			"fallbackReply": "haha wait what? anyway, how are you?",
		})
		return string(b)
	}
	return fakeLines[(n-1)%len(fakeLines)]
}

func (f *Fake) ListModels(context.Context) ([]model.ModelInfo, error) {
	return []model.ModelInfo{{ID: "fake-model", Label: "Fake model (no LLM)"}}, nil
}

func (f *Fake) Ping(context.Context) error { return nil }

// fakeMemories answers a memory-extraction request with one memory about
// the last line someone other than the persona wrote ("(not noted)" marks
// the persona's own lines).
func fakeMemories(req Request) string {
	type mem struct {
		Person   string `json:"person"`
		Evidence string `json:"evidence"`
		Text     string `json:"text"`
		Kind     string `json:"kind"`
		Expires  string `json:"expires"`
	}
	out := struct {
		Memories []mem `json:"memories"`
	}{Memories: []mem{}}
	if len(req.Messages) > 0 {
		body := req.Messages[len(req.Messages)-1].Content
		if _, after, ok := strings.Cut(body, "New messages (oldest first):\n"); ok {
			lines := strings.Split(after, "\n")
			for i := len(lines) - 1; i >= 0; i-- {
				who, text, ok := strings.Cut(lines[i], ": ")
				if !ok || strings.Contains(who, "(not noted)") || len(strings.Fields(text)) < 3 {
					continue
				}
				words := strings.Fields(text)
				if len(words) > 8 {
					words = words[:8]
				}
				out.Memories = append(out.Memories, mem{Person: who, Evidence: text, Text: "mentioned " + strings.Join(words, " "), Kind: "fact"})
				break
			}
		}
	}
	b, _ := json.Marshal(out)
	return string(b)
}
