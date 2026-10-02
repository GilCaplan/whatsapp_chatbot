package handoff

import (
	"testing"

	"whatsappdoppel/internal/model"
)

func TestDetectStrong(t *testing.T) {
	cases := []struct{ text, cat string }{
		{"are you a bot?", model.HandoffBot},
		{"wait r u an AI", model.HandoffBot},
		{"Are you even a real person??", model.HandoffBot},
		{"am I talking to a bot right now", model.HandoffBot},
		{"is this really you?", model.HandoffBot},
		{"you sound like chatgpt lol", model.HandoffBot},
		{"אתה בוט?", model.HandoffBot},
		{"רגע, זה באמת אתה?", model.HandoffBot},
		{"can you send me money? like 200 bucks", model.HandoffMoney},
		{"send me $50 pls", model.HandoffMoney},
		{"could you lend me some until friday", model.HandoffMoney},
		{"what's your IBAN", model.HandoffMoney},
		{"תעביר לי 300 שקל בביט", model.HandoffMoney},
		{"I'm in hospital", model.HandoffHealth},
		{"we called an ambulance", model.HandoffHealth},
		{"אני בבית חולים", model.HandoffHealth},
		{"הוא אושפז אתמול", model.HandoffHealth},
		{"let's meet tomorrow at 6", model.HandoffMeeting},
		{"wanna grab a coffee this week?", model.HandoffMeeting},
		{"send me your location", model.HandoffMeeting},
		{"בוא ניפגש מחר", model.HandoffMeeting},
		{"I want to kill myself", model.HandoffDistress},
		{"honestly I don't want to live anymore", model.HandoffDistress},
		{"אני לא רוצה לחיות", model.HandoffDistress},
		{"my lawyer will call you", model.HandoffLegal},
		{"I'm calling the police", model.HandoffLegal},
		{"אני הולך לעורך דין", model.HandoffLegal},
	}
	for _, c := range cases {
		hit, ok := Detect(c.text, All())
		if !ok || hit.Strength != Strong || hit.Category != c.cat {
			t.Errorf("%q = %+v %v, want strong %s", c.text, hit, ok, c.cat)
		}
	}
}

func TestDetectWeak(t *testing.T) {
	cases := []struct{ text, cat string }{
		{"lol you're such a bot", model.HandoffBot},
		{"I still owe you for the pizza", model.HandoffMoney},
		{"been sick all week", model.HandoffHealth},
		{"meet you at the gym lol", model.HandoffMeeting},
		{"feeling kinda sad today", model.HandoffDistress},
		{"I signed the contract", model.HandoffLegal},
		{"אני חולה", model.HandoffHealth},
		{"אני בוכה מצחוק", model.HandoffDistress},
	}
	for _, c := range cases {
		hit, ok := Detect(c.text, All())
		if !ok || hit.Strength != Weak || hit.Category != c.cat {
			t.Errorf("%q = %+v %v, want weak %s", c.text, hit, ok, c.cat)
		}
	}
}

func TestDetectNone(t *testing.T) {
	for _, text := range []string{
		"this movie killed me",
		"grab me a bottle of water",
		"the robotics club was fun", // not "robot"
		"I'm dead lol",
		"what a beautiful day",
		"aim higher",
		"a bit tired but good",
		"haha same",
		"מה נשמע?",
		"ביטוח רכב זה יקר", // "ביט" inside a word
		"",
	} {
		if hit, ok := Detect(text, All()); ok {
			t.Errorf("%q should not hit, got %+v", text, hit)
		}
	}
}

func TestDetectCategoriesAndPriority(t *testing.T) {
	// Switched-off categories are ignored.
	cats := All()
	cats[model.HandoffBot] = false
	if hit, ok := Detect("are you a bot?", cats); ok {
		t.Errorf("bot off: %+v", hit)
	}
	if _, ok := Detect("are you a bot?", Categories{}); ok {
		t.Error("no categories should never hit")
	}
	// A strong hit beats an earlier weak one; distress beats money.
	hit, _ := Detect("I'm so sad, can you lend me money", All())
	if hit.Category != model.HandoffMoney || hit.Strength != Strong {
		t.Errorf("strong wins: %+v", hit)
	}
	hit, _ = Detect("I owe money and I want to kill myself", All())
	if hit.Category != model.HandoffDistress {
		t.Errorf("distress first: %+v", hit)
	}
	if got := cats.List(); len(got) != 5 || got[0] != model.HandoffMoney {
		t.Errorf("List = %v", got)
	}
}

func TestNormalize(t *testing.T) {
	if got := Normalize("  Don’t   WANT\nto live "); got != "don't want to live" {
		t.Errorf("Normalize = %q", got)
	}
	if hit, ok := Detect("I DON’T want to live", All()); !ok || hit.Category != model.HandoffDistress {
		t.Errorf("curly apostrophe: %+v %v", hit, ok)
	}
}

func TestReasonAndLabel(t *testing.T) {
	for _, c := range model.HandoffCategories {
		if Label(c) == c || Reason(c, "Dana") == "" {
			t.Errorf("missing copy for %s", c)
		}
	}
	if got := Reason(model.HandoffBot, "Dana"); got != "Dana asked if they're talking to a bot" {
		t.Errorf("Reason = %q", got)
	}
}
