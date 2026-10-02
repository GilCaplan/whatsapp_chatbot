package prompt

// Impersonation eval: in a group, when the last turn is your own (trigger)
// message, does the persona write as another member ("Josh: haha …")?
// Not part of the normal run:
//
//	DOPPEL_IMPERSONATION_EVAL=1 go test ./internal/prompt -run TestImpersonationEval -v -timeout 30m
//
// Optional: IMP_EVAL_N (samples per variant, default 12), IMP_EVAL_MODEL
// (default llama3.1:8b), IMP_EVAL_VARIANTS (comma list).

import (
	"context"
	"os"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"whatsappdoppel/internal/llm"
	"whatsappdoppel/internal/model"
)

// legacyMessages is Messages before the fix: a trailing "me" turn is an
// unnamed user turn.
func legacyMessages(history []model.Message, isGroup bool) []llm.Message {
	out := make([]llm.Message, 0, len(history))
	for i, m := range history {
		role := llm.RoleUser
		if m.Speaker == "me" && i != len(history)-1 {
			role = llm.RoleAssistant
		}
		content := m.Text
		if isGroup && m.Speaker != "me" && m.Name != "" {
			content = m.Name + ": " + m.Text
		}
		out = append(out, llm.Message{Role: role, Content: content})
	}
	return out
}

func TestImpersonationEval(t *testing.T) {
	if os.Getenv("DOPPEL_IMPERSONATION_EVAL") == "" {
		t.Skip("set DOPPEL_IMPERSONATION_EVAL=1 to run against a local Ollama")
	}
	n := 12
	if v, err := strconv.Atoi(os.Getenv("IMP_EVAL_N")); err == nil && v > 0 {
		n = v
	}
	mdl := os.Getenv("IMP_EVAL_MODEL")
	if mdl == "" {
		mdl = "llama3.1:8b"
	}
	o := llm.NewOllama("http://127.0.0.1:11434", 8192, nil)
	p := leo(t)
	chat := model.ChatAssignment{Kind: "group", Name: "Brunch crew", GoalOverride: "Get Josh to say the word apple"}
	opts := Options{Participants: []string{"Josh", "Maya"}}
	if os.Getenv("IMP_EVAL_NOPLAN") == "" {
		// The plan-ahead note from the goal eval run that showed the bug.
		opts.Goal.Plan = "Ask Josh what he's drinking when he finally wakes up"
	}

	scenarios := map[string][]model.Message{
		"owner-last": {
			{Speaker: "them", Name: "Maya", Text: "morning people"},
			{Speaker: "them", Name: "Josh", Text: "ugh barely awake"},
			{Speaker: "me", Text: "i think Josh needs some coffee"},
		},
		"owner-last-after-reply": {
			{Speaker: "them", Name: "Maya", Text: "morning people"},
			{Speaker: "me", Text: "morning! who's alive", FromBot: true},
			{Speaker: "them", Name: "Josh", Text: "ugh barely awake"},
			{Speaker: "me", Text: "i think Josh needs some coffee"},
		},
		"owner-asks-josh": {
			{Speaker: "them", Name: "Maya", Text: "morning people"},
			{Speaker: "them", Name: "Josh", Text: "ugh barely awake"},
			{Speaker: "me", Text: "josh what are you even drinking rn"},
		},
		"member-asks-josh": {
			{Speaker: "them", Name: "Maya", Text: "morning people"},
			{Speaker: "them", Name: "Josh", Text: "ugh barely awake"},
			{Speaker: "them", Name: "Maya", Text: "josh what are you even drinking rn"},
		},
		"member-last": {
			{Speaker: "them", Name: "Maya", Text: "morning people"},
			{Speaker: "them", Name: "Josh", Text: "ugh barely awake"},
			{Speaker: "them", Name: "Maya", Text: "i think Josh needs some coffee"},
		},
	}
	variants := map[string]func([]model.Message) llm.Request{
		"before": func(h []model.Message) llm.Request {
			r := Compose(p, chat, h, true, opts)
			r.Messages = legacyMessages(h, true)
			return r
		},
		"after": func(h []model.Message) llm.Request { return Compose(p, chat, h, true, opts) },
		"cue-only": func(h []model.Message) llm.Request {
			r := Compose(p, chat, h, true, opts)
			r.Messages = legacyMessages(h, true)
			last := &r.Messages[len(r.Messages)-1]
			last.Content += "\n\n(Your turn, " + p.Name + ": write only your own next message, no name prefix.)"
			return r
		},
	}
	if v := os.Getenv("IMP_EVAL_VARIANTS"); v != "" {
		keep := map[string]bool{}
		for _, x := range strings.Split(v, ",") {
			keep[strings.TrimSpace(x)] = true
		}
		for k := range variants {
			if !keep[k] {
				delete(variants, k)
			}
		}
	}

	type job struct{ variant, scenario string }
	var jobs []job
	for v := range variants {
		for s := range scenarios {
			for i := 0; i < n; i++ {
				jobs = append(jobs, job{v, s})
			}
		}
	}
	type tally struct {
		total, prefixed, other int
		samples                []string
	}
	results := map[string]*tally{}
	var mu sync.Mutex
	sem := make(chan struct{}, 2) // parallelism ≤ 2
	var wg sync.WaitGroup
	for _, j := range jobs {
		wg.Add(1)
		sem <- struct{}{}
		go func(j job) {
			defer wg.Done()
			defer func() { <-sem }()
			h := scenarios[j.scenario]
			req := variants[j.variant](h)
			req.Model, req.MaxTokens, req.Temperature = mdl, 200, 0.8
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
			defer cancel()
			resp, err := o.Chat(ctx, req)
			if err != nil {
				t.Logf("error: %v", err)
				return
			}
			text := strings.TrimSpace(resp.Text)
			as := SpeaksAs(text, p.Name, h)
			// Also count replies that are clearly written as Josh without a prefix.
			low := strings.ToLower(text)
			other := as == "" && (strings.Contains(low, "i need coffee") || strings.Contains(low, "i do need") || strings.HasPrefix(low, "ugh") || strings.Contains(low, "my coffee"))
			key := j.variant + " / " + j.scenario
			mu.Lock()
			tl := results[key]
			if tl == nil {
				tl = &tally{}
				results[key] = tl
			}
			tl.total++
			if as != "" {
				tl.prefixed++
			}
			if other {
				tl.other++
			}
			if len(tl.samples) < 6 {
				tl.samples = append(tl.samples, text)
			}
			mu.Unlock()
		}(j)
	}
	wg.Wait()
	for k, tl := range results {
		t.Logf("%-40s  as-someone-else(prefix) %d/%d  written-as-Josh(heuristic) %d/%d", k, tl.prefixed, tl.total, tl.other, tl.total)
		for _, s := range tl.samples {
			t.Logf("    %q", s)
		}
	}
}
