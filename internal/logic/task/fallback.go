package task

import (
	"strings"
	"time"

	"whatsappdoppel/internal/core/model"
)

// Fallback is a safe message written by code, used when a draft fails the
// leak check twice. code is the reply language ("he" = Hebrew, else English).
func Fallback(t Turn, tone model.TaskTone, code string, loc *time.Location) string {
	if loc == nil {
		loc = time.Local
	}
	he := code == "he"
	var times []string
	for _, x := range t.Times {
		times = append(times, When(x.In(loc), tone, code))
	}
	var wins []string
	for _, w := range t.Windows {
		wins = append(wins, WindowText(w, tone, code))
	}
	pick := func(en, hb string) string {
		if he {
			return hb
		}
		return en
	}
	switch t.Say {
	case SayOffer:
		if len(times) > 0 {
			return pick("How about "+List(times, code)+"?", "מה דעתך על "+List(times, code)+"?")
		}
	case SayWindows:
		if len(wins) > 0 {
			return pick("Would "+List(wins, code)+" suit you?", "מתאים לך "+List(wins, code)+"?")
		}
	case SayAnswerNo:
		if len(times) > 0 {
			return pick("That one doesn't work for me, sorry. Would "+times[0]+" work?", "זה לא מסתדר לי, סליחה. מתאים "+times[0]+"?")
		}
		return pick("That one doesn't work for me, sorry. What else would suit you?", "זה לא מסתדר לי, סליחה. מה עוד מתאים לך?")
	case SayAskConfirm, SayHold:
		if len(times) > 0 {
			if t.Say == SayHold {
				return pick(times[0]+" sounds good, I'll confirm very soon.", times[0]+" נשמע טוב, אאשר ממש בקרוב.")
			}
			return pick("Shall we say "+times[0]+"?", "אז "+times[0]+"?")
		}
	case SayAskWhich:
		if len(times) > 0 {
			return pick("Great, which one: "+List(times, code)+"?", "מעולה, איזה מהם: "+List(times, code)+"?")
		}
	case SayAskSlot:
		if t.Slot != nil {
			return pick("And "+strings.ToLower(t.Slot.Label)+"?", "ו"+t.Slot.Label+"?")
		}
	case SayCover:
		if t.Cover != "" {
			return t.Cover
		}
		return pick("I've got something then, sorry.", "יש לי משהו אז, סליחה.")
	case SayNudge:
		return pick("Hi, just checking: did you get a chance to look at a time?", "היי, רק בודק/ת אם יצא לך לחשוב על זמן?")
	case SayWait:
		return pick("No rush, talk soon.", "אין לחץ, נדבר.")
	case SayConfirmed:
		if len(times) > 0 {
			return pick("Great, it's set for "+times[0]+". Thanks!", "מעולה, קבענו ל"+times[0]+". תודה!")
		}
	case SayHandoff:
		return pick("Let me get back to you on this soon.", "אחזור אליך על זה בקרוב.")
	case SayCheck:
		return pick("Let me check and get back to you very soon.", "אבדוק ואחזור אליך ממש בקרוב.")
	case SayGoodbye:
		return pick("Let's leave it for now, talk soon.", "בוא נשאיר את זה לבינתיים, נדבר.")
	}
	return pick("Let me get back to you on a time.", "אחזור אליך לגבי זמן.")
}
