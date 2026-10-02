package memory

import (
	"testing"
	"time"

	"whatsappdoppel/internal/model"
)

var now = time.Date(2026, 10, 1, 20, 0, 0, 0, time.UTC)

func mem(id, person, text string, age time.Duration) model.Memory {
	return model.Memory{ID: id, ChatKey: "c", Person: person, Text: text, Kind: "fact", Source: model.MemorySourceLearned, CreatedAt: now.Add(-age), UpdatedAt: now.Add(-age)}
}

func TestMergeDedupeUpdateAdd(t *testing.T) {
	existing := []model.Memory{
		mem("1", "Dana", "works as a nurse at Ichilov", time.Hour),
		mem("2", "Josh", "has a dog called Bruno", time.Hour),
	}
	found := []Candidate{
		{Person: "Dana", Text: "works as a nurse at Ichilov."},                // repeat → skipped
		{Person: "Dana", Text: "works as a nurse at Ichilov hospital nights"}, // same thing, changed → update
		{Person: "Josh", Text: "supports Arsenal", Kind: "preference"},        // new → add
		{Person: "Maya", Text: "has a dog called Bruno"},                      // other person → add
		{Person: "Josh", Text: "   "},                                         // empty → ignored
	}
	out, added, updated := Merge(existing, found, "c", now, 0)
	if len(out) != 4 || len(added) != 2 || len(updated) != 1 {
		t.Fatalf("out=%d added=%d updated=%d: %+v", len(out), len(added), len(updated), out)
	}
	if updated[0].ID != "1" || updated[0].Text != "works as a nurse at Ichilov hospital nights" || !updated[0].UpdatedAt.Equal(now) {
		t.Errorf("updated = %+v", updated[0])
	}
	if added[0].Source != model.MemorySourceLearned || added[0].Confidence != LearnedConf || added[0].ID == "" || added[0].Kind != "preference" {
		t.Errorf("added = %+v", added[0])
	}
}

func TestMergeKeepsUserAndPinnedOnCap(t *testing.T) {
	var existing []model.Memory
	pinned := mem("p", "Dana", "pinned thing", 900*time.Hour)
	pinned.Pinned = true
	user := mem("u", "Dana", "my own note", 800*time.Hour)
	user.Source = model.MemorySourceUser
	existing = append(existing, pinned, user)
	for i := 0; i < 5; i++ {
		existing = append(existing, mem(string(rune('a'+i)), "Josh", "fact number "+string(rune('a'+i))+" zz"+string(rune('a'+i)), time.Duration(100-i)*time.Hour))
	}
	out, added, _ := Merge(existing, []Candidate{{Person: "Maya", Text: "brand new thing"}}, "c", now, 5)
	if len(out) != 5 || len(added) != 1 {
		t.Fatalf("len=%d added=%d", len(out), len(added))
	}
	ids := map[string]bool{}
	for _, m := range out {
		ids[m.ID] = true
	}
	if !ids["p"] || !ids["u"] || ids["a"] || ids["b"] {
		t.Errorf("eviction kept %v", ids)
	}
	// Your own memory is never rewritten by the extractor.
	out2, _, upd := Merge([]model.Memory{user}, []Candidate{{Person: "Dana", Text: "my own note changed"}}, "c", now, 0)
	if len(upd) != 0 || out2[0].Text != "my own note" {
		t.Errorf("user memory rewritten: %+v", out2)
	}
}

func TestMergeExpiry(t *testing.T) {
	past := now.Add(-8 * 24 * time.Hour)
	soon := now.Add(-2 * 24 * time.Hour)
	ev1 := mem("old", "Dana", "exam on Friday", 10*24*time.Hour)
	ev1.ExpiresAt = &past
	ev2 := mem("recent", "Dana", "trip to Rome", 3*24*time.Hour)
	ev2.ExpiresAt = &soon
	out, _, _ := Merge([]model.Memory{ev1, ev2}, nil, "c", now, 0)
	if len(out) != 1 || out[0].ID != "recent" {
		t.Errorf("expiry: %+v", out)
	}
}

func TestSelect(t *testing.T) {
	in3 := now.Add(3 * 24 * time.Hour)
	ev := mem("ev", "Dana", "driving test on Sunday", 20*24*time.Hour)
	ev.ExpiresAt = &in3
	pin := mem("pin", "Josh", "allergic to nuts", 90*24*time.Hour)
	pin.Pinned = true
	mems := []model.Memory{
		mem("old", "Maya", "plays violin", 60*24*time.Hour),
		mem("topic", "Josh", "supports Arsenal football club", 30*24*time.Hour),
		ev, pin,
		mem("new", "Maya", "moved to Haifa", time.Hour),
	}
	got := Select(mems, []string{"did you watch the football last night?"}, now, 3)
	if len(got) != 3 || got[0].ID != "pin" {
		t.Fatalf("select = %+v", got)
	}
	ids := map[string]bool{got[1].ID: true, got[2].ID: true}
	if !ids["topic"] || !ids["ev"] {
		t.Errorf("want topic and event, got %v", ids)
	}
	if Select(mems, nil, now, 0) != nil {
		t.Error("k=0")
	}
}

func TestTokensJaccard(t *testing.T) {
	if j := Jaccard(Tokens("Has a dog called Bruno"), Tokens("has a dog named Bruno")); j < 0.5 || j > 0.8 {
		t.Errorf("jaccard = %v", j)
	}
	if len(Tokens("עובדת כאחות")) != 2 {
		t.Error("hebrew tokens")
	}
	if CleanText("  works\nas a   nurse. ") != "works as a nurse" {
		t.Error("clean text")
	}
}
