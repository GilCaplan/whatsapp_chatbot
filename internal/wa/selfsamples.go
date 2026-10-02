package wa

import (
	"regexp"
	"slices"
	"strings"
	"sync"
	"unicode/utf8"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/proto/waHistorySync"
	"go.mau.fi/whatsmeow/types"

	"whatsappdoppel/internal/model"
)

// Samples of your own messages for "Clone yourself" (wave 3, Engineer A).
// Only with your consent (Options.CollectSelf): messages you type on your
// phone (live and from history sync) are redacted (links, e-mail addresses,
// phone numbers), capped and handed to Options.OnSelfMessage. Messages this
// app sends for a persona are never collected.

const (
	maxSampleRunes    = 300
	maxSamplesPerSync = 300 // newest per history-sync batch
	sentIDsKept       = 512
)

var (
	sampleURL   = regexp.MustCompile(`(?i)\b(?:https?://|www\.)\S+`)
	sampleEmail = regexp.MustCompile(`[\w.+-]+@[\w-]+\.[\w.]+`)
	samplePhone = regexp.MustCompile(`\+?\d[\d\s\-().]{6,}\d`)
)

var placeholders = strings.NewReplacer("[link]", "", "[email]", "", "[number]", "")

// RedactSample removes links, e-mail addresses and phone numbers.
func RedactSample(s string) string {
	s = sampleURL.ReplaceAllString(s, "[link]")
	s = sampleEmail.ReplaceAllString(s, "[email]")
	s = samplePhone.ReplaceAllString(s, "[number]")
	return strings.TrimSpace(s)
}

// selfSample turns one of your messages into a stored sample (false: not
// worth keeping — empty after redaction, or only a link/number).
func selfSample(in model.Incoming) (model.SelfSample, bool) {
	raw := in.Text
	if in.Media != "" {
		// A photo/video caption is how you text; a bare "[photo]" is not.
		_, caption, ok := strings.Cut(raw, "] ")
		if !ok {
			return model.SelfSample{}, false
		}
		raw = caption
	}
	text := RedactSample(raw)
	if strings.TrimSpace(placeholders.Replace(text)) == "" {
		return model.SelfSample{}, false
	}
	if utf8.RuneCountInString(text) > maxSampleRunes {
		text = strings.TrimSpace(string([]rune(text)[:maxSampleRunes]))
	}
	kind := "dm"
	if in.IsGroup {
		kind = "group"
	}
	return model.SelfSample{TS: in.Timestamp, Kind: kind, Text: text}, true
}

// sentIDs remembers the ids of messages this app sent, so their echoes are
// never mistaken for messages you typed.
type sentIDs struct {
	mu  sync.Mutex
	ids []string
}

func (s *sentIDs) add(id string) {
	if id == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ids = append(s.ids, id)
	if len(s.ids) > sentIDsKept {
		s.ids = slices.Clone(s.ids[len(s.ids)-sentIDsKept:])
	}
}

func (s *sentIDs) has(id string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return id != "" && slices.Contains(s.ids, id)
}

// collecting reports whether samples should be collected right now.
func (m *Manager) collecting() bool {
	return m.onSelf != nil && m.collectSelf != nil && m.collectSelf()
}

// sampleLive hands a live message you sent from your phone to OnSelfMessage.
func (m *Manager) sampleLive(in model.Incoming) {
	if !in.IsFromMe || !m.collecting() || m.sent.has(in.MessageID) {
		return
	}
	if s, ok := selfSample(in); ok {
		m.onSelf([]model.SelfSample{s})
	}
}

// sampleHistory collects your own messages from a history-sync batch
// (newest maxSamplesPerSync).
func (m *Manager) sampleHistory(cli *whatsmeow.Client, convs []*waHistorySync.Conversation, r resolver) {
	if !m.collecting() || cli == nil {
		return
	}
	var out []model.SelfSample
	for _, conv := range convs {
		chat, err := types.ParseJID(conv.GetID())
		if err != nil || chat.User == "" || skippedChat(chat) {
			continue
		}
		for _, hm := range conv.GetMessages() {
			wm := hm.GetMessage()
			if wm == nil || !wm.GetKey().GetFromMe() || m.sent.has(wm.GetKey().GetID()) {
				continue
			}
			evt, err := cli.ParseWebMessage(chat, wm)
			if err != nil {
				continue
			}
			in, ok := normalizeMessage(evt, r)
			if !ok || !in.IsFromMe {
				continue
			}
			if s, ok := selfSample(in); ok {
				out = append(out, s)
			}
		}
	}
	if len(out) == 0 {
		return
	}
	slices.SortStableFunc(out, func(a, b model.SelfSample) int { return a.TS.Compare(b.TS) })
	if len(out) > maxSamplesPerSync {
		out = out[len(out)-maxSamplesPerSync:]
	}
	m.onSelf(out)
}
