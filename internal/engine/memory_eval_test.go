package engine

// Memory extraction eval against a real local Ollama: a few scripted chats
// with known facts. Checks that memories are about the OTHER people, durable
// and backed by what they wrote (the same prompt + checks the engine uses).
// Not part of the normal run:
//
//	DOPPEL_MEMORY_EVAL=1 go test ./internal/engine -run TestMemoryEval -v -timeout 20m
//
// Optional: MEMORY_EVAL_MODEL (default llama3.1:8b), MEMORY_EVAL_RUNS (default 2).

import (
	"context"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"whatsappdoppel/internal/llm"
	"whatsappdoppel/internal/memory"
	"whatsappdoppel/internal/model"
	"whatsappdoppel/internal/prompt"
)

type memEvalCase struct {
	id     string
	group  bool
	chat   string
	known  []string
	msgs   []model.Message
	expect [][]string // each: any of these words in some kept memory
	forbid []string   // none of these words in any kept memory
	max    int        // at most this many kept memories
}

func them(name, text string) model.Message {
	return model.Message{Speaker: "them", Name: name, Text: text, SenderJID: strings.ToLower(name) + "@s.whatsapp.net"}
}
func leoSays(text string) model.Message {
	return model.Message{Speaker: "me", Text: text, FromBot: true}
}

var memEvalCases = []memEvalCase{
	{
		id: "dm-dana", chat: "Dana",
		msgs: []model.Message{
			them("Dana", "heyy sorry was at work all day, the hospital was insane"),
			leoSays("darling you work too hard. I spent the day at a showroom in Milan"),
			them("Dana", "haha yeah night shifts again this week. my nursing exam is next thursday so im studying like crazy"),
			them("Dana", "also my sister noa is visiting from berlin this weekend"),
			leoSays("how lovely! is she the one who hates pineapple pizza?"),
			them("Dana", "lol no thats me. pineapple on pizza is a crime"),
			them("Dana", "im so tired rn"),
		},
		expect: [][]string{{"nurs", "hospital"}, {"exam"}, {"noa", "sister"}, {"pineapple"}},
		forbid: []string{"milan", "showroom", "tired"},
		max:    6,
	},
	{
		id: "group-brunch", group: true, chat: "Brunch crew",
		msgs: []model.Message{
			them("Maya", "morning people"),
			them("Josh", "ugh barely awake, our release is tomorrow"),
			them("Maya", "i think Josh needs some coffee"),
			leoSays("darling, coffee is my religion, I own three espresso machines"),
			them("Josh", "i only drink tea actually. green tea"),
			them("Maya", "btw my birthday party is on saturday at my place, you're all invited"),
			them("Josh", "will bring Bruno, he loves parties"),
			them("Josh", "my dog i mean lol"),
		},
		expect: [][]string{{"tea"}, {"birthday", "party"}, {"bruno", "dog"}},
		forbid: []string{"espresso", "religion", "awake", "three"},
		max:    6,
	},
	{
		id: "smalltalk", chat: "Alex",
		msgs: []model.Message{
			them("Alex", "hey"),
			leoSays("hello darling, how are you?"),
			them("Alex", "good good, you?"),
			leoSays("fabulous as always"),
			them("Alex", "lol nice"),
			them("Alex", "cool cool"),
		},
		max: 0,
	},
	{
		id: "hebrew", chat: "Noa",
		msgs: []model.Message{
			them("Noa", "היי! סליחה, הייתי בבית הספר כל היום"),
			them("Noa", "אני מורה לכיתה ג׳ בבית ספר יסודי"),
			leoSays("darling how sweet"),
			them("Noa", "ויש לי חתולה בשם מיצי שהורסת לי את הספה"),
		},
		expect: [][]string{{"teach", "מורה", "school", "בית ספר"}, {"cat", "חתול", "mitzi", "מיצי", "mitsi"}},
		max:    4,
	},
	{
		id: "known-dedupe", chat: "Dana",
		known: []string{"Dana: works night shifts as a nurse at Ichilov"},
		msgs: []model.Message{
			them("Dana", "back from another night shift at ichilov"),
			them("Dana", "being a nurse is exhausting"),
			leoSays("you poor thing"),
			them("Dana", "we're adopting a puppy next month!!"),
		},
		expect: [][]string{{"puppy", "dog"}},
		forbid: []string{"exhaust"},
		max:    2,
	},
}

func TestMemoryEval(t *testing.T) {
	if os.Getenv("DOPPEL_MEMORY_EVAL") == "" {
		t.Skip("set DOPPEL_MEMORY_EVAL=1 to run against a local Ollama")
	}
	mdl := os.Getenv("MEMORY_EVAL_MODEL")
	if mdl == "" {
		mdl = "llama3.1:8b"
	}
	runs := 2
	if v, err := strconv.Atoi(os.Getenv("MEMORY_EVAL_RUNS")); err == nil && v > 0 {
		runs = v
	}
	o := llm.NewOllama("http://127.0.0.1:11434", 8192, nil)
	h := newHarness(t, nil, dmChat(), groupChat())
	p, _ := h.st.Persona("leo")
	now := time.Date(2026, 10, 1, 18, 0, 0, 0, time.Local) // Thursday
	var expTotal, expHit, forbidHits, overMax, raw, kept, rejected int
	for _, tc := range memEvalCases {
		c := model.ChatAssignment{Key: "eval:" + tc.id, Kind: "dm", Name: tc.chat, JID: strings.ToLower(tc.chat) + "@s.whatsapp.net", PersonaID: "leo"}
		if tc.group {
			c.Kind = "group"
		}
		for run := 0; run < runs; run++ {
			req := prompt.ExtractMemories(p.Name, tc.group, tc.chat, tc.known, tc.msgs, now)
			req.Model, req.MaxTokens = mdl, memoryMaxTokens
			if run > 0 {
				req.Temperature = 0.3 // a second, slightly different sample
			}
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
			resp, err := o.Chat(ctx, req)
			cancel()
			if err != nil {
				t.Fatalf("%s: %v", tc.id, err)
			}
			found, err := prompt.ParseMemories(resp.Text)
			if err != nil {
				t.Errorf("%s: parse: %v\n%s", tc.id, err, resp.Text)
				continue
			}
			cands := h.e.checkMemories(c, p, tc.msgs, found)
			// The merge drops repeats of what is already known.
			var existing []model.Memory
			for _, k := range tc.known {
				who, text, _ := strings.Cut(k, ": ")
				existing = append(existing, model.Memory{ID: "k", Person: who, Text: text, Source: model.MemorySourceLearned, UpdatedAt: now.Add(-time.Hour)})
			}
			_, added, updated := memory.Merge(existing, cands, c.Key, now, 0)
			raw += len(found)
			kept += len(added) + len(updated)
			rejected += len(found) - len(cands)
			var texts []string
			for _, m := range append(added, updated...) {
				texts = append(texts, strings.ToLower(m.Person+": "+m.Text))
			}
			all := strings.Join(texts, " | ")
			for _, any := range tc.expect {
				expTotal++
				for _, w := range any {
					if strings.Contains(all, strings.ToLower(w)) {
						expHit++
						break
					}
				}
			}
			for _, f := range tc.forbid {
				if strings.Contains(all, f) {
					forbidHits++
					t.Logf("  FORBIDDEN %q in %s", f, tc.id)
				}
			}
			if len(added)+len(updated) > tc.max {
				overMax++
			}
			t.Logf("%s run %d: found %d, kept %d", tc.id, run+1, len(found), len(added)+len(updated))
			for _, f := range found {
				ok := false
				for _, cd := range cands {
					if strings.EqualFold(memory.CleanText(cd.Text), memory.CleanText(stripPersonPrefix(f.Text, cd.Person, f.Person))) {
						ok = true
					}
				}
				mark := "kept?"
				if !ok {
					mark = "REJECTED"
				}
				t.Logf("    %-8s %s: %q (kind %s, expires %q, evidence %q)", mark, f.Person, f.Text, f.Kind, f.Expires, f.Evidence)
			}
			for _, m := range append(added, updated...) {
				exp := ""
				if m.ExpiresAt != nil {
					exp = " until " + m.ExpiresAt.Format("Mon 2 Jan")
				}
				t.Logf("    => %s: %s [%s%s]", m.Person, m.Text, m.Kind, exp)
			}
		}
	}
	t.Logf("SUMMARY: expected facts found %d/%d, forbidden %d, over-max cases %d, raw %d → kept %d (rejected by checks %d)",
		expHit, expTotal, forbidHits, overMax, raw, kept, rejected)
}
