package persona

import (
	"hash/fnv"
	"slices"
	"strings"
	"unicode"

	"whatsappdoppel/internal/model"
)

// Gradients are the two-color pairs used for generated avatars.
var Gradients = [][2]string{
	{"#f472b6", "#8b5cf6"},
	{"#38bdf8", "#6366f1"},
	{"#34d399", "#0f766e"},
	{"#fb923c", "#ef4444"},
	{"#c084fc", "#f9a8d4"},
	{"#facc15", "#f97316"},
	{"#22d3ee", "#3b82f6"},
	{"#a3e635", "#16a34a"},
	{"#f43f5e", "#a855f7"},
	{"#60a5fa", "#14b8a6"},
}

// Glyphs are the ids of the custom avatar graphics drawn by the frontend
// (web/components/glyphs.js). Keep the two lists in sync.
var Glyphs = []string{
	"spark", "rocket", "crystal", "cash", "dumbbell", "martini", "sofa", "palette",
	"coffee", "headphones", "leaf", "moon", "sun", "wave", "flame", "heart",
	"book", "camera", "crown", "paw", "flower", "bolt", "chat", "globe",
}

// IsGlyph reports whether id names a known avatar glyph.
func IsGlyph(id string) bool { return slices.Contains(Glyphs, id) }

// GeneratedAvatar derives a stable gradient, glyph and initials from a name.
func GeneratedAvatar(name string) model.Avatar {
	h := fnv.New32a()
	h.Write([]byte(strings.ToLower(strings.TrimSpace(name))))
	sum := h.Sum32()
	g := Gradients[sum%uint32(len(Gradients))]
	return model.Avatar{
		Kind:     "generated",
		Gradient: []string{g[0], g[1]},
		Glyph:    Glyphs[(sum/7)%uint32(len(Glyphs))],
		Initials: Initials(name),
	}
}

// Initials returns up to two uppercase initials ("Chad Remington" → "CR").
func Initials(name string) string {
	words := strings.FieldsFunc(name, func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) })
	var out []rune
	for _, w := range words {
		r := []rune(w)
		out = append(out, unicode.ToUpper(r[0]))
		if len(out) == 2 {
			break
		}
	}
	if len(out) == 0 {
		return "?"
	}
	return string(out)
}
