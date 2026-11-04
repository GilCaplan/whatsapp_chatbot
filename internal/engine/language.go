package engine

import (
	"regexp"
	"slices"

	"whatsappdoppel/internal/behavior"
	"whatsappdoppel/internal/lang"
	"whatsappdoppel/internal/model"
)

// Languages (internal/lang): a message clearly written in a language the
// chat doesn't speak is left alone (unless other languages are answered),
// and every reply gets a language — the person's choice, else the chat's,
// where "match" follows what they write (unclear messages follow the
// conversation).

// langConvMessages is how many recent messages set the conversation language.
const langConvMessages = 12

// tagWord matches an "@Name" tag, left out when detecting a language.
var tagWord = regexp.MustCompile(`@\S+`)

// languages is the chat's effective language preferences.
func (e *Engine) languages(c model.ChatAssignment) model.LanguagePrefs {
	return lang.Effective(e.cfg.Get().Behavior.LanguagesFor(behavior.KindOf(c)), c.Language)
}

// detectLang is lang.Detect without @tags (names say nothing about the language).
func detectLang(text string) (string, bool) {
	return lang.Detect(tagWord.ReplaceAllString(text, " "))
}

// foreignLanguage reports the language of text when the chat leaves
// messages in it alone ("" = answer it).
func (e *Engine) foreignLanguage(c model.ChatAssignment, text string) string {
	code, sure := detectLang(text)
	if !sure || !lang.Ignores(e.languages(c), code) {
		return ""
	}
	return code
}

// replyLanguage picks the language (code) of the next reply from the history:
// the message(s) it answers — theirs since its last message, or your own
// trigger message — and the conversation.
func (e *Engine) replyLanguage(c model.ChatAssignment, hist []model.Message) string {
	prefs := e.languages(c)
	var sender string
	var theirs, conv []string
	for i := len(hist) - 1; i >= 0; i-- {
		m := hist[i]
		if m.Speaker == "me" && m.FromBot {
			break // its own last message: what came after is what it answers
		}
		if sender == "" {
			sender = m.SenderJID
			if m.Speaker == "me" {
				sender = "me"
			}
		}
		if m.Speaker == "me" && sender == "me" || m.Speaker == "them" && m.SenderJID == sender {
			theirs = append(theirs, tagWord.ReplaceAllString(m.Text, " "))
		}
	}
	slices.Reverse(theirs) // oldest first: lang.Main gives ties to the latest text
	for _, m := range hist[max(0, len(hist)-langConvMessages):] {
		if m.Speaker == "them" || !m.FromBot {
			conv = append(conv, tagWord.ReplaceAllString(m.Text, " "))
		}
	}
	person := ""
	if sender != "" && sender != "me" {
		ids := []string{sender}
		if !isGroupChat(c) {
			ids = append(ids, c.JID, c.AltJID)
		}
		if i := behavior.FindPerson(c.People.People, ids...); i >= 0 {
			person = c.People.People[i].Language
		}
	} else if !isGroupChat(c) && len(c.People.People) > 0 {
		if i := behavior.FindPerson(c.People.People, c.JID, c.AltJID); i >= 0 {
			person = c.People.People[i].Language
		}
	}
	return lang.Reply(prefs, person, lang.Main(theirs), lang.Main(conv))
}
