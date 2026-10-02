// Package mention handles @tags in group chats: a directory of the group's
// members with unique display names, validation of the "@Name" tags an LLM
// writes (Normalize), the wire encoding WhatsApp needs ("@<user>" plus the
// MentionedJID list, Encode) and the reverse for incoming messages
// (Humanize). It imports only model.
package mention

import (
	"slices"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"

	"whatsappdoppel/internal/model"
)

// MaxNameRunes caps a cleaned display name.
const MaxNameRunes = 32

// Entry is one taggable member.
type Entry struct {
	model.Participant
	Display string // unique label: "Dana" | "Dan K." | "Dan Katz" | "Dan Katz (…4567)"
}

// Policy limits the tags a reply may keep.
type Policy struct {
	Allow bool
	Max   int
}

// Directory is the set of taggable members of one group (never yourself).
type Directory struct {
	entries []Entry
	own     map[string]bool // user parts of your own JIDs
}

// NewDirectory builds a directory from a roster, skipping yourself and members
// without a usable name.
func NewDirectory(parts []model.Participant, ownJIDs []string) *Directory {
	d := &Directory{own: map[string]bool{}}
	for _, j := range ownJIDs {
		if u := User(j); u != "" {
			d.own[u] = true
		}
	}
	for _, p := range parts {
		if p.IsSelf || d.isOwn(p) {
			continue
		}
		name := CleanName(p.Name)
		if name == "" {
			continue
		}
		p.Name = name
		d.entries = append(d.entries, Entry{Participant: p})
	}
	d.rebuild()
	return d
}

// Len is the number of taggable members.
func (d *Directory) Len() int {
	if d == nil {
		return 0
	}
	return len(d.entries)
}

// Entries returns a copy of the members.
func (d *Directory) Entries() []Entry {
	if d == nil {
		return nil
	}
	return slices.Clone(d.entries)
}

func (d *Directory) isOwn(p model.Participant) bool {
	for _, j := range []string{p.JID, p.Phone, p.LID} {
		if u := User(j); u != "" && d.own[u] {
			return true
		}
	}
	return false
}

// AddSeen adds someone seen in the chat history who is not in the roster
// (e.g. the roster is unavailable, or they left). Known JIDs are ignored.
func (d *Directory) AddSeen(jid, pushName string) {
	u := User(jid)
	if d == nil || u == "" || d.own[u] {
		return
	}
	if _, ok := d.ByJID(jid); ok {
		return
	}
	name := CleanName(pushName)
	if name == "" {
		return
	}
	p := model.Participant{JID: jid, Name: name}
	if strings.HasSuffix(jid, "@lid") {
		p.LID = jid
	} else {
		p.Phone = u
	}
	d.entries = append(d.entries, Entry{Participant: p})
	d.rebuild()
}

// ByJID finds a member by the user part of its JID, phone or lid (device
// suffixes are ignored).
func (d *Directory) ByJID(jid string) (Entry, bool) {
	u := User(jid)
	if d == nil || u == "" {
		return Entry{}, false
	}
	for _, e := range d.entries {
		if User(e.JID) == u || e.Phone == u || User(e.LID) == u {
			return e, true
		}
	}
	return Entry{}, false
}

// IsOwn reports whether jid is one of your own addresses.
func (d *Directory) IsOwn(jid string) bool {
	return d != nil && d.own[User(jid)]
}

// Names lists display names for the prompt: the members whose JIDs appear in
// recentJIDs first (in that order), then everyone else alphabetically; at most
// max names (0 = all).
func (d *Directory) Names(recentJIDs []string, max int) []string {
	if d == nil {
		return nil
	}
	var out []string
	seen := map[string]bool{}
	for _, j := range recentJIDs {
		if e, ok := d.ByJID(j); ok && !seen[e.Display] {
			seen[e.Display] = true
			out = append(out, e.Display)
		}
	}
	var rest []string
	for _, e := range d.entries {
		if !seen[e.Display] {
			rest = append(rest, e.Display)
		}
	}
	sort.SliceStable(rest, func(i, j int) bool { return strings.ToLower(rest[i]) < strings.ToLower(rest[j]) })
	out = append(out, rest...)
	if max > 0 && len(out) > max {
		out = out[:max]
	}
	return out
}

// rebuild assigns unique display names: first name → "First L." → full
// name → full name + last four digits.
func (d *Directory) rebuild() {
	byFirst := map[string][]int{}
	for i := range d.entries {
		k := strings.ToLower(firstWord(d.entries[i].Name))
		byFirst[k] = append(byFirst[k], i)
	}
	for _, idx := range byFirst {
		if len(idx) == 1 {
			d.entries[idx[0]].Display = firstWord(d.entries[idx[0]].Name)
			continue
		}
		count := func(f func(Entry) string, v string) int {
			n := 0
			for _, i := range idx {
				if strings.EqualFold(f(d.entries[i]), v) {
					n++
				}
			}
			return n
		}
		short := func(e Entry) string { return firstInitial(e.Name) }
		full := func(e Entry) string { return e.Name }
		for _, i := range idx {
			e := d.entries[i]
			switch {
			case count(short, short(e)) == 1:
				d.entries[i].Display = short(e)
			case count(full, e.Name) == 1:
				d.entries[i].Display = e.Name
			default:
				d.entries[i].Display = e.Name + " (…" + lastDigits(e.Participant) + ")"
			}
		}
	}
	// Belt and braces: displays must be unique case-insensitively.
	seen := map[string]int{}
	for i := range d.entries {
		k := strings.ToLower(d.entries[i].Display)
		seen[k]++
		if seen[k] > 1 {
			d.entries[i].Display += " (…" + lastDigits(d.entries[i].Participant) + ")"
		}
	}
}

func firstWord(s string) string {
	if f := strings.Fields(s); len(f) > 0 {
		return f[0]
	}
	return s
}

// firstInitial renders "Dan Katz" as "Dan K." (single words stay as they are).
func firstInitial(s string) string {
	f := strings.Fields(s)
	if len(f) < 2 {
		return s
	}
	r, _ := utf8.DecodeRuneInString(f[len(f)-1])
	return f[0] + " " + string(r) + "."
}

func lastDigits(p model.Participant) string {
	s := p.Phone
	if s == "" {
		s = User(p.LID)
	}
	if s == "" {
		s = User(p.JID)
	}
	if len(s) > 4 {
		s = s[len(s)-4:]
	}
	return s
}

// User returns the user part of a JID ("972501234567:12@s.whatsapp.net" → "972501234567").
func User(jid string) string {
	u, _, _ := strings.Cut(jid, "@")
	u, _, _ = strings.Cut(u, ":")
	return strings.TrimSpace(u)
}

// CleanName makes an untrusted WhatsApp name safe for prompts and tags:
// control characters, newlines, "@" and commas are removed, whitespace is
// collapsed and the result capped at MaxNameRunes. Names that are phone
// numbers ("+1…" or only digits) become "".
func CleanName(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch {
		case r == '@' || r == ',' || r == '…':
			b.WriteRune(' ')
		case unicode.IsControl(r) || unicode.Is(unicode.Cf, r):
			b.WriteRune(' ')
		default:
			b.WriteRune(r)
		}
	}
	name := strings.Join(strings.Fields(b.String()), " ")
	if r := []rune(name); len(r) > MaxNameRunes {
		name = strings.TrimSpace(string(r[:MaxNameRunes]))
	}
	if name == "" || strings.HasPrefix(name, "+") {
		return ""
	}
	digits := true
	for _, r := range name {
		if !unicode.IsDigit(r) && r != ' ' && r != '-' {
			digits = false
			break
		}
	}
	if digits {
		return ""
	}
	return name
}

// ─── tokeniser ───────────────────────────────────────────────

func isWordRune(r rune) bool { return unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' }

// token is one "@…" occurrence in a text.
type token struct {
	start, end int    // byte offsets of the whole token incl. "@"
	entry      *Entry // roster match (nil when unknown)
	own        bool   // "@<your number>"
	word       string // the text after "@" for unknown tokens
}

// scan finds every "@" that starts a mention: at the start of the text or after
// a non-word rune. Roster names match case-insensitively, longest first, and
// must not be followed by a word rune; "@<digits>" matches by phone/lid.
func (d *Directory) scan(text string) []token {
	var out []token
	var names []*Entry
	if d != nil {
		for i := range d.entries {
			names = append(names, &d.entries[i])
		}
		sort.SliceStable(names, func(i, j int) bool {
			return utf8.RuneCountInString(names[i].Display) > utf8.RuneCountInString(names[j].Display)
		})
	}
	prev := rune(0)
	for i := 0; i < len(text); {
		r, size := utf8.DecodeRuneInString(text[i:])
		if r != '@' || (i > 0 && isWordRune(prev)) {
			prev = r
			i += size
			continue
		}
		rest := text[i+1:]
		tok := token{start: i}
		matched := false
		for _, e := range names {
			if n, ok := prefixFold(rest, e.Display); ok {
				if nr, _ := utf8.DecodeRuneInString(rest[n:]); n < len(rest) && isWordRune(nr) {
					continue
				}
				tok.end, tok.entry, matched = i+1+n, e, true
				break
			}
		}
		if !matched {
			n := 0
			for n < len(rest) && rest[n] >= '0' && rest[n] <= '9' {
				n++
			}
			if n >= 5 {
				digits := rest[:n]
				tok.end = i + 1 + n
				if d != nil && d.own[digits] {
					tok.own, matched = true, true
				} else if e, ok := d.ByJID(digits); ok {
					for _, x := range names {
						if x.JID == e.JID {
							tok.entry = x
						}
					}
					matched = true
				} else {
					tok.word, matched = digits, true
				}
			}
		}
		if !matched {
			n := 0
			for n < len(rest) {
				wr, ws := utf8.DecodeRuneInString(rest[n:])
				if !isWordRune(wr) {
					break
				}
				n += ws
			}
			if n == 0 {
				prev = r
				i += size
				continue
			}
			tok.end, tok.word = i+1+n, rest[:n]
		}
		out = append(out, tok)
		i = tok.end
		prev, _ = utf8.DecodeLastRuneInString(text[:tok.end])
	}
	return out
}

// prefixFold reports whether s starts with p (case-insensitive, rune-wise)
// and returns the byte length of the match in s.
func prefixFold(s, p string) (int, bool) {
	n := 0
	for _, pr := range p {
		if n >= len(s) {
			return 0, false
		}
		sr, size := utf8.DecodeRuneInString(s[n:])
		if sr != pr && !strings.EqualFold(string(sr), string(pr)) {
			return 0, false
		}
		n += size
	}
	return n, p != ""
}

// rewrite replaces each token with repl(tok) and tidies doubled spaces left
// by removed tokens.
func rewrite(text string, toks []token, repl func(token) string) string {
	if len(toks) == 0 {
		return text
	}
	var b strings.Builder
	last := 0
	removed := false
	for _, t := range toks {
		b.WriteString(text[last:t.start])
		s := repl(t)
		if s == "" {
			removed = true
		}
		b.WriteString(s)
		last = t.end
	}
	b.WriteString(text[last:])
	out := b.String()
	if removed {
		for strings.Contains(out, "  ") {
			out = strings.ReplaceAll(out, "  ", " ")
		}
		out = strings.TrimSpace(out)
		out = strings.ReplaceAll(out, " ,", ",")
	}
	return out
}

// ─── public operations ───────────────────────────────────────

// Normalize validates the "@Name" tags in an LLM reply: it keeps at most
// p.Max resolvable tags (first occurrence per person, canonical display
// name), turns later/excess tags and tags of unknown names into the plain
// name, and drops tags of yourself. With p.Allow false every tag becomes
// plain text.
func Normalize(text string, d *Directory, p Policy) (string, []Entry) {
	var tagged []Entry
	toks := d.scan(text)
	out := rewrite(text, toks, func(t token) string {
		switch {
		case t.own:
			return ""
		case t.entry == nil:
			return t.word
		}
		e := *t.entry
		dup := slices.ContainsFunc(tagged, func(x Entry) bool { return x.JID == e.JID })
		if p.Allow && !dup && len(tagged) < p.Max {
			tagged = append(tagged, e)
			return "@" + e.Display
		}
		return e.Display
	})
	return out, tagged
}

// Encode rewrites "@Display" (and "@<digits>") tags to the wire form
// "@<user>" and returns the JIDs for MentionedJID, deduplicated, in order.
func Encode(display string, d *Directory) (string, []string) {
	var jids []string
	toks := d.scan(display)
	wire := rewrite(display, toks, func(t token) string {
		if t.entry == nil {
			return display[t.start:t.end]
		}
		if !slices.Contains(jids, t.entry.JID) {
			jids = append(jids, t.entry.JID)
		}
		return "@" + User(t.entry.JID)
	})
	return wire, jids
}

// Humanize rewrites incoming "@<user>" tags whose JID is listed in mentioned
// to "@Display" (your own to "@"+selfName) and returns the display names
// tagged. Unknown numbers are left as they are.
func Humanize(text string, mentioned []string, d *Directory, ownJIDs []string, selfName string) (string, []string) {
	if len(mentioned) == 0 {
		return text, nil
	}
	users := map[string]string{} // user → full JID
	for _, m := range mentioned {
		if u := User(m); u != "" {
			users[u] = m
		}
	}
	own := map[string]bool{}
	for _, j := range ownJIDs {
		if u := User(j); u != "" {
			own[u] = true
		}
	}
	var names []string
	add := func(n string) {
		if !slices.Contains(names, n) {
			names = append(names, n)
		}
	}
	var out strings.Builder
	prev := rune(0)
	for i := 0; i < len(text); {
		r, size := utf8.DecodeRuneInString(text[i:])
		if r == '@' && !(i > 0 && isWordRune(prev)) {
			n := 0
			rest := text[i+1:]
			for n < len(rest) && rest[n] >= '0' && rest[n] <= '9' {
				n++
			}
			if u := rest[:n]; n > 0 && users[u] != "" {
				switch e, ok := d.ByJID(users[u]); {
				case own[u] && selfName != "":
					out.WriteString("@" + selfName)
					add(selfName)
				case ok:
					out.WriteString("@" + e.Display)
					add(e.Display)
				default:
					out.WriteString(text[i : i+1+n])
				}
				i += 1 + n
				prev = '0'
				continue
			}
		}
		out.WriteRune(r)
		prev = r
		i += size
	}
	return out.String(), names
}

// TagFirst addresses e at the start of text: a leading "Display," /
// "Display:" becomes "@Display", a leading "Display " gains the "@", anything
// else gets "@Display " in front. Nothing changes when e is already tagged.
func TagFirst(text string, e Entry, tagged []Entry) (string, bool) {
	if e.Display == "" || slices.ContainsFunc(tagged, func(x Entry) bool { return x.JID == e.JID }) {
		return text, false
	}
	text = strings.TrimSpace(text)
	if n, ok := prefixFold(text, e.Display); ok {
		rest := text[n:]
		r, size := utf8.DecodeRuneInString(rest)
		switch {
		case r == ',' || r == ':':
			return "@" + e.Display + " " + strings.TrimSpace(rest[size:]), true
		case unicode.IsSpace(r) || rest == "":
			return "@" + e.Display + rest, true
		}
	}
	return "@" + e.Display + " " + text, true
}

// Names returns the display names of entries.
func Names(es []Entry) []string {
	if len(es) == 0 {
		return nil
	}
	out := make([]string, len(es))
	for i, e := range es {
		out[i] = e.Display
	}
	return out
}

// JIDs returns the JIDs of entries.
func JIDs(es []Entry) []string {
	out := make([]string, len(es))
	for i, e := range es {
		out[i] = e.JID
	}
	return out
}
