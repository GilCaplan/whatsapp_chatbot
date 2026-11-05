package prompt

import "whatsappdoppel/internal/runtime/llm"

// LanguageSection tells the persona which language to write in (Options.
// ReplyLanguage, picked by internal/logic/lang). It comes after STAY YOURSELF so it
// wins over a language named in the profile: the chat settings decide the
// language, the profile only how the persona writes in it. A language chosen
// in the settings is stricter: see FixedLanguageSection.
func LanguageSection(name string) string {
	if name == "" {
		return ""
	}
	return "\n\nREPLY LANGUAGE: write this message in " + name + ". The chat settings choose the language and this overrides any language named in your profile above; keep your own voice, slang and style, just in " + name + "."
}

// FixedLanguageSection is the language chosen in the settings
// (Options.LanguageFixed): the last thing in the system prompt, because a
// small model follows the language of the chat over a rule it read earlier.
func FixedLanguageSection(name string) string {
	return "\n\nMOST IMPORTANT RULE, REPLY LANGUAGE: write every word of this message in " + name + ", and nothing else. This group's or person's language setting is " + name + ". The others may write in any language, and your own earlier messages may be in another language: understand them, then answer in " + name + " anyway. Never switch to the language of the chat. This overrides everything above; keep your own voice and style, just in " + name + "."
}

// LanguageCue repeats a fixed language on the turn being answered.
func LanguageCue(name string) string {
	return "\n\n(Write your message in " + name + " only.)"
}

// withLanguageCue adds LanguageCue to the last turn of a request.
func withLanguageCue(msgs []llm.Message, opts []Options) []llm.Message {
	if len(opts) > 0 && opts[0].LanguageFixed && opts[0].ReplyLanguage != "" && len(msgs) > 0 {
		msgs[len(msgs)-1].Content += LanguageCue(opts[0].ReplyLanguage)
	}
	return msgs
}
