package model

// LanguagePrefs are the languages a persona speaks in a chat and how it
// picks the language of a reply (settings.behavior.languages.private|group →
// ChatAssignment.Language → PersonPrefs.Language; see internal/lang).
type LanguagePrefs struct {
	// Languages it speaks (codes, e.g. "en", "he"); empty = English and Hebrew.
	Languages []string `json:"languages"`
	// Others: a message clearly in another language is ignored (stays quiet,
	// kept as context) or answered. "" = ignore.
	Others string `json:"others"`
	// Reply: "match" (the language of the message it answers) or a language
	// code (always that one). "" = match.
	Reply string `json:"reply"`
}

// Language choices (LanguagePrefs.Others, .Reply, PersonPrefs.Language).
const (
	LangIgnore = "ignore"
	LangAnswer = "answer"
	LangMatch  = "match"
)
