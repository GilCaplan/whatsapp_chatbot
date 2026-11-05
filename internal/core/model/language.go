package model

// LanguagePrefs are the languages a persona understands in a chat, the ones
// it replies to, and how it picks the language of a reply (settings.behavior.languages.private|group →
// ChatAssignment.Language → PersonPrefs.Language; see internal/logic/lang).
type LanguagePrefs struct {
	// Understands: the languages it can read (codes); empty = all. A message
	// clearly in any other is left out altogether, not even kept as context.
	Understands []string `json:"understands,omitempty"`
	// Languages it replies to (codes, e.g. "en", "he"); empty = English and Hebrew.
	Languages []string `json:"languages"`
	// Others: a message clearly in another language it understands is ignored
	// (stays quiet, kept as context) or answered. "" = ignore.
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
