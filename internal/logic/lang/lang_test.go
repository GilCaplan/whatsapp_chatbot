package lang

import (
	"testing"

	"whatsappdoppel/internal/core/model"
)

func TestDetect(t *testing.T) {
	for _, c := range []struct {
		text string
		want string
		sure bool
	}{
		{"what are you doing tonight?", "en", true},
		{"מה אתה עושה הערב?", "he", true},
		{"מה קורה, did you see the game", "en", true}, // more English letters
		{"אחי זה היה מטורף lol", "he", true},
		{"привет, как дела?", "ru", true},
		{"привіт, як справи? Їдемо", "uk", true},
		{"¿qué haces esta noche?", "es", true},
		{"je ne sais pas, c'est la vie", "fr", true},
		{"ich bin nicht da, danke", "de", true},
		{"merhaba, nasılsın? çok iyi", "tr", true},
		{"مرحبا كيف حالك", "ar", true},
		{"今日は何をしますか", "ja", true},
		{"안녕하세요", "ko", true},
		{"ok", "", false},
		{"lol", "", false},
		{"😂😂", "", false},
		{"haha", "", false},
		{"Vsevolod", "", false},
		// English with one word another language shares: never sure.
		{"a quick question", "", false},
		{"come here", "", false},
		{"che guevara shirt", "", false},
	} {
		got, sure := Detect(c.text)
		if sure != c.sure || (c.sure && got != c.want) {
			t.Errorf("Detect(%q) = %q %v, want %q %v", c.text, got, sure, c.want, c.sure)
		}
	}
}

func TestMain(t *testing.T) {
	if got := Main([]string{"lol", "מה קורה", "ok", "מתי נפגשים?", "see you there"}); got != "he" {
		t.Errorf("Main = %q", got)
	}
	if got := Main([]string{"ok", "😂"}); got != "" {
		t.Errorf("unsure Main = %q", got)
	}
}

func TestPrefsAndReply(t *testing.T) {
	p := Effective(model.LanguagePrefs{}, nil)
	if !Speaks(p, "en") || !Speaks(p, "he") || p.Others != model.LangIgnore || p.Reply != model.LangMatch {
		t.Fatalf("defaults = %+v", p)
	}
	if !Ignores(p, "ru") || Ignores(p, "he") || Ignores(p, "") {
		t.Error("ignore rules")
	}
	chat := &model.LanguagePrefs{Languages: []string{"ru"}, Others: model.LangAnswer}
	if q := Effective(p, chat); Speaks(q, "en") || Ignores(q, "fr") {
		t.Errorf("chat block replaces the app one: %+v", q)
	}
	for _, c := range []struct {
		person, msg, conv, want string
	}{
		{"", "he", "en", "he"},      // match the message
		{"", "", "he", "he"},        // unclear: the conversation
		{"", "ru", "en", "en"},      // not spoken here: the conversation
		{"", "", "", "en"},          // nothing known: the first language
		{"he", "en", "en", "he"},    // the person's language
		{"match", "he", "en", "he"}, // the person's "match"
	} {
		if got := Reply(p, c.person, c.msg, c.conv); got != c.want {
			t.Errorf("Reply(%q, %q, %q) = %q, want %q", c.person, c.msg, c.conv, got, c.want)
		}
	}
	if got := Reply(model.LanguagePrefs{Languages: []string{"en"}, Reply: "he"}, "", "en", "en"); got != "he" {
		t.Errorf("fixed chat language = %q", got)
	}
	for _, bad := range []model.LanguagePrefs{{Languages: []string{"xx"}}, {Languages: []string{"en", "en"}}, {Others: "maybe"}, {Reply: "klingon"}} {
		if Validate(bad) == nil {
			t.Errorf("%+v accepted", bad)
		}
	}
	if !ValidPerson("") || !ValidPerson("match") || !ValidPerson("he") || ValidPerson("x") {
		t.Error("ValidPerson")
	}
}

func TestFixed(t *testing.T) {
	cases := []struct {
		chat, person string
		want         bool
	}{{"", "", false}, {model.LangMatch, "", false}, {"ru", "", true}, {"", "he", true}, {"ru", model.LangMatch, false}}
	for _, c := range cases {
		if got := Fixed(model.LanguagePrefs{Reply: c.chat}, c.person); got != c.want {
			t.Errorf("Fixed(%q, %q) = %v", c.chat, c.person, got)
		}
	}
}

func TestOffAlphabet(t *testing.T) {
	cases := []struct {
		text, code string
		want       bool
	}{
		{"sure thing mate", "ru", true}, {"לא, תגידי לנו.", "ru", true}, {"Нет, скажи нам.", "ru", false},
		{"Привет Ben, как дела?", "ru", false}, {"ok", "ru", false}, {"😀 😀", "ru", false},
		{"sure thing mate", "en", false}, {"sure thing mate", "es", false}, {"こんにちは 友達", "ja", false},
	}
	for _, c := range cases {
		if got := OffAlphabet(c.text, c.code); got != c.want {
			t.Errorf("OffAlphabet(%q, %q) = %v", c.text, c.code, got)
		}
	}
}

func TestUnderstandsLimitsWhatItReads(t *testing.T) {
	p := Effective(model.LanguagePrefs{Understands: []string{"en"}, Languages: []string{"he"}, Others: model.LangAnswer}, nil)
	if !Understood(p, "he") || Understood(p, "ru") {
		t.Fatalf("understands = %v", p.Understands)
	}
	if !Ignores(p, "ru") || Ignores(p, "en") {
		t.Fatal("an unknown language is ignored even when others are answered")
	}
	if got := Reply(p, "", "ru", "en"); got != "en" {
		t.Fatalf("reply = %s", got)
	}
	if !Understood(Effective(model.LanguagePrefs{}, nil), "ru") {
		t.Fatal("empty means all")
	}
}
