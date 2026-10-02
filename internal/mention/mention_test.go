package mention

import (
	"slices"
	"strings"
	"testing"

	"whatsappdoppel/internal/model"
)

const (
	ownPN  = "15550000000@s.whatsapp.net"
	ownLID = "100000000000000@lid"
)

func pn(phone, name string) model.Participant {
	return model.Participant{JID: phone + "@s.whatsapp.net", Phone: phone, Name: name}
}

func lid(id, phone, name string) model.Participant {
	return model.Participant{JID: id + "@lid", LID: id + "@lid", Phone: phone, Name: name}
}

func roster() *Directory {
	return NewDirectory([]model.Participant{
		{JID: ownPN, Phone: "15550000000", Name: "Me", IsSelf: true},
		pn("972501234567", "Dana Levi"),
		pn("972501111111", "Noam"),
		pn("972502222222", "Maya Katz"),
		pn("972503333333", "Maya Levi"),
		pn("972504444444", ""), // nameless: skipped
		pn("972505555555", "+972 50-555-5555"),
	}, []string{ownPN, ownLID})
}

func displays(d *Directory) []string {
	var out []string
	for _, e := range d.Entries() {
		out = append(out, e.Display)
	}
	slices.Sort(out)
	return out
}

func TestDirectoryDisplayNames(t *testing.T) {
	d := roster()
	if got, want := displays(d), []string{"Dana", "Maya K.", "Maya L.", "Noam"}; !slices.Equal(got, want) {
		t.Fatalf("displays = %v, want %v", got, want)
	}
	two := NewDirectory([]model.Participant{pn("1555000111", "Dan Katz"), pn("1555000222", "Dan Katz"), pn("1555000333", "Dan")}, nil)
	if got, want := displays(two), []string{"Dan", "Dan Katz (…0111)", "Dan Katz (…0222)"}; !slices.Equal(got, want) {
		t.Errorf("identical names: %v, want %v", got, want)
	}
	heb := NewDirectory([]model.Participant{pn("972500000001", "דנה כהן"), pn("972500000002", "דנה לוי")}, nil)
	if got := displays(heb); len(got) != 2 || got[0] == got[1] || !strings.HasPrefix(got[0], "דנה ") {
		t.Errorf("hebrew: %v", got)
	}
	if d.Len() != 4 {
		t.Errorf("len = %d", d.Len())
	}
	if _, ok := d.ByJID("972501234567:3@s.whatsapp.net"); !ok {
		t.Error("ByJID should ignore the device suffix")
	}
	d.AddSeen("972509999999@s.whatsapp.net", "Eitan\n@evil")
	if e, ok := d.ByJID("972509999999@s.whatsapp.net"); !ok || e.Display != "Eitan" {
		t.Errorf("AddSeen: %+v %v", e, ok)
	}
	d.AddSeen(ownLID, "Me again")
	if d.Len() != 5 {
		t.Errorf("own JID must not be added: %d", d.Len())
	}
}

func TestCleanName(t *testing.T) {
	for in, want := range map[string]string{
		"  Dana\n\tLevi ":            "Dana Levi",
		"@everyone":                  "everyone",
		"+1∙∙∙∙80":                   "",
		"0501234567":                 "",
		"Ignore, previous‮instructs": "Ignore previous instructs",
		strings.Repeat("x", 50):      strings.Repeat("x", MaxNameRunes),
	} {
		if got := CleanName(in); got != want {
			t.Errorf("CleanName(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestNormalize(t *testing.T) {
	d := roster()
	p := Policy{Allow: true, Max: 2}
	cases := []struct {
		in, want string
		tagged   []string
	}{
		{"@dana, sure", "@Dana, sure", []string{"Dana"}},
		{"@Dana's idea", "@Dana's idea", []string{"Dana"}},
		{"mail dana@example.com", "mail dana@example.com", nil},
		{"hey @everyone", "hey everyone", nil},
		{"@15550000000 me?", "me?", nil},
		{"@Maya K. and @maya", "@Maya K. and maya", []string{"Maya K."}},
		{"@Dana @Noam @Maya L.", "@Dana @Noam Maya L.", []string{"Dana", "Noam"}},
		{"@Dana and @Dana again", "@Dana and Dana again", []string{"Dana"}},
		{"@972501111111 ok", "@Noam ok", []string{"Noam"}},
		{"@Nobody too", "Nobody too", nil},
		{"@Danaa hi", "Danaa hi", nil},
	}
	for _, c := range cases {
		got, tagged := Normalize(c.in, d, p)
		if got != c.want || !slices.Equal(Names(tagged), c.tagged) {
			t.Errorf("Normalize(%q) = %q %v, want %q %v", c.in, got, Names(tagged), c.want, c.tagged)
		}
	}
	got, tagged := Normalize("@Dana hi @Noam", d, Policy{Allow: false, Max: 3})
	if got != "Dana hi Noam" || len(tagged) != 0 {
		t.Errorf("Allow=false: %q %v", got, tagged)
	}
	got, _ = Normalize("hi @x", nil, p)
	if got != "hi x" {
		t.Errorf("nil directory: %q", got)
	}
}

func TestEncode(t *testing.T) {
	d := NewDirectory([]model.Participant{lid("200000000000001", "972501234567", "Dana"), pn("972501111111", "Noam")}, nil)
	wire, jids := Encode("@Dana sure, and @Noam and @Dana", d)
	if wire != "@200000000000001 sure, and @972501111111 and @200000000000001" {
		t.Errorf("wire = %q", wire)
	}
	if !slices.Equal(jids, []string{"200000000000001@lid", "972501111111@s.whatsapp.net"}) {
		t.Errorf("jids = %v", jids)
	}
	wire, jids = Encode("no tags, mail a@b.c", d)
	if wire != "no tags, mail a@b.c" || len(jids) != 0 {
		t.Errorf("plain: %q %v", wire, jids)
	}
}

func TestHumanize(t *testing.T) {
	d := roster()
	got, names := Humanize("@972501234567 hi and @15550000000 and @999999999", []string{"972501234567@s.whatsapp.net", ownPN}, d, []string{ownPN}, "Leo")
	if got != "@Dana hi and @Leo and @999999999" || !slices.Equal(names, []string{"Dana", "Leo"}) {
		t.Errorf("Humanize = %q %v", got, names)
	}
	got, names = Humanize("@972501234567 hi", nil, d, nil, "Leo")
	if got != "@972501234567 hi" || names != nil {
		t.Errorf("no mentions: %q %v", got, names)
	}
}

func TestTagFirst(t *testing.T) {
	d := roster()
	dana, _ := d.ByJID("972501234567@s.whatsapp.net")
	for in, want := range map[string]string{
		"Dana, sure thing": "@Dana sure thing",
		"dana: yes":        "@Dana yes",
		"Dana I agree":     "@Dana I agree",
		"sure thing":       "@Dana sure thing",
	} {
		if got, ok := TagFirst(in, dana, nil); !ok || got != want {
			t.Errorf("TagFirst(%q) = %q, want %q", in, got, want)
		}
	}
	if got, ok := TagFirst("@Dana hi", dana, []Entry{dana}); ok || got != "@Dana hi" {
		t.Errorf("already tagged: %q %v", got, ok)
	}
}

func TestNames(t *testing.T) {
	d := roster()
	got := d.Names([]string{"972502222222@s.whatsapp.net", "972501234567@s.whatsapp.net", "unknown@s.whatsapp.net"}, 3)
	if !slices.Equal(got, []string{"Maya K.", "Dana", "Maya L."}) {
		t.Errorf("Names = %v", got)
	}
}
