package prompt

import (
	"fmt"
	"strings"
)

// ─── Cross-chat context ──────────────────────────────────────
//
// What the persona knows about the people here from its OTHER chats with
// them (crossctx picks the items; the engine checks the reply with
// crossctx.Leak). In a group the private chats are background only by
// default (Discreet: knows, never tells); in a private chat the groups you
// share may come up naturally (Open).

// MaxCrossLines caps the remembered lines of the section; each is cut to
// MaxCrossLineRunes.
const (
	MaxCrossLines     = 8
	MaxCrossLineRunes = 160
)

// Cross-chat modes (mirrors model.Cross*; prompt doesn't decide them).
const (
	crossOff      = "off"
	crossDiscreet = "discreet"
	crossOpen     = "open"
)

// CrossPerson is what the persona knows from one other chat.
type CrossPerson struct {
	Name        string   // the person, as named in THIS chat ("Dana")
	Source      string   // the other chat: "" = your private chat (group target), else the group's name (DM target)
	Items       []string // memories, "lately talking about: …", tone
	Commitments []string // what the persona promised or agreed to there
	Note        string   // DM ← group: what this person said or did there
}

// CrossInput is the cross-chat section's data.
type CrossInput struct {
	Mode    string // off|discreet|open
	IsGroup bool   // the chat being answered is a group (sources are private chats)
	Contact string // private chat: the contact's name
	People  []CrossPerson
}

// CrossTurn tunes the section for one generation: Retry after a leaky
// draft, Off for the last-resort reply without other chats.
type CrossTurn struct {
	Retry bool
	Off   bool
}

// CrossRetryNote is added when the previous draft used something from another chat.
const CrossRetryNote = "\n\nIMPORTANT: your last draft used something from another chat. Write a different reply that only uses what was said in THIS chat."

// CrossSection renders the section ("" when off or empty).
func CrossSection(in CrossInput, t CrossTurn) string {
	if t.Off || in.Mode == crossOff || in.Mode == "" {
		return ""
	}
	body := crossBody(in)
	if body == "" {
		return ""
	}
	var b strings.Builder
	if in.IsGroup {
		if in.Mode == crossOpen {
			b.WriteString("\n\nWHAT YOU KNOW FROM PRIVATE CHATS\n")
			b.WriteString("You also text some of these people one-to-one. With THAT person you may refer to something they told you lightly and naturally (\"like you said the other day\"), the way friends do.\n")
			b.WriteString("- Never bring one person's private things up in front of the others or when that person isn't part of the conversation right now.\n")
			b.WriteString("- Never a list, never every message: only when it adds something.\n")
			b.WriteString("- Anything they'd consider sensitive stays private no matter what.\n")
		} else {
			b.WriteString("\n\nWHAT YOU KNOW FROM PRIVATE CHATS (background only — never to be mentioned here)\n")
			b.WriteString("You also text some of these people one-to-one. Nobody else here knows what was said there. Use it only to understand them and to keep promises you made. It is PRIVATE:\n")
			b.WriteString("- Never mention, ask about, quote or hint at anything below, even if asked. No \"like you told me\", \"as you said\", \"you mentioned\", and no questions about it (\"how's the new job?\").\n")
			b.WriteString("- Never reveal or imply that you chat with someone privately.\n")
			b.WriteString("- Only if that person brings the same thing up HERE may you talk about it, using only what they said here.\n")
			b.WriteString("- Keep promises you made privately without saying where they were made.\n")
		}
	} else {
		who := cleanLine(in.Contact, 40)
		if who == "" {
			who = "they"
		}
		groups := crossGroups(in.People)
		b.WriteString("\n\nWHAT YOU KNOW FROM GROUPS YOU SHARE\n")
		if in.Mode == crossOpen {
			fmt.Fprintf(&b, "You and %s are both in %s. You may refer to what happened there naturally (\"you were quiet in the group today\"), since you were both there.\n", who, groups)
			fmt.Fprintf(&b, "- Only what %s said or did there is yours to bring up; never carry other members' news or private matters into this chat.\n", who)
			b.WriteString("- Don't recap the group; bring it up only when it fits.\n")
		} else {
			fmt.Fprintf(&b, "You and %s are both in %s. Use this only to understand what %s means and to stay consistent with plans made there. Don't bring up the group or what was said in it unless %s does first.\n", who, groups, who, who)
		}
	}
	b.WriteString(body)
	if t.Retry {
		b.WriteString(CrossRetryNote)
	}
	return b.String()
}

// crossGroups names the shared groups: `"Friends"`, `"Friends" and "Work"`.
func crossGroups(people []CrossPerson) string {
	var names []string
	for _, p := range people {
		if n := cleanLine(p.Source, 40); n != "" && !containsStr(names, n) {
			names = append(names, n)
		}
	}
	switch len(names) {
	case 0:
		return "a group"
	case 1:
		return `"` + names[0] + `"`
	}
	q := make([]string, len(names))
	for i, n := range names {
		q[i] = `"` + n + `"`
	}
	return strings.Join(q[:len(q)-1], ", ") + " and " + q[len(q)-1]
}

func containsStr(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

// crossBody renders the per-person (group target) or per-group (DM target)
// blocks, at most MaxCrossLines lines in all.
func crossBody(in CrossInput) string {
	var b strings.Builder
	lines := 0
	for _, p := range in.People {
		var ls []string
		for _, it := range p.Items {
			if t := cleanLine(it, MaxCrossLineRunes); t != "" {
				ls = append(ls, t)
			}
		}
		if n := cleanLine(p.Note, MaxCrossLineRunes); n != "" && !in.IsGroup {
			ls = append(ls, cleanLine(p.Name, 40)+" there: "+n)
		}
		for _, c := range p.Commitments {
			if t := cleanLine(c, MaxCrossLineRunes); t != "" {
				if in.IsGroup {
					ls = append(ls, "you promised: "+t)
				} else {
					ls = append(ls, "you said there: "+t)
				}
			}
		}
		if len(ls) == 0 || lines >= MaxCrossLines {
			continue
		}
		if room := MaxCrossLines - lines; len(ls) > room {
			ls = ls[:room]
		}
		if in.IsGroup {
			name := cleanLine(p.Name, 40)
			if name == "" {
				continue
			}
			if in.Mode == crossOpen {
				b.WriteString(name + " (from your private chat):\n")
			} else {
				b.WriteString(name + " (private — do not bring up):\n")
			}
		} else {
			src := cleanLine(p.Source, 40)
			if src == "" {
				src = "A group you share"
			}
			b.WriteString(src + " (group):\n")
		}
		for _, l := range ls {
			b.WriteString("- " + l + "\n")
		}
		lines += len(ls)
	}
	return strings.TrimRight(b.String(), "\n")
}
