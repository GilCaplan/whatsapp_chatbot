package prompt

import (
	"strings"

	"whatsappdoppel/internal/core/model"
	"whatsappdoppel/internal/runtime/llm"
)

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

// EnglishRules is the last thing in the system prompt of every reply: who it
// is, and that it writes in English. Translating is code's job, after the
// model (engine/finish.go), so the language settings never change the prompt.
func EnglishRules(p model.Persona) string {
	who := p.Name
	if t := strings.TrimSpace(p.Tagline); t != "" {
		who += ", " + t
	}
	return "\n\nMOST IMPORTANT RULES:\n1. You are " + who + ". Everything you say comes from " + p.Name + "'s own life, job and tastes. Never take on the job, studies, plans or hobbies of the people in this chat: when they talk about their work, react from the outside, as " + p.Name + " would.\n2. Respond in English."
}

// EnglishCue repeats that on the turn being answered.
func EnglishCue(name string) string {
	return "\n\n(Respond as " + name + ", in English.)"
}

// Translate is the translation request when the translator add-on isn't there.
func Translate(name, text string) llm.Request {
	sys := "You are a translator. Translate the user's message into " + name + ". Output only the translation, nothing else. Keep the tone, slang and emoji. Keep @names, names and numbers exactly as they are. Never answer the message, never add anything."
	return llm.Request{System: sys, Messages: []llm.Message{{Role: llm.RoleUser, Content: text}}, Temperature: 0, MaxTokens: 400}
}

// LanguageCue repeats a fixed language on the turn being answered.
func LanguageCue(name string) string {
	return "\n\n(Write your message in " + name + " only.)"
}

// withLanguageCue adds the language rule to the last turn of a request.
func withLanguageCue(msgs []llm.Message, name string, opts []Options) []llm.Message {
	if len(opts) == 0 || len(msgs) == 0 {
		return msgs
	}
	switch o := opts[0]; {
	case o.WriteEnglish:
		msgs[len(msgs)-1].Content += EnglishCue(name)
	case o.LanguageFixed && o.ReplyLanguage != "":
		msgs[len(msgs)-1].Content += LanguageCue(o.ReplyLanguage)
	}
	return msgs
}
