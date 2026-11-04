package prompt

// LanguageSection tells the persona which language to write in (Options.
// ReplyLanguage, picked by internal/logic/lang). It comes after STAY YOURSELF so it
// wins over a language named in the profile: the chat settings decide the
// language, the profile only how the persona writes in it.
func LanguageSection(name string) string {
	if name == "" {
		return ""
	}
	return "\n\nREPLY LANGUAGE: write this message in " + name + ". The chat settings choose the language and this overrides any language named in your profile above; keep your own voice, slang and style, just in " + name + "."
}
