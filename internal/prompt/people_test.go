package prompt

import (
	"fmt"
	"strings"
	"testing"

	"whatsappdoppel/internal/model"
)

func TestSystemPromptGroupParticipants(t *testing.T) {
	p := leo(t)
	opts := Options{
		LengthBias:    "normal",
		Participants:  []string{"Dana", "Noam", "Maya K.", "Maya L."},
		AllowMentions: true,
		MentionMax:    1,
		PeopleNotes: []PersonNote{
			{Name: "Noam", Notes: "my little brother,\ntease him about Arsenal"},
			{Name: "Dana", Notes: ""},
		},
	}
	sys := SystemPrompt(p, model.ChatAssignment{}, true, "hey", opts)
	golden(t, "leo_group_people.golden", sys)
	for _, want := range []string{
		GroupNote + "\n\nPEOPLE IN THIS GROUP: Dana, Noam, Maya K., Maya L.\nTagging: writing @ followed by a name exactly as listed (e.g. @Dana) pings that person.",
		"never tag everyone. At most one person per reply; most replies tag nobody.",
		"Don't tag the person you are simply replying to",
		"\n\nABOUT THE PEOPLE HERE:\n- Noam: my little brother, tease him about Arsenal",
	} {
		if !strings.Contains(sys, want) {
			t.Errorf("prompt missing %q", want)
		}
	}
	if strings.Contains(sys, "- Dana:") {
		t.Error("members without notes must not be listed")
	}

	opts.AllowMentions, opts.MentionMax = false, 3
	sys = SystemPrompt(p, model.ChatAssignment{}, true, "hey", opts)
	if !strings.Contains(sys, "Maya L.\nNever put @ in front of anyone's name.") || strings.Contains(sys, "Tagging:") {
		t.Errorf("allow=false: %q", sys)
	}
	opts.AllowMentions = true
	if sys = SystemPrompt(p, model.ChatAssignment{}, true, "hey", opts); !strings.Contains(sys, "At most 3 people per reply") {
		t.Error("mentionMax 3 not rendered")
	}

	var many []string
	for i := range 45 {
		many = append(many, fmt.Sprintf("P%02d", i))
	}
	sec := PeopleSection(Options{Participants: many, AllowMentions: true, MentionMax: 1}, true)
	if !strings.Contains(sec, "P39") || strings.Contains(sec, "P40") {
		t.Errorf("participants should be capped at %d: %q", MaxParticipants, sec)
	}

	// DMs: no member list, only the contact note.
	dm := SystemPrompt(p, model.ChatAssignment{}, false, "hey", Options{Participants: []string{"Dana"}, ContactNote: "my sister\u0007 — loves cats"})
	if strings.Contains(dm, "PEOPLE IN THIS GROUP") || !strings.Contains(dm, "\n\nABOUT THE PERSON YOU'RE TALKING TO: my sister — loves cats") {
		t.Errorf("dm: %q", dm)
	}
	if PeopleSection(Options{}, true) != "" || PeopleSection(Options{}, false) != "" {
		t.Error("empty options must add nothing")
	}
	long := PeopleSection(Options{PeopleNotes: []PersonNote{{Name: "X", Notes: strings.Repeat("a", 400)}}}, true)
	if !strings.HasSuffix(long, strings.Repeat("a", MaxNoteRunes)+"…") {
		t.Error("notes should be capped")
	}
}
