// Package handoff spots messages a real person should answer themselves —
// money, health, meeting up, distress, "are you a bot?", legal — with a
// keyword classifier (English and Hebrew). A strong hit pauses the persona on
// its own; a weak hit is a maybe that the engine may confirm with a tiny AI
// check (prompt.HandoffCheck). It imports only model and the standard library.
package handoff

import (
	"regexp"
	"strings"

	"whatsappdoppel/internal/model"
)

// Strength of a keyword hit.
const (
	Strong = "strong" // pause right away
	Weak   = "weak"   // maybe: ask the AI when that is switched on
)

// Hit is the outcome of Detect.
type Hit struct {
	Category string // model.Handoff* category
	Strength string // Strong | Weak
	Match    string // the words that matched
}

// Categories says which categories are switched on (model.Handoff* keys).
type Categories map[string]bool

// All switches every category on.
func All() Categories {
	c := Categories{}
	for _, k := range model.HandoffCategories {
		c[k] = true
	}
	return c
}

// List returns the enabled categories in display order.
func (c Categories) List() []string {
	var out []string
	for _, k := range model.HandoffCategories {
		if c[k] {
			out = append(out, k)
		}
	}
	return out
}

// priority is the order categories are checked in: the most urgent first.
var priority = []string{
	model.HandoffDistress, model.HandoffHealth, model.HandoffBot,
	model.HandoffMoney, model.HandoffLegal, model.HandoffMeeting,
}

// Detect scans text for the enabled categories. Strong hits win over weak
// ones; within a strength the most urgent category wins.
func Detect(text string, cats Categories) (Hit, bool) {
	s := Normalize(text)
	if s == "" {
		return Hit{}, false
	}
	for _, strength := range []string{Strong, Weak} {
		for _, cat := range priority {
			if !cats[cat] {
				continue
			}
			for _, re := range rules[cat][strength] {
				if m := re.FindStringSubmatch(s); m != nil {
					return Hit{Category: cat, Strength: strength, Match: strings.TrimSpace(m[1])}, true
				}
			}
		}
	}
	return Hit{}, false
}

// Normalize lowercases text, unifies apostrophes and quote marks (including
// the Hebrew geresh/gershayim) and collapses whitespace.
func Normalize(text string) string {
	text = strings.NewReplacer(
		"’", "'", "‘", "'", "׳", "'", "`", "'",
		"“", "\"", "”", "\"", "״", "\"",
	).Replace(strings.ToLower(text))
	return strings.Join(strings.Fields(text), " ")
}

// ─── rules ───────────────────────────────────────────────────

// Patterns are RE2 fragments matched as whole words: English ones need a
// non-letter (or the text edge) on both sides; Hebrew ones may carry the
// usual one- or two-letter prefixes (ו ה ב ל מ ש כ).
type ruleSet struct{ strongEN, strongHE, weakEN, weakHE []string }

const (
	you   = `(?:you|u|ya)`
	are   = `(?:are|r)`
	bots  = `(?:bot|robot|ai|a\.i\.?|chat ?bot|chatgpt|chat gpt|gpt|machine|computer|program)`
	money = `(?:money|cash|dough|funds|\$ ?\d[\d,.]*k?|\d[\d,.]*k? ?(?:\$|dollars?|bucks|usd|eur|euros?|€|£|pounds?|shekels?|nis|ils|₪))`
)

var ruleSets = map[string]ruleSet{
	model.HandoffBot: {
		strongEN: []string{
			are + ` ` + you + ` (?:even |actually |really |just )?(?:an? )?(?:real )?` + bots,
			are + ` ` + you + ` (?:even |actually |really )?(?:an? )?(?:human|real person|real human|actual person)`,
			`(?:talking|chatting|texting|speaking) (?:to|with) (?:an? )?` + bots,
			`(?:is|was) (?:this|that|it) (?:an? )?(?:` + bots + `|automated|auto.?reply|auto.?response)`,
			you + ` (?:sound|talk|text|write|type|reply) like (?:an? )?` + bots,
			`(?:is|it'?s) (?:this|that) (?:really|actually|even) ` + you,
			`who am i (?:really |actually )?(?:talking|texting|chatting|speaking) (?:to|with)`,
			`(?:using|use|uses) (?:chatgpt|gpt|ai|an ai|a bot) to (?:reply|answer|text|write|respond)`,
			`(?:chatgpt|gpt|an ai|a bot) (?:wrote|is writing|writes|is answering|is replying)`,
		},
		strongHE: []string{
			`(?:אתה|את|זה|זאת) (?:בכלל |באמת )?(?:בוט|רובוט|בינה מלאכותית|מכונה|צ'?אט ?ג'?י?פי?טי)`,
			`(?:מדבר|מדברת|מתכתב|מתכתבת|מדברים) (?:עם|מול) (?:בוט|רובוט|מכונה|בינה מלאכותית)`,
			`(?:אתה|את) (?:בכלל |באמת )?(?:בן אדם|בת אדם|אדם אמיתי)`,
			`זה (?:באמת|בכלל) (?:אתה|את)`,
			`מי (?:כותב|כותבת|עונה) (?:לי )?(?:פה|כאן|את זה|בשבילך)`,
		},
		weakEN: []string{
			`bots?`, `robots?`, `ai`, `a\.i\.?`, `chat ?bots?`, `chatgpt`, `chat gpt`, `gpt`,
			`automated`, `auto.?reply`, `real person`, are + ` ` + you + ` (?:even |actually )?real`, `human`,
			`who is this`, `who'?s this`,
		},
		weakHE: []string{
			`בוט`, `רובוט`, `בינה מלאכותית`, `צ'?אט ?ג'?י?פי?טי`, `(?:אתה|את) (?:אמיתי|אמיתית)`, `מי זה`,
		},
	},
	model.HandoffMoney: {
		strongEN: []string{
			`(?:send|transfer|wire|lend|loan|give|spot|front|venmo|zelle|paypal|cash ?app) (?:me|us)(?: back)? (?:some |the |a few |a little |like )?` + money,
			`(?:can|could|would|will) ` + you + ` (?:please )?(?:lend|loan|spot|front|send|transfer|wire) (?:me|us)`,
			`(?:venmo|zelle|paypal|revolut|cash ?app) (?:me|it)`,
			`iban`, `swift code`, `routing number`, `account number`, `bank (?:account|details|transfer|info)`,
			`credit card`, `card (?:number|details)`, `cvv`, `gift ?cards?`, `western union`, `moneygram`, `wallet address`,
		},
		strongHE: []string{
			`(?:תעביר|תעבירי|תשלח|תשלחי|תן|תני|תביא|תביאי|תלווה|תלווי|תפקיד|תפקידי) (?:לי|לנו) (?:קצת |את ה)?(?:כסף|מזומן|\d+|שקל|שקלים|דולר|דולרים)`,
			`(?:להלוות|תלווה|תלווי|תוכל להלוות|תוכלי להלוות) (?:לי|לנו)`,
			`חשבון (?:ה)?בנק`, `מספר (?:ה)?חשבון`, `העברה בנקאית`, `כרטיס (?:ה)?אשראי`, `פרטי (?:ה)?כרטיס`,
			`ביט`, `פייבוקס`, `paybox`, `(?:אתה|את) חייב(?:ת)? לי`,
		},
		weakEN: []string{
			`money`, `cash`, `pay`, `paid`, `paying`, `payment`, `owe`, `owes`, `debt`, `loan`, `lend`,
			`dollars?`, `bucks`, `broke`, `bitcoin`, `crypto`, `venmo`, `paypal`, `zelle`, `\$ ?\d[\d,.]*`, `\d[\d,.]* ?(?:\$|€|£|₪|nis|shekels?)`,
		},
		weakHE: []string{
			`כסף`, `חוב`, `חובות`, `לשלם`, `תשלום`, `שילמת`, `הלוואה`, `שקלים`, `מזומן`, `חייב לך`, `חייבת לך`,
		},
	},
	model.HandoffHealth: {
		strongEN: []string{
			`hospital`, `hospitali[sz]ed`, `ambulance`, `emergency room`, `icu`, `intensive care`,
			`heart attack`, `(?:had|having) a stroke`, `seizures?`, `overdosed?`, `can'?t breathe`, `chest pains?`,
			`collapsed`, `unconscious`, `diagnosed`, `cancer`, `tumou?r`, `chemo(?:therapy)?`, `surgery`,
			`miscarriage`, `broke my (?:arm|leg|wrist|ankle|hip|back|neck)`, `in labou?r`, `car (?:crash|accident)`,
			`(?:was|were|got|been) in an accident`, `call(?:ed|ing)? (?:an )?(?:ambulance|911|999|112|101)`,
		},
		strongHE: []string{
			`בית (?:ה)?חולים`, `אמבולנס`, `מד"א`, `חדר מיון`, `טיפול נמרץ`, `התקף לב`, `שבץ`, `ניתוח`, `מנותח(?:ת)?`,
			`סרטן`, `כימותרפיה`, `אושפז(?:ה|תי)?`, `מאושפז(?:ת)?`, `אשפוז`, `תאונ(?:ה|ת דרכים)`, `לא (?:מצליח|מצליחה) לנשום`,
			`הפלה`, `בלידה`,
		},
		weakEN: []string{
			`sick`, `doctor`, `pain`, `hurts?`, `hurting`, `fever`, `injur(?:ed|y)`, `covid`, `flu`, `pregnant`,
			`medicine`, `meds`, `pills`, `clinic`, `vomit(?:ing)?`, `throwing up`, `migraine`, `bleeding`,
		},
		weakHE: []string{
			`חולה`, `רופא(?:ה)?`, `כואב(?:ת)?`, `כאב(?:ים)?`, `חום`, `תרופ(?:ה|ות)`, `קופת חולים`, `בהריון`, `מיון`,
			`הקאתי`, `מקיא(?:ה)?`, `נפצע(?:תי|ה)?`,
		},
	},
	model.HandoffMeeting: {
		strongEN: []string{
			`(?:let'?s|lets|wanna|want to|should we|can we|could we|shall we|we should|why don'?t we|how about we) (?:meet|hang ?out|meet ?up|see each other|get together|grab (?:a |some )?(?:coffee|drinks?|lunch|dinner|beers?|food|a bite)|get (?:coffee|drinks?|lunch|dinner|food))`,
			`come over`, `coming over`, `pick ` + you + ` up`, `picking ` + you + ` up`,
			`(?:send|share|drop) (?:me )?(?:your |the )?(?:location|address|pin)`, `what'?s your address`,
			`where do ` + you + ` live`, `(?:i'?m|im|i am) (?:outside|downstairs|at your (?:place|door|house|building))`,
			`(?:when|where) (?:should|shall|can|do|will) we meet`,
		},
		strongHE: []string{
			`(?:בוא|בואי|בואו) (?:נפגש|ניפגש|נשב|נצא|לקפה|לשבת|נראה)`, `(?:נפגש|ניפגש|להיפגש) (?:מחר|היום|הערב|בערב|בשבוע|בסופ"ש|ב\d)`,
			`(?:מתי|איפה) (?:נפגשים|ניפגש|נפגש)`, `(?:תבוא|תבואי|תקפוץ|תקפצי) (?:אליי|אלי)`,
			`(?:שלח|שלחי|תשלח|תשלחי) (?:לי )?(?:מיקום|כתובת|לוקיישן)`, `מה הכתובת`, `איפה (?:אתה גר|את גרה)`, `אאסוף אותך`,
			`אני (?:למטה|בחוץ|ליד הבית שלך)`,
		},
		weakEN: []string{
			`meet`, `meeting up`, `hang ?out`, `address`, `location`, `(?:tomorrow|tonight|today) at \d`,
			`see ` + you + ` (?:at|there|tonight|tomorrow|later)`, are + ` ` + you + ` free`,
		},
		weakHE: []string{
			`נפגש`, `ניפגש`, `להיפגש`, `פגישה`, `כתובת`, `מיקום`, `(?:מחר|הערב|היום) ב\d`, `(?:אתה|את) (?:פנוי|פנויה)`,
		},
	},
	model.HandoffDistress: {
		strongEN: []string{
			`kill(?:ing)? myself`, `end (?:it all|my life)`, `suicid(?:e|al)`, `self.?harm`, `cut(?:ting)? myself`,
			`hurt(?:ing)? myself`, `(?:want|wanna|going) to die`, `don'?t want to (?:live|be alive|be here anymore|wake up)`,
			`no (?:reason|point) (?:to live|in living)`, `can'?t go on`, `better off (?:dead|without me)`, `panic attack`,
			`(?:he|she|they) (?:hit|hits|beat|beats|hurt|hurts) me`, `being abused`, `abusing me`, `(?:i'?m|im|i am) not safe`,
		},
		strongHE: []string{
			`להתאבד`, `אתאבד`, `התאבדות`, `לא רוצה (?:לחיות|להיות פה|להתעורר)`, `רוצה למות`, `לפגוע בעצמי`, `פוגע(?:ת)? בעצמי`,
			`אין לי בשביל מה לחיות`, `לגמור עם (?:זה|הכל)`, `התקף (?:חרדה|פאניקה)`, `(?:הוא|היא) (?:מרביץ|מרביצה|הרביץ|הרביצה) לי`,
			`מתעלל(?:ת)?`, `לא (?:בטוח|בטוחה) בבית`,
		},
		weakEN: []string{
			`sad`, `crying`, `cried`, `depress(?:ed|ion|ing)`, `anxious`, `anxiety`, `lonely`, `hopeless`, `worthless`,
			`can'?t take (?:it|this)(?: anymore)?`, `can'?t do this anymore`, `overwhelmed`, `breakdown`, `falling apart`,
			`heartbroken`, `miserable`,
		},
		weakHE: []string{
			`עצוב(?:ה)?`, `בוכה`, `בכיתי`, `(?:ב)?דיכאון`, `חרדה`, `בודד(?:ה)?`, `אין לי כוח(?: יותר)?`, `נשבר(?:תי|ה)?`, `לא עומד(?:ת)? בזה`,
		},
	},
	model.HandoffLegal: {
		strongEN: []string{
			`lawyers?`, `attorney`, `sue (?:you|u|me|him|her|them|us)`, `suing`, `lawsuit`, `legal action`, `police`, `cops`,
			`arrested`, `subpoena`, `restraining order`, `(?:take|taking) ` + you + ` to court`, `in court`, `press charges`,
			`report (?:you|u) to`, `warrant`, `cease and desist`,
		},
		strongHE: []string{
			`עורך דין`, `עורכת דין`, `עו"ד`, `משטר(?:ה|ת)`, `שוטר(?:ים|ת)?`, `תביעה`, `לתבוע`, `אתבע`, `תובע(?:ת)? אותך`,
			`בית (?:ה)?משפט`, `נעצר(?:תי|ה)?`, `מעצר`, `צו הרחקה`, `חקירה`,
		},
		weakEN: []string{
			`legal`, `illegal`, `contract`, `court`, `judge`, `nda`, `sue`,
		},
		weakHE: []string{
			`חוזה`, `משפטי`, `לא חוקי`, `קנס`,
		},
	},
}

// rules are the compiled patterns per category and strength.
var rules = compile()

const (
	edgeEN   = `(?:^|[^\p{L}\p{N}_])`
	edgeEnd  = `(?:$|[^\p{L}\p{N}_])`
	prefixHE = `(?:[והבלמשכ]{1,2})?`
)

func compile() map[string]map[string][]*regexp.Regexp {
	out := map[string]map[string][]*regexp.Regexp{}
	for cat, rs := range ruleSets {
		m := map[string][]*regexp.Regexp{}
		add := func(strength string, pats []string, he bool) {
			for _, p := range pats {
				pre := edgeEN
				if he {
					pre += prefixHE
				}
				m[strength] = append(m[strength], regexp.MustCompile(pre+`(`+p+`)`+edgeEnd))
			}
		}
		add(Strong, rs.strongEN, false)
		add(Strong, rs.strongHE, true)
		add(Weak, rs.weakEN, false)
		add(Weak, rs.weakHE, true)
		out[cat] = m
	}
	return out
}

// Label is a short plain-English name for a category ("Money").
func Label(cat string) string {
	switch cat {
	case model.HandoffMoney:
		return "Money"
	case model.HandoffHealth:
		return "Health"
	case model.HandoffMeeting:
		return "Meeting up"
	case model.HandoffDistress:
		return "Someone struggling"
	case model.HandoffBot:
		return "Is this a bot?"
	case model.HandoffLegal:
		return "Legal"
	}
	return cat
}

// Reason describes why who's message paused the chat, for the activity feed
// ("Dana asked if they're talking to a bot").
func Reason(cat, who string) string {
	if who == "" {
		who = "Someone"
	}
	switch cat {
	case model.HandoffMoney:
		return who + " brought up money"
	case model.HandoffHealth:
		return who + " mentioned something about health"
	case model.HandoffMeeting:
		return who + " wants to meet up"
	case model.HandoffDistress:
		return who + " may be going through something hard"
	case model.HandoffBot:
		return who + " asked if they're talking to a bot"
	case model.HandoffLegal:
		return who + " mentioned something legal"
	}
	return who + " wrote something sensitive"
}
