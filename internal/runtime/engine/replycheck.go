package engine

import (
	"context"
	"strings"
	"time"
	"unicode"

	"whatsappdoppel/internal/core/model"
	"whatsappdoppel/internal/logic/lang"
	"whatsappdoppel/internal/runtime/prompt"
)

// The reply check (a context system's replyCheck): instant checks in code
// on every draft — its own instructions or plan copied into the message,
// talk about being an AI or about its agenda, an opening or a whole reply
// it used lately, the wrong language — and, if the system says so, a small
// AI judge (on borderline drafts or on all). A failed draft is written again
// with the reason (checkRetries times); if it still fails it stays quiet.

const judgeTimeout = 30 * time.Second

// replyCheck is what one chat's check needs.
type replyCheck struct {
	cp      model.ContextParams
	persona model.Persona
	rules   string // its instructions (the parts that aren't chat content)
	plan    string // the private next move ("" = none)
	lang    string // the reply language rule (English name, "" = none)
	hist    []model.Message
}

// checkRules collects the prompt parts a reply must never echo: safety,
// how to read the chat, people rules, goal, your instructions, stay
// yourself, language and guidance — and the persona's own guidelines.
func checkRules(sections []model.PromptSection, p model.Persona) string {
	var b strings.Builder
	for _, s := range sections {
		switch s.Key {
		case "safety", "secretary", "history", "people", "late", "goal", "instructions", "stay", "language", "guidance", "context-open", "context-close":
			b.WriteString(s.Text + "\n")
		}
	}
	b.WriteString(p.Rules)
	return b.String()
}

// metaPhrases only talk about the persona's own machinery; everyday phrases
// ("my goal is to run 5k", "note to self") are left to the other checks.
var metaPhrases = []string{"as an ai", "language model", "my agenda", "private agenda", "my secret goal", "next move:",
	"system prompt", "my instructions", "stay in character", "reply language", "guidance:", "chat context"}

// codeCheck runs the instant checks; soft are borderline signs for the judge.
func codeCheck(ck replyCheck, text string) (fails, soft []string) {
	words := normWords(text)
	low := strings.ToLower(text)
	for _, ph := range metaPhrases {
		if strings.Contains(low, ph) {
			fails = append(fails, "it talked about its instructions or agenda")
			break
		}
	}
	echo := max(3, ck.cp.EchoWords)
	if run := sharedRun(words, normWords(ck.rules)); run >= echo {
		fails = append(fails, "it copied its own instructions into the message")
	} else if run >= max(3, echo-2) {
		soft = append(soft, "close to its instructions")
	}
	if ck.plan != "" {
		pw := normWords(ck.plan)
		if run := sharedRun(words, pw); run >= echo || (len(pw) >= 5 && jaccard(words, pw) >= 0.6) {
			fails = append(fails, "it sent its private plan instead of a message")
		}
	}
	if ck.cp.RepeatWindow > 0 && len(words) >= 3 {
		n := 0
		for i := len(ck.hist) - 1; i >= 0 && n < ck.cp.RepeatWindow; i-- {
			m := ck.hist[i]
			if m.Speaker != "me" || !m.FromBot {
				continue
			}
			n++
			mw := normWords(m.Text)
			switch j := jaccard(words, mw); {
			case len(mw) >= 3 && j >= 0.7:
				fails = append(fails, "it nearly repeated one of its last replies")
			case len(mw) >= 3 && len(words) >= 3 && words[0] == mw[0] && words[1] == mw[1] && words[2] == mw[2]:
				fails = append(fails, "it started the same way as one of its last replies")
			case j >= 0.5:
				soft = append(soft, "similar to a recent reply")
			default:
				continue
			}
			break
		}
	}
	if ck.lang != "" && len(words) >= 4 {
		if code, sure := lang.Detect(text); sure && !strings.EqualFold(lang.Name(code), ck.lang) {
			fails = append(fails, "it wrote in "+lang.Name(code)+" instead of "+ck.lang)
		}
	}
	return fails, soft
}

// checkedReply checks a generated reply and, on a failure, writes it again
// with the reason; regen appends a note to the system prompt. A reply that
// keeps failing comes back with Quiet set (never the owner's fallback line).
func (e *Engine) checkedReply(ctx context.Context, ck replyCheck, res genResult, regen func(note string) (genResult, error)) genResult {
	if ck.cp.ReplyCheck == model.CheckOff || res.Fallback {
		return res
	}
	for attempt := 0; ; attempt++ {
		fails, soft := codeCheck(ck, res.Text)
		if len(fails) == 0 && (ck.cp.ReplyCheck == model.CheckJudge || (ck.cp.ReplyCheck == model.CheckCodeJudge && len(soft) > 0)) {
			res.Judged = true
			if ok, why := e.judge(ctx, ck, res.Text); !ok {
				fails = append(fails, "the check found it odd here"+whyText(why))
			}
		}
		if len(fails) == 0 {
			return res
		}
		res.Checks = append(res.Checks, fails...)
		if attempt >= ck.cp.CheckRetries || ctx.Err() != nil || regen == nil {
			res.Quiet = true
			return res
		}
		again, err := regen(prompt.CheckRetryNote(fails))
		if err != nil || again.Fallback {
			res.Quiet = true
			return res
		}
		again.Checks, again.Judged = res.Checks, again.Judged || res.Judged
		again.Latency += res.Latency
		again.CheckRewritten = true
		res = again
	}
}

func whyText(why string) string {
	if why == "" {
		return ""
	}
	return " (" + why + ")"
}

func (e *Engine) judge(ctx context.Context, ck replyCheck, text string) (bool, string) {
	prov, mdl, err := e.llm.Resolve(ck.persona.LLM)
	if err != nil {
		return true, ""
	}
	req := prompt.Judge(ck.persona.Name, ck.hist, text)
	req.Model = mdl
	cctx, cancel := context.WithTimeout(ctx, judgeTimeout)
	defer cancel()
	resp, err := prov.Chat(cctx, req)
	if err != nil {
		return true, "" // a broken judge never blocks a reply
	}
	return prompt.ParseJudge(resp.Text)
}

// normWords lowercases text into words (letters and digits only).
func normWords(s string) []string {
	return strings.FieldsFunc(strings.ToLower(s), func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) && r != '\'' })
}

// sharedRun is the longest run of words a has in a row somewhere in b.
func sharedRun(a, b []string) int {
	if len(a) == 0 || len(b) == 0 {
		return 0
	}
	at := map[string][]int{}
	for i, w := range b {
		at[w] = append(at[w], i)
	}
	best := 0
	for i := range a {
		for _, j := range at[a[i]] {
			n := 0
			for i+n < len(a) && j+n < len(b) && a[i+n] == b[j+n] {
				n++
			}
			best = max(best, n)
		}
	}
	return best
}

// jaccard is the share of distinct words two texts have in common.
func jaccard(a, b []string) float64 {
	if len(a) == 0 || len(b) == 0 {
		return 0
	}
	sa := map[string]bool{}
	for _, w := range a {
		sa[w] = true
	}
	sb := map[string]bool{}
	inter := 0
	for _, w := range b {
		if !sb[w] {
			sb[w] = true
			if sa[w] {
				inter++
			}
		}
	}
	return float64(inter) / float64(len(sa)+len(sb)-inter)
}
