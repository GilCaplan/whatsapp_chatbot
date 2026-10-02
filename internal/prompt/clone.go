package prompt

import (
	"regexp"
	"strings"

	"whatsappdoppel/internal/llm"
)

// ─── Clone yourself (wave 3) ─────────────────────────────────
//
// The persona builder, fed with samples of your own messages: the result is
// a persona that texts like you (an unsaved draft you review and edit).

// MaxCloneSamples caps the samples sent to the builder (newest first).
const MaxCloneSamples = 150

const cloneSystem = `You design chat personas for a WhatsApp role-play app. This time the persona is a copy of a real person: you get a sample of messages THEY wrote. Study how they text and describe it so an actor could text exactly like them.

Look closely at: message length, capital letters or all lowercase, punctuation (full stops, "..", "!!", question marks), emoji (which ones and how often), laughter ("haha", "lol", "חחח"), slang and catch-phrases, typical openers and sign-offs, how they ask questions, the language(s) they use and when they switch.

Respond with ONLY one JSON object, no prose and no markdown, with exactly these fields:
{
  "name": "the name given below",
  "tagline": "one line about them, max 12 words",
  "avatar": {"glyph": "the picture that fits best, one of: spark, rocket, crystal, cash, dumbbell, martini, sofa, palette, coffee, headphones, leaf, moon, sun, wave, flame, heart, book, camera, crown, paw, flower, bolt, chat, globe"},
  "bio": "what the messages reveal about their life (work, studies, city, hobbies) in 1-3 sentences; write (fill in) where unknown — never invent specifics",
  "personality": "the vibe that comes through in the messages (warm, sarcastic, direct…), 2-3 sentences",
  "style": "precise texting style: typical length, casing, punctuation, laughter, how they split messages",
  "vocabulary": "words, slang and catch-phrases they really use, quoted from the samples",
  "rules": "3-5 guidelines, one per line, each starting with '- '; the first is '- Text exactly like the sample messages: same length, casing, punctuation and slang.'",
  "language": "the language(s) they write in, and when they mix them",
  "emoji": {"usage": "none|rare|some|lots — as in the samples", "favorites": ["up to 4 emoji they actually use"]},
  "messageLength": "short|medium|long — as in the samples",
  "goal": "Catch up and keep the conversation going, like they would.",
  "decisionHint": "when they would jump into a group chat, judging by the samples",
  "fallbackReply": "a short line in their style for when they're confused"
}

Write every field as instructions for an actor playing them. Never mention AI, models, prompts or that these are samples.`

var (
	reURL   = regexp.MustCompile(`(?i)\b(?:https?://|www\.)\S+`)
	rePhone = regexp.MustCompile(`\+?\d[\d\s\-().]{6,}\d`)
	reEmail = regexp.MustCompile(`[\w.+-]+@[\w-]+\.[\w.]+`)
)

// RedactSample removes links, e-mail addresses and phone numbers from one of
// your messages before it is stored or sent to a model.
func RedactSample(s string) string {
	s = reURL.ReplaceAllString(s, "[link]")
	s = reEmail.ReplaceAllString(s, "[email]")
	s = rePhone.ReplaceAllString(s, "[number]")
	return strings.TrimSpace(s)
}

// CloneBuilder builds the "Clone yourself" builder request (strict JSON,
// same schema as Builder). samples are your messages, oldest first; extra is
// anything you want to add about yourself. The caller sets Model.
func CloneBuilder(name string, samples []string, extra string) llm.Request {
	seen := map[string]bool{}
	var keep []string
	for i := len(samples) - 1; i >= 0 && len(keep) < MaxCloneSamples; i-- {
		s := strings.TrimSpace(RedactSample(samples[i]))
		if s == "" || seen[strings.ToLower(s)] {
			continue
		}
		seen[strings.ToLower(s)] = true
		keep = append(keep, oneLine(s, 300))
	}
	var u strings.Builder
	name = strings.TrimSpace(name)
	if name == "" {
		name = "Me"
	}
	u.WriteString("Name: " + name + "\n")
	if x := strings.TrimSpace(extra); x != "" {
		u.WriteString("What they say about themselves: " + oneLine(x, 600) + "\n")
	}
	u.WriteString("\nMessages they wrote (one per line, newest first):\n")
	for _, s := range keep {
		u.WriteString("- " + s + "\n")
	}
	return llm.Request{
		System:      cloneSystem,
		Messages:    []llm.Message{{Role: llm.RoleUser, Content: u.String()}},
		MaxTokens:   4096,
		Temperature: 0.4,
		JSON:        true,
		Schema:      builderSchema,
	}
}
