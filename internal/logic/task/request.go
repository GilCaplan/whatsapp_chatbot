package task

import (
	"regexp"
	"strings"
)

// Meeting requests (English + Hebrew): what makes a secretary start Find a
// time. Social plans ("let's grab coffee") are the hand-off classifier's.
var (
	reMeetEN = regexp.MustCompile(`(?i)\b(?:meeting|meet (?:with|up with)|set up a (?:time|call)|schedule (?:a|an|some)|book (?:a|an|some) (?:time|slot|call|meeting)|` +
		`(?:a|quick|short) (?:call|chat|zoom)|zoom|appointment|when (?:is|are) (?:he|she|they|you|\w+) (?:free|available)|` +
		`(?:is|are) (?:he|she|they|you|\w+) (?:free|available) (?:on|at|this|next|tomorrow)|find a time|a time to (?:talk|meet))\b`)
	meetHE = []string{"פגישה", "להיפגש", "לקבוע", "שיחת זום", "זום", "שיחה קצרה", "פנוי ב", "פנויה ב", "זמין ב", "זמינה ב", "מתי הוא פנוי", "מתי היא פנויה", "מתי אתה פנוי", "מתי את פנויה", "לתאם"}
)

// MeetingRequest reports whether text asks for a meeting or a call.
func MeetingRequest(text string) bool {
	if reMeetEN.MatchString(text) {
		return true
	}
	for _, w := range meetHE {
		if strings.Contains(text, w) {
			return true
		}
	}
	return false
}
