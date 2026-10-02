package behavior

import "whatsappdoppel/internal/model"

// LegacyReplies is the v1 config.json "replies" block.
type LegacyReplies struct {
	DebounceSeconds         int    `json:"debounceSeconds"`
	BurstCapSeconds         int    `json:"burstCapSeconds"`
	TypingIndicator         bool   `json:"typingIndicator"`
	IgnoreLinks             bool   `json:"ignoreLinks"`
	GroupRandomReplyPercent int    `json:"groupRandomReplyPercent"`
	TriggerPrefix           string `json:"triggerPrefix"`
	MaxHistoryMessages      int    `json:"maxHistoryMessages"`
	MaxHistoryChars         int    `json:"maxHistoryChars"`
	InjectionFilter         string `json:"injectionFilter"` // strict|balanced|off
}

// LegacyApprovals is the v1 config.json "approvals" block.
type LegacyApprovals struct {
	AutoSendSeconds int `json:"autoSendSeconds"`
}

// LegacyDefaults are the v1 defaults (used for blocks that were left empty).
func LegacyDefaults() LegacyReplies {
	return LegacyReplies{
		DebounceSeconds:         9,
		BurstCapSeconds:         25,
		TypingIndicator:         true,
		IgnoreLinks:             true,
		GroupRandomReplyPercent: 40,
		TriggerPrefix:           "1",
		MaxHistoryMessages:      40,
		MaxHistoryChars:         12000,
		InjectionFilter:         "balanced",
	}
}

// MigrateLegacy maps v1 settings onto the Natural profiles, field for field:
// debounceSeconds→waitForMoreSec, burstCapSeconds→burstCapSec, typingIndicator,
// ignoreLinks, maxHistoryMessages→historyMessages, maxHistoryChars→historyChars,
// injectionFilter (both kinds), groupRandomReplyPercent→group.chimeInPercent and
// approvals.autoSendSeconds→autoSendSeconds (both). Everything new keeps the
// Natural value. Profiles are normalised, so they read "custom" when the old
// values differ from Natural.
func MigrateLegacy(r LegacyReplies, a LegacyApprovals) (private, group model.BehaviorProfile) {
	d := LegacyDefaults()
	if r.DebounceSeconds == 0 && r.BurstCapSeconds == 0 && r.MaxHistoryMessages == 0 {
		// An empty block meant "defaults" in v1.
		prefix := r.TriggerPrefix
		r = d
		if prefix != "" {
			r.TriggerPrefix = prefix
		}
	}
	if r.MaxHistoryMessages == 0 {
		r.MaxHistoryMessages = d.MaxHistoryMessages
	}
	if r.MaxHistoryChars == 0 {
		r.MaxHistoryChars = d.MaxHistoryChars
	}
	if r.InjectionFilter == "" {
		r.InjectionFilter = d.InjectionFilter
	}
	apply := func(p *model.BehaviorProfile) {
		p.WaitForMoreSec = r.DebounceSeconds
		p.BurstCapSec = r.BurstCapSeconds
		p.TypingIndicator = r.TypingIndicator
		p.IgnoreLinks = r.IgnoreLinks
		p.HistoryMessages = r.MaxHistoryMessages
		p.HistoryChars = r.MaxHistoryChars
		p.InjectionFilter = r.InjectionFilter
		p.AutoSendSeconds = a.AutoSendSeconds
	}
	private, group = Default(KindDM), Default(KindGroup)
	apply(&private)
	apply(&group)
	group.ChimeInPercent = r.GroupRandomReplyPercent
	Normalize(&private, KindDM)
	Normalize(&group, KindGroup)
	return private, group
}

// MigrateChatOverrides converts v1 per-chat overrides: debounceSeconds →
// waitForMoreSec, groupRandomReplyPercent → chimeInPercent and
// alwaysReplyInGroup → chimeInPercent 100 + aiJudgement off. Existing
// behaviour overrides win over legacy values.
func MigrateChatOverrides(o model.ChatOverrides, cur model.BehaviorOverrides) model.BehaviorOverrides {
	if o.DebounceSeconds != nil && cur.WaitForMoreSec == nil {
		v := min(max(*o.DebounceSeconds, ranges["waitForMoreSec"].Min), ranges["waitForMoreSec"].Max)
		cur.WaitForMoreSec = &v
	}
	if o.GroupRandomReplyPercent != nil && cur.ChimeInPercent == nil {
		v := min(max(*o.GroupRandomReplyPercent, 0), 100)
		cur.ChimeInPercent = &v
	}
	if o.AlwaysReplyInGroup {
		hundred, off := 100, false
		cur.ChimeInPercent = &hundred
		if cur.AIJudgement == nil {
			cur.AIJudgement = &off
		}
	}
	return cur
}
