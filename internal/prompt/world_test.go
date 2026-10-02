package prompt

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"whatsappdoppel/internal/model"
)

func tz(t *testing.T, name string) *time.Location {
	t.Helper()
	loc, err := time.LoadLocation(name)
	if err != nil {
		t.Skipf("no tzdata: %v", err)
	}
	return loc
}

// Thursday 1 October 2026, 17:11 UTC = 20:11 in Tel Aviv.
var worldNow = time.Date(2026, 10, 1, 17, 11, 0, 0, time.UTC)

func TestWorldSectionGolden(t *testing.T) {
	p := leo(t)
	p.World = model.World{City: "Tel Aviv", Country: "Israel", Timezone: "Asia/Jerusalem", Routine: []model.RoutineBlock{
		{Label: "dinner", From: "19:30", To: "21:00", Reach: model.ReachSlow},
	}}
	sys := SystemPrompt(p, model.ChatAssignment{}, false, "hey", Options{Now: worldNow, MacZone: tz(t, "Europe/Berlin")})
	golden(t, "leo_dm_world.golden", sys)
	for _, want := range []string{
		"RIGHT NOW: Thursday 1 October, 20:11 — evening in Tel Aviv, Israel (your local time). The weekend (Friday–Saturday here) starts tomorrow.",
		"You're in the middle of: dinner (19:30–21:00).",
		"probably on Europe/Berlin time, where it's 19:11.",
		"don't announce it",
	} {
		if !strings.Contains(sys, want) {
			t.Errorf("missing %q", want)
		}
	}
	// Zero Now: nothing added (existing goldens stay valid).
	if WorldSection(p.World, time.Time{}, nil) != "" {
		t.Error("zero Now must render nothing")
	}
}

func TestWorldSectionVariants(t *testing.T) {
	berlin := tz(t, "Europe/Berlin")
	// Same zone as the Mac: no "people are on … time" line; weekend today.
	sat := time.Date(2026, 10, 3, 9, 0, 0, 0, berlin)
	got := WorldSection(model.World{Timezone: "Europe/Berlin", Country: "Germany"}, sat, berlin)
	if !strings.Contains(got, "Saturday 3 October, 09:00 — morning in Germany") || !strings.Contains(got, "It's the weekend (Saturday–Sunday here).") || strings.Contains(got, "probably on") {
		t.Errorf("same zone:\n%s", got)
	}
	// Just finished a block; a different day for the people chatting.
	w := model.World{Timezone: "Asia/Tokyo", Routine: []model.RoutineBlock{{Label: "gym", From: "06:00", To: "07:00", Reach: model.ReachUnreachable}}}
	now := time.Date(2026, 10, 2, 7, 12, 0, 0, tz(t, "Asia/Tokyo")) // 00:12 Friday in Berlin
	got = WorldSection(w, now, tz(t, "America/New_York"))
	if !strings.Contains(got, "You just finished: gym (ended 12 minutes ago).") || !strings.Contains(got, "where it's 18:12 on Thursday") {
		t.Errorf("tokyo:\n%s", got)
	}
	// Upcoming block.
	now = time.Date(2026, 10, 2, 5, 30, 0, 0, tz(t, "Asia/Tokyo"))
	if got = WorldSection(w, now, nil); !strings.Contains(got, "Coming up: gym at 06:00.") || !strings.Contains(got, "late night") && !strings.Contains(got, "early morning") {
		t.Errorf("upcoming:\n%s", got)
	}
}

func TestLateSectionGolden(t *testing.T) {
	sys := SystemPrompt(leo(t), model.ChatAssignment{}, false, "you there?", Options{Late: &LateNote{Minutes: 50, Busy: "gym until 20:00"}})
	golden(t, "leo_dm_late.golden", sys)
	if !strings.Contains(sys, "about 50 minutes after their message — your routine had you busy (gym until 20:00)") {
		t.Error("late note")
	}
	if g := LateSection(&LateNote{Minutes: 130}); !strings.Contains(g, "about 2 hours") || !strings.Contains(g, "don't invent a specific reason") {
		t.Errorf("generic late = %q", g)
	}
	if LateSection(nil) != "" || LateSection(&LateNote{}) != "" {
		t.Error("nil late")
	}
}

func TestMemorySectionGolden(t *testing.T) {
	lines := []MemoryLine{{Person: "Dana", Text: "has an exam on Friday 3 Oct"}, {Person: "Dana", Text: "works night shifts\nas a nurse"}, {Text: "  "}}
	sys := SystemPrompt(leo(t), model.ChatAssignment{}, false, "hey", Options{Memories: lines})
	golden(t, "leo_dm_memories.golden", sys)
	if !strings.Contains(sys, "WHAT YOU REMEMBER ABOUT THEM") || !strings.Contains(sys, "- works night shifts as a nurse") || strings.Contains(sys, "Dana:") {
		t.Errorf("dm memories:\n%s", sys)
	}
	grp := MemorySection(lines, true)
	if !strings.Contains(grp, "ABOUT PEOPLE HERE") || !strings.Contains(grp, "- Dana: has an exam") {
		t.Errorf("group memories: %s", grp)
	}
	if MemorySection(nil, false) != "" {
		t.Error("empty")
	}
}

func TestExtractMemoriesRequest(t *testing.T) {
	msgs := []model.Message{
		{Speaker: "them", Name: "Dana", Text: "my exam is tomorrow"},
		{Speaker: "me", Text: "good luck! i'm a designer btw"},
		{Speaker: "me", Text: "*designer", Kind: model.MsgKindFix},
	}
	req := ExtractMemories("Leo", false, "Dana", []string{"Dana: works as a nurse"}, msgs, worldNow)
	if !req.JSON || req.Temperature != 0 || !json.Valid(req.Schema) {
		t.Fatalf("request = %+v", req)
	}
	body := req.Messages[0].Content
	for _, want := range []string{"Dana: my exam is tomorrow", "Leo (not noted): good luck", "- Dana: works as a nurse"} {
		if !strings.Contains(body, want) {
			t.Errorf("missing %q in\n%s", want, body)
		}
	}
	if strings.Contains(body, "*designer") {
		t.Error("fix bubbles must be skipped")
	}
	if !strings.Contains(req.System, "Today is Thursday 1 October 2026") || !strings.Contains(req.System, "Never note anything about Leo") {
		t.Errorf("system:\n%s", req.System)
	}
}

func TestParseMemories(t *testing.T) {
	cases := map[string]int{
		`{"memories":[{"person":"Dana","text":"works as a nurse","kind":"fact","expires":"","evidence":"i'm a nurse"}]}`: 1,
		"Sure! ```json\n{\"notes\":[{\"name\":\"Dana\",\"memory\":\"likes jazz\"}]}\n```":                                1,
		`[{"person":"Josh","text":"supports Arsenal"},{"person":"Josh","text":""}]`:                                      1,
		`{"memories":[]}`: 0,
	}
	for raw, n := range cases {
		got, err := ParseMemories(raw)
		if err != nil || len(got) != n {
			t.Errorf("ParseMemories(%q) = %+v, %v; want %d", raw, got, err, n)
		}
	}
	if _, err := ParseMemories("no json here"); err == nil {
		t.Error("garbage must fail")
	}
}

func TestRedactAndCloneBuilder(t *testing.T) {
	if got := RedactSample("call me 054-123-4567 or mail a.b@x.com, see https://x.com/y"); got != "call me [number] or mail [email], see [link]" {
		t.Errorf("redact = %q", got)
	}
	req := CloneBuilder("", []string{"old one", "hey!!", "HEY!!", "newest"}, "")
	body := req.Messages[0].Content
	if !strings.Contains(body, "Name: Me") || strings.Index(body, "newest") > strings.Index(body, "old one") || strings.Count(strings.ToLower(body), "hey!!") != 1 {
		t.Errorf("clone body:\n%s", body)
	}
	if !req.JSON || !json.Valid(req.Schema) {
		t.Error("clone request must be strict JSON")
	}
}
