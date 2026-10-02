package mission

import (
	"slices"
	"time"

	"whatsappdoppel/internal/model"
)

// An achievement is unlocked by the completed missions in the history; it is
// computed, never stored. UnlockedAt is when the record that crossed the
// target was reached.

type rule struct {
	id, title, blurb, badge string
	target                  int
	// count reports whether a reached record counts towards the target; nil = every record.
	count func(r model.MissionRecord, loc *time.Location) bool
	// distinctChats counts distinct chats instead of records.
	distinctChats bool
	// window: target records within this span (0 = all time).
	window time.Duration
}

var rules = []rule{
	{id: "first-mission", title: "First steps", blurb: "Complete your first mission.", badge: "first", target: 1},
	{id: "three-missions", title: "Hat trick", blurb: "Complete three missions.", badge: "hattrick", target: 3},
	{id: "word-smith", title: "Word whisperer", blurb: "Get someone to say the word, three times.", badge: "word", target: 3,
		count: func(r model.MissionRecord, _ *time.Location) bool {
			return r.TemplateID == "say-word" || r.How == model.GoalHowSaidWord
		}},
	{id: "social-butterfly", title: "Social butterfly", blurb: "Complete missions in three different chats.", badge: "butterfly", target: 3, distinctChats: true},
	{id: "smooth-operator", title: "Smooth operator", blurb: "Reach a goal with the subtle style — nobody suspected a thing.", badge: "mask", target: 1,
		count: func(r model.MissionRecord, _ *time.Location) bool { return r.Style == model.GoalStyleSubtle }},
	{id: "night-shift", title: "Night shift", blurb: "Complete a mission between midnight and 5 in the morning.", badge: "moon", target: 1,
		count: func(r model.MissionRecord, loc *time.Location) bool {
			h := r.ReachedAt.In(loc).Hour()
			return h < 5
		}},
	{id: "photo-finish", title: "Photo finish", blurb: "Get someone to send a photo or a voice note.", badge: "camera", target: 1,
		count: func(r model.MissionRecord, _ *time.Location) bool {
			return r.How == HowMedia || r.TemplateID == "get-photo" || r.TemplateID == "voice-note"
		}},
	{id: "planner", title: "Social planner", blurb: "Turn a chat into real plans: a meetup, a movie night or a yes to your idea.", badge: "calendar", target: 1,
		count: func(r model.MissionRecord, _ *time.Location) bool {
			t, _ := Template(r.TemplateID)
			return t.Category == CatPlans
		}},
	{id: "streak-week", title: "On a roll", blurb: "Complete three missions within seven days.", badge: "flame", target: 3, window: 7 * 24 * time.Hour},
	{id: "ten-missions", title: "Mission master", blurb: "Complete ten missions.", badge: "crown", target: 10},
}

// AchievementIDs lists every achievement id in display order.
func AchievementIDs() []string {
	out := make([]string, len(rules))
	for i, r := range rules {
		out[i] = r.id
	}
	return out
}

// Achievements computes every achievement from the mission history (any
// order). loc is the zone used for time-of-day rules (nil = Local).
func Achievements(records []model.MissionRecord, loc *time.Location) []model.Achievement {
	if loc == nil {
		loc = time.Local
	}
	var reached []model.MissionRecord
	for _, r := range records {
		if r.ReachedAt != nil {
			reached = append(reached, r)
		}
	}
	slices.SortStableFunc(reached, func(a, b model.MissionRecord) int { return a.ReachedAt.Compare(*b.ReachedAt) })

	out := make([]model.Achievement, 0, len(rules))
	for _, ru := range rules {
		a := model.Achievement{ID: ru.id, Title: ru.title, Blurb: ru.blurb, Badge: ru.badge, Target: ru.target}
		var hits []model.MissionRecord
		for _, r := range reached {
			if ru.count == nil || ru.count(r, loc) {
				hits = append(hits, r)
			}
		}
		switch {
		case ru.distinctChats:
			seen := map[string]bool{}
			for _, r := range hits {
				if seen[r.ChatKey] {
					continue
				}
				seen[r.ChatKey] = true
				if len(seen) == ru.target && a.UnlockedAt == nil {
					t := *r.ReachedAt
					a.UnlockedAt = &t
				}
			}
			a.Progress = len(seen)
		case ru.window > 0:
			best := 0
			for i := range hits {
				n := 0
				for j := i; j >= 0 && hits[i].ReachedAt.Sub(*hits[j].ReachedAt) <= ru.window; j-- {
					n++
				}
				best = max(best, n)
				if n >= ru.target && a.UnlockedAt == nil {
					t := *hits[i].ReachedAt
					a.UnlockedAt = &t
				}
			}
			a.Progress = best
		default:
			a.Progress = len(hits)
			if len(hits) >= ru.target {
				t := *hits[ru.target-1].ReachedAt
				a.UnlockedAt = &t
			}
		}
		a.Progress = min(a.Progress, a.Target)
		a.Unlocked = a.UnlockedAt != nil
		out = append(out, a)
	}
	return out
}
