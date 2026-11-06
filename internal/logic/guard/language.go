package guard

import (
	"regexp"
	"slices"
	"strings"
	"unicode"
)

// A persona writes in English and code translates, so which language anyone
// used is never a persona's topic: small models sometimes make it one, with or
// without a rule against it. Unless someone else brought it up, a reply that
// talks about language doesn't go out.

// languageTalk is talk about language in general, whichever one.
var languageTalk = regexp.MustCompile(`(?i)\b(?:in|speak|speaks|speaking|write|writing|different|another|foreign|other|which|what|this|that|wrong)\s+(?:a\s+|the\s+)?languages?\b|\btranslat(?:e[sd]?|ing|ions?|ors?)\b`)

// usedAs: before a name, the words that make it the language rather than a
// style or a dish ("Italian marble" has none of them and a word after it).
const usedAs = `all the|all this|all that|so much|more|in|use|uses|using|speak|speaks|speaking|write|writes|writing|type|types|typing|talk|talks|talking|say|saying|reply|replies|replying|respond|responding|answer|answering|read|reading|understand|understands|understood|know|learn|learning`

// pointedAt: these only point at a name, which may as well be a dish ("the
// Thai around here"), so they take the shorter list of what may follow.
const pointedAt = `the|this|that|your`

// after: what may follow a name used as a language.
const after = `\s*(?:$|[.,;:!?…)"'’”\p{So}])|\s+(?:messages?|texts?|stuff|talk|words?)\b`

// afterUsed: what may also follow it after usedAs.
const afterUsed = after + `|\s+(?:around|here|there|again|anymore|now|today|everywhere|please|in\s+(?:here|this|the\s+(?:chat|group)))\b`

// aboutLanguage: people ask in their own language and the history holds their
// words as they wrote them, so "language", "translate" and "English" are here
// in each one. Parts of words, as most of these languages add endings.
var aboutLanguage = []string{
	"language", "translat",
	"שפה", "שפות", "תרגום", "תרגם", "תתרגם", "מתרגם", "אנגלית",
	"لغة", "ترجم", "إنجليزي", "انجليزي",
	"язык", "перевод", "перевед", "английск", "русск",
	"мова", "мови", "переклад", "англійськ", "українськ",
	"idioma", "lengua", "traduc", "inglés",
	"langue", "tradui", "anglais",
	"sprache", "übersetz", "englisch",
	"lingua", "tradu", "inglese",
	"língua", "inglês",
	"taal", "vertaal", "engels",
	"çevir", "ingilizce",
	"γλώσσα", "μετάφρ", "αγγλικά",
	"भाषा", "अनुवाद", "अंग्रेज़ी",
	"ภาษา", "แปล",
	"语言", "翻译", "英文", "英语",
	"言語", "翻訳", "英語",
	"언어", "번역", "영어",
}

// TalksAboutLanguage reports whether reply talks about the language anyone
// writes in. names are the languages' names, in English and in themselves;
// others is what other people said lately, which can make it the topic;
// spoken names the languages they wrote it in. Naming one of those at all is a
// remark about them, even as an adjective ("Enough of the French poetry").
func TalksAboutLanguage(reply string, names, spoken, others []string) bool {
	said := strings.ToLower(strings.Join(others, "\n"))
	if slices.ContainsFunc(aboutLanguage, func(w string) bool { return strings.Contains(said, w) }) {
		return false
	}
	words := strings.FieldsFunc(said, func(r rune) bool { return !unicode.IsLetter(r) })
	alts := make([]string, 0, len(names))
	for _, n := range names {
		if mentions(said, words, strings.ToLower(n)) {
			return false
		}
		alts = append(alts, regexp.QuoteMeta(n))
	}
	if languageTalk.MatchString(reply) {
		return true
	}
	if len(spoken) > 0 {
		quoted := make([]string, len(spoken))
		for i, s := range spoken {
			quoted[i] = regexp.QuoteMeta(s)
		}
		if regexp.MustCompile(`(?i)\b(?:` + strings.Join(quoted, "|") + `)\b`).MatchString(reply) {
			return true
		}
	}
	name := `\s+(?:` + strings.Join(alts, "|") + `)`
	named := `(?im)\b(?:(?:` + pointedAt + `)` + name + `(?:` + after + `)|(?:` + usedAs + `)` + name + `(?:` + afterUsed + `))`
	return regexp.MustCompile(named).MatchString(reply)
}

// mentions: a plain Latin name is a whole word, so Germany doesn't count as
// German. Other scripts join words on ("בעברית" is "in Hebrew").
func mentions(said string, words []string, name string) bool {
	if strings.IndexFunc(name, func(r rune) bool { return r > unicode.MaxASCII }) < 0 {
		return slices.Contains(words, name)
	}
	return strings.Contains(said, name)
}
