// Package store persists personas, chat assignments, pending approvals
// (JSON files) and per-chat history (JSON lines) in the data directory.
package store

import (
	"bufio"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"sync"
	"time"

	"whatsappdoppel/internal/behavior"
	"whatsappdoppel/internal/config"
	"whatsappdoppel/internal/model"
	"whatsappdoppel/internal/world"
)

var ErrNotFound = errors.New("not found")

type Store struct {
	paths config.Paths

	mu        sync.RWMutex
	personas  []model.Persona
	chats     []model.ChatAssignment
	approvals []model.PendingReply

	histMu sync.Mutex // serializes history file I/O

	rt runtimeState // runtime.json
}

// Open loads all JSON files. If personas.json does not exist it is created from seeds.
func Open(p config.Paths, seeds []model.Persona) (*Store, error) {
	s := &Store{paths: p}
	seeded := false
	if err := readJSON(p.PersonasFile(), &s.personas); err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("personas.json: %w", err)
		}
		now := time.Now()
		for _, sp := range seeds {
			sp.CreatedAt, sp.UpdatedAt = now, now
			s.personas = append(s.personas, sp)
		}
		seeded = true
	}
	for i := range s.personas {
		world.Normalize(&s.personas[i].World) // wave 3: older files have no world (routine null)
	}
	if err := readJSON(p.ChatsFile(), &s.chats); err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("chats.json: %w", err)
	}
	if err := readJSON(p.ApprovalsFile(), &s.approvals); err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("approvals.json: %w", err)
	}
	if seeded {
		if err := s.savePersonas(); err != nil {
			return nil, err
		}
	}
	if s.migrateChats() {
		if err := s.saveChats(); err != nil {
			return nil, err
		}
	}
	if err := s.loadRuntime(); err != nil {
		return nil, err
	}
	return s, nil
}

// migrateChats converts v1 per-chat "overrides" into "behavior" overrides.
// Reports whether anything changed (chats.json is then rewritten once).
// V3BehaviorFields mirrors config.V3Fields (the behaviour fields added in settings v3).
var V3BehaviorFields = config.V3Fields

// V4BehaviorFields mirrors config.V4Fields (the behaviour fields added in settings v4).
var V4BehaviorFields = config.V4Fields

func (s *Store) migrateChats() bool {
	changed := false
	for i := range s.chats {
		c := &s.chats[i]
		// Settings v3 added behaviour fields: a chat that applied a whole
		// named preset gets that preset's values for them.
		if behavior.FillOverridesFromPreset(&c.Behavior, behavior.KindOf(*c), V3BehaviorFields...) {
			changed = true
		}
		if behavior.FillOverridesFromPreset(&c.Behavior, behavior.KindOf(*c), V4BehaviorFields...) {
			// A pre-v4 chat holding a whole Busy/Slow preset moves to the
			// tweaked numbers once, so it keeps its label (vibe dials).
			behavior.RelabelLegacyOverrides(&c.Behavior, behavior.KindOf(*c))
			changed = true
		}
		// v4: mode replaces approvalMode (which stays, derived).
		before := *c
		syncMode(nil, c)
		if c.Mode != before.Mode || c.ApprovalMode != before.ApprovalMode {
			changed = true
		}
		if c.LegacyOverrides == nil {
			continue
		}
		c.Behavior = behavior.MigrateChatOverrides(*c.LegacyOverrides, c.Behavior)
		c.LegacyOverrides = nil
		changed = true
	}
	return changed
}

// NewID returns a random 16-hex-char identifier.
func NewID() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

var slugRe = regexp.MustCompile(`[^a-z0-9]+`)

// Slug makes a URL-safe id from a name ("Chad The Shred" → "chad-the-shred").
func Slug(name string) string {
	s := strings.Trim(slugRe.ReplaceAllString(strings.ToLower(name), "-"), "-")
	if len(s) > 32 {
		s = strings.Trim(s[:32], "-")
	}
	return s
}

// ─── Personas ────────────────────────────────────────────────

func (s *Store) Personas() []model.Persona {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return slices.Clone(s.personas)
}

func (s *Store) Persona(id string) (model.Persona, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, p := range s.personas {
		if p.ID == id {
			return p, true
		}
	}
	return model.Persona{}, false
}

// UpsertPersona saves p. An empty ID gets a unique slug-based ID. Returns the stored persona.
func (s *Store) UpsertPersona(p model.Persona) (model.Persona, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now()
	p.UpdatedAt = now
	world.Normalize(&p.World)
	if p.ID == "" {
		base := Slug(p.Name)
		if base == "" {
			base = "persona"
		}
		id := base
		for i := 2; s.personaIndex(id) >= 0; i++ {
			id = fmt.Sprintf("%s-%d", base, i)
		}
		p.ID = id
	}
	if i := s.personaIndex(p.ID); i >= 0 {
		p.CreatedAt = s.personas[i].CreatedAt
		s.personas[i] = p
	} else {
		if p.CreatedAt.IsZero() {
			p.CreatedAt = now
		}
		s.personas = append(s.personas, p)
	}
	return p, s.savePersonas()
}

func (s *Store) DeletePersona(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	i := s.personaIndex(id)
	if i < 0 {
		return ErrNotFound
	}
	s.personas = slices.Delete(s.personas, i, i+1)
	return s.savePersonas()
}

func (s *Store) personaIndex(id string) int {
	return slices.IndexFunc(s.personas, func(p model.Persona) bool { return p.ID == id })
}

func (s *Store) savePersonas() error {
	return config.WriteJSONAtomic(s.paths.PersonasFile(), nonNil(s.personas), 0o600)
}

// AvatarPath is where an uploaded persona picture is stored (always PNG).
func (s *Store) AvatarPath(personaID string) string {
	return filepath.Join(s.paths.AvatarsDir(), Slug(personaID)+".png")
}

// ─── Chats ───────────────────────────────────────────────────

func (s *Store) Chats() []model.ChatAssignment {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return slices.Clone(s.chats)
}

func (s *Store) Chat(key string) (model.ChatAssignment, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if i := s.chatIndex(key); i >= 0 {
		return s.chats[i], true
	}
	return model.ChatAssignment{}, false
}

// ChatByJID finds an assignment whose JID or AltJID equals jid.
func (s *Store) ChatByJID(jid string) (model.ChatAssignment, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, c := range s.chats {
		if c.JID == jid || (c.AltJID != "" && c.AltJID == jid) {
			return c, true
		}
	}
	return model.ChatAssignment{}, false
}

// syncMode keeps Mode and the legacy ApprovalMode consistent. prev is the
// stored assignment before this change (nil for a new or loaded one): when
// only approvalMode changed, the mode follows it; otherwise the mode wins.
// An empty or unknown mode is derived from approvalMode.
func syncMode(prev, c *model.ChatAssignment) {
	switch {
	case !model.ValidChatMode(c.Mode):
		c.Mode = model.ModeFromApproval(c.ApprovalMode)
	case prev != nil && c.Mode == prev.Mode && c.ApprovalMode != prev.ApprovalMode:
		c.Mode = model.ModeFromApproval(c.ApprovalMode)
	}
	c.ApprovalMode = c.Mode != model.ChatModeAuto
}

func (s *Store) UpsertChat(c model.ChatAssignment) (model.ChatAssignment, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if c.Key == "" {
		return c, errors.New("chat key required")
	}
	if i := s.chatIndex(c.Key); i >= 0 {
		syncMode(&s.chats[i], &c)
		c.CreatedAt = s.chats[i].CreatedAt
		s.chats[i] = c
	} else {
		syncMode(nil, &c)
		if c.CreatedAt.IsZero() {
			c.CreatedAt = time.Now()
		}
		s.chats = append(s.chats, c)
	}
	return c, s.saveChats()
}

// UpdateChat applies fn to the stored assignment under the lock.
func (s *Store) UpdateChat(key string, fn func(*model.ChatAssignment)) (model.ChatAssignment, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	i := s.chatIndex(key)
	if i < 0 {
		return model.ChatAssignment{}, ErrNotFound
	}
	c := s.chats[i]
	fn(&c)
	c.Key = s.chats[i].Key
	syncMode(&s.chats[i], &c)
	s.chats[i] = c
	return c, s.saveChats()
}

// TouchChat records activity time without rewriting other fields. Errors are ignored.
func (s *Store) TouchChat(key string, t time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if i := s.chatIndex(key); i >= 0 {
		s.chats[i].LastActivityAt = &t
		_ = s.saveChats()
	}
}

func (s *Store) DeleteChat(key string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	i := s.chatIndex(key)
	if i < 0 {
		return ErrNotFound
	}
	s.chats = slices.Delete(s.chats, i, i+1)
	s.DeleteRunnerState(key)
	return s.saveChats()
}

func (s *Store) chatIndex(key string) int {
	return slices.IndexFunc(s.chats, func(c model.ChatAssignment) bool { return c.Key == key })
}

func (s *Store) saveChats() error {
	return config.WriteJSONAtomic(s.paths.ChatsFile(), nonNil(s.chats), 0o600)
}

// ─── Approvals ───────────────────────────────────────────────

func (s *Store) Approvals() []model.PendingReply {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return slices.Clone(s.approvals)
}

func (s *Store) Approval(id string) (model.PendingReply, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if i := s.approvalIndex(id); i >= 0 {
		return s.approvals[i], true
	}
	return model.PendingReply{}, false
}

func (s *Store) UpsertApproval(p model.PendingReply) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if i := s.approvalIndex(p.ID); i >= 0 {
		s.approvals[i] = p
	} else {
		s.approvals = append(s.approvals, p)
	}
	return s.saveApprovals()
}

func (s *Store) DeleteApproval(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	i := s.approvalIndex(id)
	if i < 0 {
		return ErrNotFound
	}
	s.approvals = slices.Delete(s.approvals, i, i+1)
	return s.saveApprovals()
}

func (s *Store) approvalIndex(id string) int {
	return slices.IndexFunc(s.approvals, func(p model.PendingReply) bool { return p.ID == id })
}

func (s *Store) saveApprovals() error {
	return config.WriteJSONAtomic(s.paths.ApprovalsFile(), nonNil(s.approvals), 0o600)
}

// ─── History (append-only JSON lines per chat) ───────────────

const historyCompactFactor = 5

func (s *Store) historyFile(chatKey string) string {
	safe := strings.NewReplacer(":", "_", "/", "_", "@", "_", ".", "_").Replace(chatKey)
	return filepath.Join(s.paths.HistoryDir(), safe+".jsonl")
}

// AppendHistory appends one message. When the file grows past keep*5 lines it is
// compacted down to the last keep lines (keep <= 0 disables compaction).
func (s *Store) AppendHistory(chatKey string, m model.Message, keep int) error {
	s.histMu.Lock()
	defer s.histMu.Unlock()
	if m.ID == "" {
		m.ID = NewID()
	}
	if m.TS.IsZero() {
		m.TS = time.Now()
	}
	b, err := json.Marshal(m)
	if err != nil {
		return err
	}
	path := s.historyFile(chatKey)
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	_, err = f.Write(append(b, '\n'))
	f.Close()
	if err != nil {
		return err
	}
	if keep > 0 {
		msgs, err := readHistory(path)
		if err == nil && len(msgs) > keep*historyCompactFactor {
			return writeHistory(path, msgs[len(msgs)-keep:])
		}
	}
	return nil
}

// History returns the last limit messages (oldest first). limit <= 0 returns all.
func (s *Store) History(chatKey string, limit int) ([]model.Message, error) {
	s.histMu.Lock()
	defer s.histMu.Unlock()
	msgs, err := readHistory(s.historyFile(chatKey))
	if errors.Is(err, os.ErrNotExist) {
		return []model.Message{}, nil
	}
	if err != nil {
		return nil, err
	}
	if limit > 0 && len(msgs) > limit {
		msgs = msgs[len(msgs)-limit:]
	}
	return msgs, nil
}

func (s *Store) HistoryCount(chatKey string) int {
	msgs, _ := s.History(chatKey, 0)
	return len(msgs)
}

func (s *Store) ClearHistory(chatKey string) error {
	s.histMu.Lock()
	defer s.histMu.Unlock()
	err := os.Remove(s.historyFile(chatKey))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}

func readHistory(path string) ([]model.Message, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var out []model.Message
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 64*1024), 4*1024*1024)
	for sc.Scan() {
		var m model.Message
		if json.Unmarshal(sc.Bytes(), &m) == nil {
			out = append(out, m)
		}
	}
	return out, sc.Err()
}

func writeHistory(path string, msgs []model.Message) error {
	var sb strings.Builder
	for _, m := range msgs {
		b, err := json.Marshal(m)
		if err != nil {
			return err
		}
		sb.Write(b)
		sb.WriteByte('\n')
	}
	return config.WriteFileAtomic(path, []byte(sb.String()), 0o600)
}

// ─── helpers ─────────────────────────────────────────────────

func readJSON(path string, v any) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(b, v)
}

func nonNil[T any](s []T) []T {
	if s == nil {
		return []T{}
	}
	return s
}
