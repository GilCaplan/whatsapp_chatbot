package wa

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
	"unicode"

	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"

	"whatsappdoppel/internal/model"
)

// SelfChatName is the display name of the "message yourself" chat.
const SelfChatName = "You (message yourself)"

// Chat key prefixes (see model.ChatAssignment.Key).
const (
	prefixDM    = "dm:"
	prefixGroup = "group:"
	prefixLID   = "lid:"
)

var errBadJID = errors.New("not a valid WhatsApp address")

// selfIDs identifies the linked account (both non-AD); empty when not linked.
type selfIDs struct {
	PN, LID types.JID
}

func (s selfIDs) linked() bool { return s.PN.User != "" }

// resolver canonicalizes user addresses. The lookups may be nil (no LID map
// available) and must return an empty JID when the mapping is unknown.
type resolver struct {
	self     selfIDs
	pnForLID func(types.JID) types.JID
	lidForPN func(types.JID) types.JID
}

// dm canonicalizes a user JID (phone or lid) into a chat key plus the phone
// and lid addresses that are known. hint is an alternate address supplied by
// the message (SenderAlt/RecipientAlt) and may be empty.
func (r resolver) dm(jid, hint types.JID) (key string, pn, lid types.JID) {
	jid, hint = jid.ToNonAD(), hint.ToNonAD()
	switch jid.Server {
	case types.DefaultUserServer:
		pn = jid
		switch {
		case hint.Server == types.HiddenUserServer:
			lid = hint
		case r.self.linked() && pn.User == r.self.PN.User && r.self.LID.User != "":
			lid = r.self.LID
		case r.lidForPN != nil:
			lid = r.lidForPN(pn).ToNonAD()
		}
	case types.HiddenUserServer:
		lid = jid
		switch {
		case hint.Server == types.DefaultUserServer:
			pn = hint
		case r.self.LID.User != "" && lid.User == r.self.LID.User:
			pn = r.self.PN
		case r.pnForLID != nil:
			pn = r.pnForLID(lid).ToNonAD()
		}
	}
	if pn.User != "" {
		return prefixDM + pn.User, pn, lid
	}
	return prefixLID + lid.User, pn, lid
}

// isUserServer reports whether jid addresses a person (phone number or lid).
func isUserServer(jid types.JID) bool {
	return jid.Server == types.DefaultUserServer || jid.Server == types.HiddenUserServer
}

// skippedChat reports chats the bot never handles: status, broadcast lists, channels.
func skippedChat(chat types.JID) bool {
	return chat == types.StatusBroadcastJID || chat.Server == types.BroadcastServer || chat.Server == types.NewsletterServer
}

// messageText extracts the user-visible text of a message ("" when none).
func messageText(msg *waE2E.Message) string {
	text, media := messageContent(msg)
	if media != "" && !strings.Contains(text, "] ") {
		return "" // media without a caption: see messageContent
	}
	return text
}

// messageContent returns a message's text and its media kind (wave 3,
// model.Incoming.Media): a photo or video keeps its caption behind
// "[photo] "/"[video] "; without one — and for voice notes, audio and
// stickers — the text is a placeholder like "[photo]" so the persona (and
// photo missions) can react to it.
func messageContent(msg *waE2E.Message) (text, media string) {
	if t := msg.GetConversation(); strings.TrimSpace(t) != "" {
		return t, ""
	}
	if t := msg.GetExtendedTextMessage().GetText(); strings.TrimSpace(t) != "" {
		return t, ""
	}
	if im := msg.GetImageMessage(); im != nil {
		if c := im.GetCaption(); strings.TrimSpace(c) != "" {
			return "[photo] " + c, model.MediaImage
		}
		return "[photo]", model.MediaImage
	}
	if v := msg.GetVideoMessage(); v != nil {
		if c := v.GetCaption(); strings.TrimSpace(c) != "" {
			return "[video] " + c, model.MediaVideo
		}
		return "[video]", model.MediaVideo
	}
	if a := msg.GetAudioMessage(); a != nil {
		if a.GetPTT() {
			return "[voice note]", model.MediaAudio
		}
		return "[audio]", model.MediaAudio
	}
	if msg.GetStickerMessage() != nil {
		return "[sticker]", model.MediaSticker
	}
	return "", ""
}

// messageMentions returns the JIDs mentioned in the message's context info.
func messageMentions(msg *waE2E.Message) []string {
	for _, ci := range []*waE2E.ContextInfo{
		msg.GetExtendedTextMessage().GetContextInfo(),
		msg.GetImageMessage().GetContextInfo(),
		msg.GetVideoMessage().GetContextInfo(),
	} {
		if m := ci.GetMentionedJID(); len(m) > 0 {
			return append([]string(nil), m...)
		}
	}
	return nil
}

// normalizeMessage turns a whatsmeow message into a model.Incoming.
// ok is false for messages the engine should never see.
func normalizeMessage(evt *events.Message, r resolver) (in model.Incoming, ok bool) {
	if evt == nil || evt.Message == nil {
		return in, false
	}
	info, msg := evt.Info, evt.Message
	chat := info.Chat.ToNonAD()
	if skippedChat(chat) || evt.IsEdit || info.Edit != "" ||
		msg.GetReactionMessage() != nil || msg.GetEncReactionMessage() != nil || msg.GetProtocolMessage() != nil {
		return in, false
	}
	text, media := messageContent(msg)
	if text == "" {
		return in, false
	}
	in = model.Incoming{
		IsFromMe:      info.IsFromMe,
		SenderJID:     info.Sender.ToNonAD().String(),
		PushName:      info.PushName,
		Text:          text,
		MentionedJIDs: messageMentions(msg),
		Timestamp:     info.Timestamp,
		MessageID:     string(info.ID),
		Media:         media,
		ChatJID:       chat.String(),
	}
	switch {
	case chat.Server == types.GroupServer:
		in.IsGroup = true
		in.ChatKey = prefixGroup + chat.User
	case isUserServer(chat):
		hint := info.SenderAlt
		if info.IsFromMe {
			hint = info.RecipientAlt
		}
		key, pn, lid := r.dm(chat, hint)
		in.ChatKey = key
		if chat.Server == types.HiddenUserServer {
			in.AltJID = jidString(pn)
		} else {
			in.AltJID = jidString(lid)
		}
	default:
		return in, false
	}
	return in, true
}

func jidString(j types.JID) string {
	if j.User == "" {
		return ""
	}
	return j.String()
}

// digitsOnly strips everything but ASCII digits.
func digitsOnly(s string) string {
	var b strings.Builder
	for _, r := range s {
		if r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// looksLikePhone accepts "+972 50-123 4567"-style input.
func looksLikePhone(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if !unicode.IsDigit(r) && !strings.ContainsRune("+-() .", r) {
			return false
		}
	}
	return len(digitsOnly(s)) >= 5
}

// parseTarget accepts a JID ("…@s.whatsapp.net", "…@lid", "…@g.us"), a chat
// key ("dm:…", "group:…", "lid:…") or bare phone digits, and returns a non-AD JID.
func parseTarget(s string) (types.JID, error) {
	s = strings.TrimSpace(s)
	switch {
	case s == "":
		return types.JID{}, errBadJID
	case strings.HasPrefix(s, prefixDM):
		if d := digitsOnly(s[len(prefixDM):]); d != "" {
			return types.NewJID(d, types.DefaultUserServer), nil
		}
	case strings.HasPrefix(s, prefixGroup):
		if id := s[len(prefixGroup):]; id != "" && !strings.Contains(id, "@") {
			return types.NewJID(id, types.GroupServer), nil
		}
	case strings.HasPrefix(s, prefixLID):
		if id := s[len(prefixLID):]; id != "" && !strings.Contains(id, "@") {
			return types.NewJID(id, types.HiddenUserServer), nil
		}
	case looksLikePhone(s):
		return types.NewJID(digitsOnly(s), types.DefaultUserServer), nil
	case strings.Contains(s, "@"):
		j, err := types.ParseJID(s)
		if err != nil || j.User == "" {
			return types.JID{}, fmt.Errorf("%w: %q", errBadJID, s)
		}
		return j.ToNonAD(), nil
	}
	return types.JID{}, fmt.Errorf("%w: %q", errBadJID, s)
}

// phoneName is the fallback display name for a number.
func phoneName(phone string) string { return "+" + phone }

// selfItem is the "message yourself" chat for the linked account.
func selfItem(self selfIDs) model.ChatItem {
	return model.ChatItem{
		Key:    prefixDM + self.PN.User,
		Kind:   "dm",
		JID:    self.PN.String(),
		AltJID: jidString(self.LID),
		Name:   SelfChatName,
		Phone:  self.PN.User,
		IsSelf: true,
	}
}

// dmItem builds a ChatItem for a canonicalized DM.
func dmItem(key string, pn, lid types.JID, name string, self selfIDs) model.ChatItem {
	it := model.ChatItem{Key: key, Kind: "dm", Name: name}
	if pn.User != "" {
		it.JID, it.AltJID, it.Phone = pn.String(), jidString(lid), pn.User
		it.IsSelf = self.linked() && pn.User == self.PN.User
		if it.Name == "" {
			it.Name = phoneName(pn.User)
		}
	} else {
		it.JID = lid.String()
		if it.Name == "" {
			it.Name = "Unknown contact"
		}
	}
	return it
}

// filterChats applies kind/q filtering and paging. pinned items come first,
// ignore q (but respect kind) and count toward total.
func filterChats(pinned, items []model.ChatItem, q model.ChatQuery) ([]model.ChatItem, int) {
	kind := strings.ToLower(q.Kind)
	if kind == "all" {
		kind = ""
	}
	needle := strings.ToLower(strings.TrimSpace(q.Q))
	digits := ""
	if looksLikePhone(needle) {
		digits = digitsOnly(needle)
	}
	matched := make([]model.ChatItem, 0, len(pinned)+min(len(items), 256))
	seen := make(map[string]bool, len(pinned))
	for _, it := range pinned {
		if kind == "" || it.Kind == kind {
			matched = append(matched, it)
			seen[it.Key] = true
		}
	}
	for _, it := range items {
		if seen[it.Key] || (kind != "" && it.Kind != kind) {
			continue
		}
		if needle != "" && !strings.Contains(strings.ToLower(it.Name), needle) &&
			!(digits != "" && it.Phone != "" && strings.Contains(it.Phone, digits)) {
			continue
		}
		matched = append(matched, it)
	}
	total := len(matched)
	limit := q.Limit
	if limit <= 0 {
		limit = 50
	}
	limit = min(limit, 500)
	off := max(q.Offset, 0)
	if off >= total {
		return []model.ChatItem{}, total
	}
	return matched[off:min(off+limit, total)], total
}

// sortByName orders named chats alphabetically, number-only names last.
func sortByName(items []model.ChatItem) {
	sort.SliceStable(items, func(i, j int) bool {
		a, b := items[i], items[j]
		an, bn := strings.HasPrefix(a.Name, "+"), strings.HasPrefix(b.Name, "+")
		if an != bn {
			return !an
		}
		al, bl := strings.ToLower(a.Name), strings.ToLower(b.Name)
		if al != bl {
			return al < bl
		}
		return a.Key < b.Key
	})
}

// sortByRecent orders chats newest first (unknown timestamps last).
func sortByRecent(items []model.ChatItem) {
	sort.SliceStable(items, func(i, j int) bool {
		a, b := items[i].LastMessageAt, items[j].LastMessageAt
		switch {
		case a == nil:
			return false
		case b == nil:
			return true
		}
		return a.After(*b)
	})
}

func timePtr(t time.Time) *time.Time {
	if t.IsZero() {
		return nil
	}
	return &t
}
