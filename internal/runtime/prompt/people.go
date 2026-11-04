package prompt

import (
	"fmt"
	"strings"
	"unicode"
)

// MaxParticipants caps the names listed in PEOPLE IN THIS GROUP.
const MaxParticipants = 40

// MaxPeopleNotes caps the members listed in ABOUT THE PEOPLE HERE.
const MaxPeopleNotes = 15

// MaxNoteRunes caps one person's notes in the prompt.
const MaxNoteRunes = 300

// PersonNote is what the owner wrote about one person in the chat.
type PersonNote struct {
	Name  string
	Notes string
}

// PeopleSection renders the group member list with the tagging rule, the
// owner's notes about members (groups) or about the contact (DMs). Empty
// when there is nothing to say.
func PeopleSection(o Options, isGroup bool) string {
	var b strings.Builder
	if isGroup && len(o.Participants) > 0 {
		names := o.Participants
		if len(names) > MaxParticipants {
			names = names[:MaxParticipants]
		}
		b.WriteString("\n\nPEOPLE IN THIS GROUP: " + strings.Join(names, ", "))
		if o.AllowMentions {
			n := max(o.MentionMax, 1)
			per := "one person"
			if n > 1 {
				per = fmt.Sprintf("%d people", n)
			}
			b.WriteString(fmt.Sprintf("\nTagging: writing @ followed by a name exactly as listed (e.g. @%s) pings that person. Only tag when it matters who you mean: answering one of several people who asked you different things, or pulling in someone who isn't talking (like a person the others are asking about). Don't tag the person you are simply replying to, don't tag in general chatter, never tag everyone. At most %s per reply; most replies tag nobody.", names[0], per))
		} else {
			b.WriteString("\nNever put @ in front of anyone's name.")
		}
	}
	if isGroup {
		var lines []string
		for _, pn := range o.PeopleNotes {
			name, notes := cleanLine(pn.Name, 40), cleanLine(pn.Notes, MaxNoteRunes)
			if name == "" || notes == "" {
				continue
			}
			lines = append(lines, "- "+name+": "+notes)
			if len(lines) == MaxPeopleNotes {
				break
			}
		}
		if len(lines) > 0 {
			b.WriteString("\n\nABOUT THE PEOPLE HERE:\n" + strings.Join(lines, "\n"))
		}
	} else if notes := cleanLine(o.ContactNote, MaxNoteRunes); notes != "" {
		b.WriteString("\n\nABOUT THE PERSON YOU'RE TALKING TO: " + notes)
	}
	return b.String()
}

// cleanLine turns s into one line: control characters and newlines become
// spaces, whitespace collapses, and the result is capped at maxRunes.
func cleanLine(s string, maxRunes int) string {
	s = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) || unicode.Is(unicode.Cf, r) {
			return ' '
		}
		return r
	}, s)
	s = strings.Join(strings.Fields(s), " ")
	if r := []rune(s); len(r) > maxRunes {
		s = strings.TrimSpace(string(r[:maxRunes])) + "…"
	}
	return s
}
