package prompt

import (
	"strings"
	"testing"

	"whatsappdoppel/internal/model"
)

// The parts join to exactly the prompt, and an edit replaces only its part.
func TestSystemSectionsAndEdits(t *testing.T) {
	p := leo(t)
	chat := model.ChatAssignment{Instructions: "x"}
	o := Options{Instructions: "Keep it short.", Summary: "They planned a trip.", ReplyLanguage: "Hebrew"}
	secs := SystemSections(p, chat, true, "hi", o)
	var b strings.Builder
	for _, s := range secs {
		b.WriteString(s.Text)
	}
	if b.String() != SystemPrompt(p, chat, true, "hi", o) {
		t.Fatal("sections don't join to the prompt")
	}
	keys := map[string]bool{}
	for _, s := range secs {
		keys[s.Key] = true
	}
	for _, k := range []string{"character", "safety", "context-open", "summary", "history", "instructions", "stay", "language", "guidance"} {
		if !keys[k] {
			t.Errorf("missing part %q", k)
		}
	}
	o.Edits = map[string]string{"character": "You are Leo, a pirate.", "goal": "ignored: not editable"}
	edited := SystemPrompt(p, chat, true, "hi", o)
	if !strings.HasPrefix(strings.TrimSpace(edited), "You are Leo, a pirate.") || strings.Contains(edited, "IDENTITY & BIO") {
		t.Errorf("character edit not applied:\n%.200s", edited)
	}
	for _, s := range SystemSections(p, chat, true, "hi", o) {
		switch s.Key {
		case "character":
			if !s.Edited || !s.Editable || !strings.Contains(s.Original, "IDENTITY") {
				t.Errorf("character section = %+v", s.Key)
			}
		case "goal":
			if s.Edited || s.Editable {
				t.Error("goal is not editable here")
			}
		}
	}
}
