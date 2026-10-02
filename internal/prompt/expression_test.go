package prompt

import (
	"strings"
	"testing"

	"whatsappdoppel/internal/model"
)

// fixedRoll is a deterministic Roller: Hit returns hits for 1..99 percent.
type fixedRoll struct {
	hits bool
	idx  int
}

func (f fixedRoll) Hit(p int) bool {
	switch {
	case p <= 0:
		return false
	case p >= 100:
		return true
	}
	return f.hits
}
func (f fixedRoll) Index(n int) int { return min(f.idx, max(n-1, 0)) }

func TestEmojiDetection(t *testing.T) {
	cases := map[string][]string{
		"hi 😂":                  {"😂"},
		"coding 👩🏽‍💻 all day":   {"👩🏽‍💻"},
		"pride 🏳️‍🌈":            {"🏳️‍🌈"},
		"🇮🇱🇺🇸":                  {"🇮🇱", "🇺🇸"},
		"love ❤️‍🔥 it":          {"❤️‍🔥"},
		"pick 1️⃣ or #️⃣":       {"1️⃣", "#️⃣"},
		"thumbs 👍🏻👍🏿":           {"👍🏻", "👍🏿"},
		"scotland 🏴󠁧󠁢󠁳󠁣󠁴󠁿":      {"🏴󠁧󠁢󠁳󠁣󠁴󠁿"},
		"❤ bare heart":          {"❤"},
		"sparkle ✨ sofa 🛋️":     {"✨", "🛋️"},
		"©️ emoji, © text, ™":   {"©️"},
		"שלום חבר, מה קורה?":    nil,
		"✓ done ★ 1 # 2 * 3 ->": nil,
		"family 👨‍👩‍👧‍👦!":       {"👨‍👩‍👧‍👦"},
	}
	for in, want := range cases {
		got := Emojis(in)
		if strings.Join(got, "|") != strings.Join(want, "|") {
			t.Errorf("Emojis(%q) = %q, want %q", in, got, want)
		}
		if CountEmoji(in) != len(want) {
			t.Errorf("CountEmoji(%q) = %d", in, CountEmoji(in))
		}
	}
}

func TestStripEmoji(t *testing.T) {
	cases := map[string]string{
		"so happy 🎉!":              "so happy!",
		"🎉 congrats darling":       "congrats darling",
		"a 😂 b":                    "a b",
		"a😂b":                      "ab",
		"end 🔥":                    "end",
		"שלום 😂 חבר, מה נשמע?":     "שלום חבר, מה נשמע?",
		"line one 😂\n🎉🎉\nline two": "line one\nline two",
		"👩🏽‍💻 coding, 🏳️‍🌈 pride 🇮🇱.":         "coding, pride.",
		"double  space stays 🙂":               "double  space stays",
		"no emoji at all,  untouched  ":       "no emoji at all,  untouched  ",
		"wow 😂😂😂 ok":                          "wow ok",
		"see https://x.com/a?b=1 😎 @Dana Lee": "see https://x.com/a?b=1 @Dana Lee",
		"😂":                                   "",
	}
	for in, want := range cases {
		if got := StripEmoji(in); got != want {
			t.Errorf("StripEmoji(%q) = %q, want %q", in, got, want)
		}
	}
	if got := KeepEmoji("a 😂 b 🎉 c 🔥", 1); got != "a 😂 b c" {
		t.Errorf("KeepEmoji 1 = %q", got)
	}
	if got := KeepEmoji("a 😂 b 🎉 c 🔥", 2); got != "a 😂 b 🎉 c" {
		t.Errorf("KeepEmoji 2 = %q", got)
	}
}

func TestEnforceEmojiLevels(t *testing.T) {
	favs := []string{"💅", "✨", "🍸"}
	prefs := func(u string) model.EmojiPrefs { return model.EmojiPrefs{Usage: u, Favorites: favs} }
	text := "omg congrats 🎉 so proud 🥂 drinks on me 🍸"
	cases := []struct {
		name   string
		usage  string
		tone   Tone
		mine   []string
		roll   fixedRoll
		text   string
		want   string
		remove int
	}{
		{"none strips", "none", ToneGood, nil, fixedRoll{hits: true}, text, "omg congrats so proud drinks on me", 3},
		{"rare keeps one when the roll hits", "rare", ToneGood, nil, fixedRoll{hits: true}, text, "omg congrats 🎉 so proud drinks on me", 2},
		{"rare keeps its one emoji without a roll", "rare", ToneGood, nil, fixedRoll{}, text, "omg congrats 🎉 so proud drinks on me", 2},
		{"rare drops it after a recent emoji", "rare", ToneGood, []string{"x 💅", "y", "z"}, fixedRoll{hits: true}, text, "omg congrats so proud drinks on me", 3},
		{"rare gap is only the last 3", "rare", ToneGood, []string{"x 💅", "a", "b", "c"}, fixedRoll{hits: true}, text, "omg congrats 🎉 so proud drinks on me", 2},
		{"some keeps two", "some", ToneGood, nil, fixedRoll{}, text, "omg congrats 🎉 so proud 🥂 drinks on me", 1},
		{"lots keeps all", "lots", ToneGood, nil, fixedRoll{}, text, text, 0},
		{"serious strips at every level", "lots", ToneSerious, nil, fixedRoll{hits: true}, "so sorry 😢 here for you 🙏", "so sorry here for you", 2},
		{"venting loses laughing emoji only", "lots", ToneVent, nil, fixedRoll{}, "ugh 😂 that's awful 😤", "ugh that's awful 😤", 1},
		{"unknown usage counts as rare", "", ToneFunny, nil, fixedRoll{hits: true}, "lol 😂😂", "lol 😂", 1},
	}
	for _, c := range cases {
		got := EnforceEmoji(c.text, prefs(c.usage), c.tone, c.mine, c.roll)
		if got.Text != c.want || got.Removed != c.remove || got.Added != "" {
			t.Errorf("%s: got %+v, want %q (removed %d)", c.name, got, c.want, c.remove)
		}
	}
	// lots: an emoji-free reply gets a fitting favourite (never on serious/venting).
	got := EnforceEmoji("you got the job, darling", prefs("lots"), ToneGood, nil, fixedRoll{hits: true})
	if got.Added != "✨" || got.Text != "you got the job, darling ✨" {
		t.Errorf("lots add = %+v", got)
	}
	if got := EnforceEmoji("you got the job", prefs("lots"), ToneGood, nil, fixedRoll{}); got.Added != "" {
		t.Errorf("lots add without the roll = %+v", got)
	}
	if got := EnforceEmoji("so sorry", prefs("lots"), ToneSerious, nil, fixedRoll{hits: true}); got.Added != "" {
		t.Errorf("lots add on serious = %+v", got)
	}
	if got := EnforceEmoji("hmm ok", prefs("lots"), ToneQuestion, nil, fixedRoll{hits: true}); got.Added != "💅" {
		t.Errorf("a calm favourite suits an everyday question: %+v", got)
	}
	laughs := model.EmojiPrefs{Usage: "lots", Favorites: []string{"😂", "💀"}}
	if got := EnforceEmoji("hmm ok", laughs, ToneQuestion, nil, fixedRoll{hits: true}); got.Added != "" {
		t.Errorf("no laughing emoji on a question: %+v", got)
	}
	if got := EnforceEmoji("ugh that's awful", laughs, ToneVent, nil, fixedRoll{hits: true}); got.Added != "" {
		t.Errorf("nothing added to venting: %+v", got)
	}
	// rare / some add one at their rate; rare never right after a recent emoji.
	if got := EnforceEmoji("congrats!", prefs("rare"), ToneGood, nil, fixedRoll{hits: true}); got.Added != "✨" {
		t.Errorf("rare add = %+v", got)
	}
	if got := EnforceEmoji("congrats!", prefs("rare"), ToneGood, []string{"yay 🎉"}, fixedRoll{hits: true}); got.Added != "" {
		t.Errorf("rare add after a recent emoji = %+v", got)
	}
	if got := EnforceEmoji("lol same", model.EmojiPrefs{Usage: "some"}, ToneFunny, nil, fixedRoll{hits: true}); got.Added != "😂" {
		t.Errorf("some add with no favourites = %+v", got)
	}
	if got := EnforceEmoji("congrats!", prefs("none"), ToneGood, nil, fixedRoll{hits: true}); got.Added != "" {
		t.Errorf("none never adds: %+v", got)
	}
}

func TestClassifyTone(t *testing.T) {
	cases := map[string]Tone{
		"my grandmother passed away this morning":              ToneSerious,
		"I got laid off today. not really sure what to do now": ToneSerious,
		"honestly I've been feeling really depressed lately":   ToneSerious,
		"סבא שלי נפטר אתמול, אני הרוס":                         ToneSerious,
		"I GOT THE JOB!!! starting next month":                 ToneGood,
		"קיבלתי את הדירה!!! סוף סוף":                           ToneGood,
		"we're engaged!!": ToneGood,
		"thank you so much for yesterday, you're the best": ToneLove,
		"תודה רבה על אתמול":                                ToneLove,
		"lol":                                              ToneFunny,
		"why did the scarecrow win an award? he was outstanding": ToneFunny,
		"חחחח אתה לא תאמין מה קרה לי היום":                       ToneFunny,
		"wait, Noa and Tom broke up??":                           ToneSurprise,
		"drinks on friday at 8?":                                 TonePlan,
		"מה אתה עושה בסופ״ש?":                                    ToneQuestion, // asks, doesn't propose
		"בא לך לצאת בסופ״ש?":                                     TonePlan,
		"what time should we meet tomorrow?":                     ToneQuestion,
		"what time works for you?":                               ToneQuestion,
		"hey":                                                    ToneGreeting,
		"היי":                                                    ToneGreeting,
		"ok":                                                     ToneNeutral,
		"ugh my landlord didn't fix the heater again, so done with him": ToneVent,
		"":                                     ToneNeutral,
		"I almost died laughing at that video": ToneFunny,
		"rip my wallet lol":                    ToneFunny,
		"הייתי הרוס מצחוק":                     ToneFunny,
		"that's sick bro":                      ToneNeutral,
		"I'm sick since yesterday":             ToneSerious,
		"my dog is dying":                      ToneSerious,
		"this meme has me dying":               ToneFunny,
	}
	for in, want := range cases {
		if got := ClassifyTone(in); got != want {
			t.Errorf("ClassifyTone(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestReactionFor(t *testing.T) {
	leo := model.EmojiPrefs{Usage: "rare", Favorites: []string{"💅", "✨", "🍸", "🛋️"}}
	cases := []struct {
		prefs model.EmojiPrefs
		text  string
		want  string
		tone  Tone
	}{
		{leo, "lol my cat just did the funniest thing", "😂", ToneFunny}, // no funny favourite → default
		{leo, "I GOT THE JOB!!!", "✨", ToneGood},                        // fitting favourites: ✨ 🍸 (💅 is too niche)
		{leo, "thank you so much, you're the best", "✨", ToneLove},      // ✨ suits thanks
		{leo, "my grandmother passed away", "❤️", ToneSerious},          // never a favourite that doesn't fit
		{model.EmojiPrefs{Usage: "lots", Favorites: []string{"😂", "💀"}}, "I got laid off today", "❤️", ToneSerious},
		{model.EmojiPrefs{Usage: "lots", Favorites: []string{"😂", "💀"}}, "ugh worst day at work, so done", "😢", ToneVent},
		{model.EmojiPrefs{Usage: "lots", Favorites: []string{"😂", "💀"}}, "חחחח מת", "😂", ToneFunny},
		{model.EmojiPrefs{Usage: "none", Favorites: []string{"🔥"}}, "we're engaged!!", "❤️", ToneGood},
		{model.EmojiPrefs{Usage: "none"}, "drinks friday?", "👍", TonePlan},
		{model.EmojiPrefs{}, "wait what?? no way", "😮", ToneSurprise},
	}
	for _, c := range cases {
		got, tone := ReactionFor(c.prefs, c.text, fixedRoll{})
		if got != c.want || tone != c.tone {
			t.Errorf("ReactionFor(%v, %q) = %q (%s), want %q (%s)", c.prefs.Favorites, c.text, got, tone, c.want, c.tone)
		}
	}
	// No draw ever laughs at bad news.
	for i := 0; i < 5; i++ {
		got, _ := ReactionFor(model.EmojiPrefs{Usage: "lots", Favorites: []string{"😂", "🤣", "💀", "🙏"}}, "סבא שלי נפטר", fixedRoll{idx: i})
		if strings.Contains("😂🤣💀😆", got) {
			t.Errorf("laughing reaction to bad news: %q", got)
		}
	}
}

func TestWordBudget(t *testing.T) {
	rant := strings.Repeat("word ", 30)
	cases := []struct {
		length, bias, in string
		want             int
	}{
		{"short", "normal", "hey", 12},
		{"short", "", "hey", 12},
		{"", "normal", "hey", 12},
		{"medium", "normal", "hey", 30},
		{"long", "normal", "hey", 60},
		{"short", "shorter", "hey", 7},
		{"short", "longer", "hey", 19},
		{"medium", "shorter", "hey", 18},
		{"long", "longer", "hey", 96},
		{"short", "normal", rant, 15}, // long message: ×1.25
		{"short", "match", "ok", 4},
		{"short", "match", "do you know a good place for dinner tonight?", 12},
		{"short", "match", rant, 19}, // capped at the "longer" budget
		{"long", "match", rant, 33},
	}
	for _, c := range cases {
		if got := WordBudget(c.length, c.bias, c.in); got != c.want {
			t.Errorf("WordBudget(%q, %q, %d words) = %d, want %d", c.length, c.bias, len(strings.Fields(c.in)), got, c.want)
		}
	}
	if HardLimit(12) != 16 || HardLimit(7) != 10 || HardLimit(60) != 80 {
		t.Errorf("HardLimit: %d %d %d", HardLimit(12), HardLimit(7), HardLimit(60))
	}
	if g := Guidance("short", "hey", "normal"); !strings.Contains(g, "at most 12 words") || !strings.HasPrefix(g, "Keep it brief") {
		t.Errorf("Guidance = %q", g)
	}
}

func TestTrimToWords(t *testing.T) {
	cases := []struct {
		in    string
		limit int
		want  string
	}{
		{"Short and sweet.", 5, "Short and sweet."},
		{"Darling, that sofa is a crime. Burn it immediately. Then call me and we'll pick something decent together.", 10, "Darling, that sofa is a crime. Burn it immediately."},
		{"Honestly I loved it! The second half dragged. Want to go again?", 4, "Honestly I loved it!"},
		{"wow 🎉 so proud of you!! seriously this is huge and you deserve every bit of it", 6, "wow 🎉 so proud of you!!"},
		{"first line here\nsecond line is much longer than the first one", 4, "first line here"},
		{"this sentence just keeps going and going, with no full stop in sight and more words", 9, "this sentence just keeps going and going"},
		{"no commas here at all just one very long run of words without any break", 6, "no commas here at all…"},
		{"omg bestie i am literally so happy for you right now", 8, "omg bestie i am literally so happy for you right now"}, // ≤ 1.5× kept whole
		{"tell @Dana Cohen about it and also bring the wine and the cheese please", 2, "tell…"},
		{"check https://example.com/a-very/long?path=1 then call me back later tonight", 2, "check https://example.com/a-very/long?path=1…"},
		{"שלום חבר. מה שלומך היום? אני מקווה שהכל טוב אצלך", 5, "שלום חבר. מה שלומך היום?"},
	}
	for _, c := range cases {
		got, _ := TrimToWords(c.in, c.limit)
		if got != c.want {
			t.Errorf("TrimToWords(%q, %d) = %q, want %q", c.in, c.limit, got, c.want)
		}
	}
	if _, ok := TrimToWords("fits fine", 5); ok {
		t.Error("ok must be false when nothing was trimmed")
	}
}
