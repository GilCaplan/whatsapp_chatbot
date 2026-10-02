// Package config owns config.json (settings) and secrets.json (API keys).
package config

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"whatsappdoppel/internal/behavior"
	"whatsappdoppel/internal/model"
)

type LLMSettings struct {
	DefaultProvider string  `json:"defaultProvider"` // ollama|anthropic|openai
	DefaultModel    string  `json:"defaultModel"`    // model for the default provider
	OllamaURL       string  `json:"ollamaURL"`
	OllamaModel     string  `json:"ollamaModel"`
	AnthropicModel  string  `json:"anthropicModel"`
	OpenAIModel     string  `json:"openaiModel"`
	OpenAIBaseURL   string  `json:"openaiBaseURL"`
	Temperature     float64 `json:"temperature"`
	ReplyMaxTokens  int     `json:"replyMaxTokens"`
	OllamaNumCtx    int     `json:"ollamaNumCtx"`
}

// BehaviorSettings are the app-wide reply-behaviour profiles; chats inherit
// Private or Group by kind and may override single fields.
type BehaviorSettings struct {
	TriggerPrefix string                `json:"triggerPrefix"` // your own "<prefix> text" makes the persona reply at once
	Private       model.BehaviorProfile `json:"private"`
	Group         model.BehaviorProfile `json:"group"`
}

// For returns the profile for a chat kind ("group" or anything else = private).
func (b BehaviorSettings) For(kind string) model.BehaviorProfile {
	if kind == behavior.KindGroup {
		return b.Group
	}
	return b.Private
}

// Legacy (v1) blocks: read once from old files, migrated into Behavior and
// never written again.
type (
	LegacyReplySettings    = behavior.LegacyReplies
	LegacyApprovalSettings = behavior.LegacyApprovals
)

// SettingsVersion is the current config.json schema version.
// v3 added the mention and "who it answers" behaviour fields (V3Fields).
// v4 added the typo behaviour fields (V4Fields) and the notifications,
// safety, memory, recap and clone blocks.
const SettingsVersion = 4

// V3Fields are the behaviour fields added in settings v3. Older files get them
// from the preset each profile is labelled with, so "Natural" stays Natural.
var V3Fields = []string{
	"allowMentions", "mentionMax", "tagReplyPercent",
	"respondToAllMaxMembers", "answerAnyoneWhoAddressesIt", "maxStreak", "triggerWords", "muteWords",
}

// V4Fields are the behaviour fields added in settings v4 (filled from the
// labelled preset like V3Fields).
var V4Fields = []string{"typoPercent", "typoFixStyle"}

// NotificationSettings controls desktop notifications (internal/notify).
type NotificationSettings struct {
	Enabled   bool `json:"enabled"`
	Approvals bool `json:"approvals"` // a reply is waiting for your OK
	Goals     bool `json:"goals"`     // a goal / mission was reached
	Handoff   bool `json:"handoff"`   // a chat needs you (always with sound)
	WhatsApp  bool `json:"whatsapp"`  // WhatsApp disconnected / logged out
	Recap     bool `json:"recap"`     // the daily recap is ready
	Sound     bool `json:"sound"`
}

// HandoffSettings chooses which sensitive messages pause the persona.
type HandoffSettings struct {
	Enabled  bool `json:"enabled"`
	Money    bool `json:"money"`
	Health   bool `json:"health"`
	Meeting  bool `json:"meeting"`
	Distress bool `json:"distress"`
	Bot      bool `json:"bot"`     // "are you a bot?"
	Legal    bool `json:"legal"`   // lawyers, police, contracts
	AICheck  bool `json:"aiCheck"` // ask the AI when a keyword hit is unclear
}

// RevealSettings holds the default "it was a persona" message.
type RevealSettings struct {
	Template string `json:"template"` // {persona} and {me} are filled in; ≤ MaxRevealTemplate characters
}

// SafetySettings groups hand-off and reveal.
type SafetySettings struct {
	Handoff HandoffSettings `json:"handoff"`
	Reveal  RevealSettings  `json:"reveal"`
}

// MemorySettings: learn about the people in chats (per chat: ChatAssignment.Memory).
type MemorySettings struct {
	Enabled bool `json:"enabled"`
}

// RecapSettings: the daily recap of every active chat.
type RecapSettings struct {
	Enabled  bool   `json:"enabled"`
	Time     string `json:"time"`     // "HH:MM" in this Mac's zone
	KeepDays int    `json:"keepDays"` // 1–365
}

// CloneSettings: consent to keep a private sample of your own messages.
type CloneSettings struct {
	CollectSamples bool `json:"collectSamples"`
	MaxSamples     int  `json:"maxSamples"` // 50–5000
}

// DefaultRevealTemplate is the default reveal message.
const DefaultRevealTemplate = "Quick confession: for a while now you've been chatting with {persona}, an AI persona I set up. It was me behind it, and I'm taking over from here. Hope it was fun, tell me what you thought!"

// MaxRevealTemplate is the longest reveal template accepted (characters).
const MaxRevealTemplate = 600

type Settings struct {
	Version             int              `json:"version"`
	Port                int              `json:"port"`
	Theme               string           `json:"theme"` // system|light|dark
	OnboardingCompleted bool             `json:"onboardingCompleted"`
	LLM                 LLMSettings      `json:"llm"`
	Behavior            BehaviorSettings `json:"behavior"`

	// v4 blocks.
	Notifications NotificationSettings `json:"notifications"`
	Safety        SafetySettings       `json:"safety"`
	Memory        MemorySettings       `json:"memory"`
	Recap         RecapSettings        `json:"recap"`
	SelfClone     CloneSettings        `json:"clone"` // "Clone yourself" (Settings.Clone is the deep copy)

	// Deprecated v1 blocks; nil after migration.
	Replies   *LegacyReplySettings    `json:"replies,omitempty"`
	Approvals *LegacyApprovalSettings `json:"approvals,omitempty"`
}

// Clone deep-copies the settings (profiles contain slices).
func (s Settings) Clone() Settings {
	s.Behavior.Private = behavior.Clone(s.Behavior.Private)
	s.Behavior.Group = behavior.Clone(s.Behavior.Group)
	if s.Replies != nil {
		r := *s.Replies
		s.Replies = &r
	}
	if s.Approvals != nil {
		a := *s.Approvals
		s.Approvals = &a
	}
	return s
}

func Defaults() Settings {
	return Settings{
		Version: SettingsVersion,
		Port:    7788,
		Theme:   "system",
		LLM: LLMSettings{
			DefaultProvider: "ollama",
			DefaultModel:    "llama3.1:8b",
			OllamaURL:       "http://127.0.0.1:11434",
			OllamaModel:     "llama3.1:8b",
			AnthropicModel:  "claude-sonnet-5-5",
			OpenAIModel:     "",
			OpenAIBaseURL:   "https://api.openai.com/v1",
			Temperature:     0.8,
			ReplyMaxTokens:  400,
			OllamaNumCtx:    8192,
		},
		Behavior: BehaviorSettings{
			TriggerPrefix: "1",
			Private:       behavior.Default(behavior.KindDM),
			Group:         behavior.Default(behavior.KindGroup),
		},
		Notifications: NotificationSettings{Enabled: true, Approvals: true, Goals: true, Handoff: true, WhatsApp: true, Recap: false, Sound: true},
		Safety: SafetySettings{
			Handoff: HandoffSettings{Enabled: true, Money: true, Health: true, Meeting: true, Distress: true, Bot: true, Legal: true, AICheck: true},
			Reveal:  RevealSettings{Template: DefaultRevealTemplate},
		},
		Memory:    MemorySettings{Enabled: true},
		Recap:     RecapSettings{Enabled: false, Time: "21:00", KeepDays: 30},
		SelfClone: CloneSettings{CollectSamples: false, MaxSamples: 500},
	}
}

// ModelFor returns the configured model for a provider.
func (l LLMSettings) ModelFor(provider string) string {
	switch provider {
	case "ollama":
		return l.OllamaModel
	case "anthropic":
		return l.AnthropicModel
	case "openai":
		return l.OpenAIModel
	}
	return ""
}

type Secrets struct {
	AnthropicKey string `json:"anthropicKey"`
	OpenAIKey    string `json:"openaiKey"`
}

// SecretView is what the browser is allowed to see.
type SecretView struct {
	Set  bool   `json:"set"`
	Hint string `json:"hint,omitempty"` // "…4f2a"
}

func mask(s string) SecretView {
	if s == "" {
		return SecretView{}
	}
	r := []rune(s)
	if len(r) <= 4 {
		return SecretView{Set: true, Hint: "…"}
	}
	return SecretView{Set: true, Hint: "…" + string(r[len(r)-4:])}
}

func (s Secrets) View() map[string]SecretView {
	return map[string]SecretView{"anthropic": mask(s.AnthropicKey), "openai": mask(s.OpenAIKey)}
}

// Manager is the thread-safe owner of settings and secrets.
type Manager struct {
	paths     Paths
	mu        sync.RWMutex
	settings  Settings
	secrets   Secrets
	listeners []func()
}

func Load(p Paths) (*Manager, error) {
	m := &Manager{paths: p, settings: Defaults()}
	if err := readJSON(p.ConfigFile(), &m.settings); err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	if err := readJSON(p.SecretsFile(), &m.secrets); err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	m.normalize()
	if err := WriteJSONAtomic(p.ConfigFile(), m.settings, 0o600); err != nil {
		return nil, err
	}
	return m, nil
}

// normalize fills zero values that older/partial config files may lack.
func (m *Manager) normalize() {
	d := Defaults()
	s := &m.settings
	loaded := s.Version
	if s.Version == 0 {
		s.Version = d.Version
	}
	if s.Port == 0 {
		s.Port = d.Port
	}
	if s.Theme == "" {
		s.Theme = d.Theme
	}
	if s.LLM.DefaultProvider == "" {
		s.LLM.DefaultProvider = d.LLM.DefaultProvider
	}
	if s.LLM.OllamaURL == "" {
		s.LLM.OllamaURL = d.LLM.OllamaURL
	}
	if s.LLM.OllamaModel == "" {
		s.LLM.OllamaModel = d.LLM.OllamaModel
	}
	if s.LLM.AnthropicModel == "" {
		s.LLM.AnthropicModel = d.LLM.AnthropicModel
	}
	if s.LLM.OpenAIBaseURL == "" {
		s.LLM.OpenAIBaseURL = d.LLM.OpenAIBaseURL
	}
	if s.LLM.ReplyMaxTokens == 0 {
		s.LLM.ReplyMaxTokens = d.LLM.ReplyMaxTokens
	}
	if s.LLM.OllamaNumCtx == 0 {
		s.LLM.OllamaNumCtx = d.LLM.OllamaNumCtx
	}
	migrateLegacy(s)
	if loaded < 3 {
		behavior.FillFromPreset(&s.Behavior.Private, behavior.KindDM, V3Fields...)
		behavior.FillFromPreset(&s.Behavior.Group, behavior.KindGroup, V3Fields...)
	}
	if loaded < 4 {
		behavior.FillFromPreset(&s.Behavior.Private, behavior.KindDM, V4Fields...)
		behavior.FillFromPreset(&s.Behavior.Group, behavior.KindGroup, V4Fields...)
		// v4 tweaked Busy/Slow so presets land on the vibe dials: a profile
		// still holding the old preset keeps its label (one-shot).
		behavior.RelabelLegacyPreset(&s.Behavior.Private, behavior.KindDM)
		behavior.RelabelLegacyPreset(&s.Behavior.Group, behavior.KindGroup)
		// Bools can't be told apart from "unset", so older files get the
		// v4 blocks' defaults wholesale.
		s.Notifications, s.Safety, s.Memory = d.Notifications, d.Safety, d.Memory
		s.Recap, s.SelfClone = d.Recap, d.SelfClone
	}
	normalizeV4(s, d)
	behavior.Normalize(&s.Behavior.Private, behavior.KindDM)
	behavior.Normalize(&s.Behavior.Group, behavior.KindGroup)
	// Keep DefaultModel in sync with the per-provider model.
	if mdl := s.LLM.ModelFor(s.LLM.DefaultProvider); mdl != "" {
		s.LLM.DefaultModel = mdl
	}
}

// normalizeV4 fills zero values of the v4 blocks that a partial file or PUT
// may leave empty (non-bool fields only).
func normalizeV4(s *Settings, d Settings) {
	if strings.TrimSpace(s.Safety.Reveal.Template) == "" {
		s.Safety.Reveal.Template = d.Safety.Reveal.Template
	}
	if s.Recap.Time == "" {
		s.Recap.Time = d.Recap.Time
	}
	if s.Recap.KeepDays <= 0 {
		s.Recap.KeepDays = d.Recap.KeepDays
	}
	if s.SelfClone.MaxSamples <= 0 {
		s.SelfClone.MaxSamples = d.SelfClone.MaxSamples
	}
}

// migrateLegacy moves v1 "replies"/"approvals" into the behaviour profiles
// (one-shot: the legacy blocks are dropped and the version bumped).
func migrateLegacy(s *Settings) {
	if s.Replies != nil {
		var a LegacyApprovalSettings
		if s.Approvals != nil {
			a = *s.Approvals
		}
		s.Behavior.Private, s.Behavior.Group = behavior.MigrateLegacy(*s.Replies, a)
		s.Behavior.TriggerPrefix = s.Replies.TriggerPrefix
		if s.Replies.DebounceSeconds == 0 && s.Replies.BurstCapSeconds == 0 && s.Replies.MaxHistoryMessages == 0 && s.Replies.TriggerPrefix == "" {
			s.Behavior.TriggerPrefix = behavior.LegacyDefaults().TriggerPrefix
		}
	} else if s.Approvals != nil {
		s.Behavior.Private.AutoSendSeconds = s.Approvals.AutoSendSeconds
		s.Behavior.Group.AutoSendSeconds = s.Approvals.AutoSendSeconds
	}
	s.Replies, s.Approvals = nil, nil
	if s.Version < SettingsVersion {
		s.Version = SettingsVersion
	}
}

func (m *Manager) Paths() Paths { return m.paths }

// Get returns a deep copy of the current settings.
func (m *Manager) Get() Settings {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.settings.Clone()
}

// Update applies fn to a copy of the settings, persists, then notifies listeners.
func (m *Manager) Update(fn func(*Settings)) (Settings, error) {
	m.mu.Lock()
	next := m.settings.Clone()
	fn(&next)
	prev := m.settings
	m.settings = next
	m.normalize()
	if err := WriteJSONAtomic(m.paths.ConfigFile(), m.settings, 0o600); err != nil {
		m.settings = prev
		m.mu.Unlock()
		return prev, err
	}
	out := m.settings.Clone()
	listeners := append([]func(){}, m.listeners...)
	m.mu.Unlock()
	for _, l := range listeners {
		l()
	}
	return out, nil
}

func (m *Manager) Secrets() Secrets {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.secrets
}

// UpdateSecrets persists secrets (mode 0600) and notifies listeners.
func (m *Manager) UpdateSecrets(fn func(*Secrets)) (Secrets, error) {
	m.mu.Lock()
	next := m.secrets
	fn(&next)
	if err := WriteJSONAtomic(m.paths.SecretsFile(), next, 0o600); err != nil {
		m.mu.Unlock()
		return m.secrets, err
	}
	m.secrets = next
	listeners := append([]func(){}, m.listeners...)
	m.mu.Unlock()
	for _, l := range listeners {
		l()
	}
	return next, nil
}

// OnChange registers fn to run (outside the lock) after any settings/secrets change.
func (m *Manager) OnChange(fn func()) {
	m.mu.Lock()
	m.listeners = append(m.listeners, fn)
	m.mu.Unlock()
}

func readJSON(path string, v any) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(b, v)
}

// WriteJSONAtomic writes v as indented JSON via temp file + rename.
func WriteJSONAtomic(path string, v any, perm os.FileMode) error {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	return WriteFileAtomic(path, b, perm)
}

func WriteFileAtomic(path string, data []byte, perm os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".tmp*")
	if err != nil {
		return err
	}
	name := tmp.Name()
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(name)
		return err
	}
	if err := tmp.Chmod(perm); err != nil {
		tmp.Close()
		os.Remove(name)
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(name)
		return err
	}
	err = os.Rename(name, path)
	// Windows refuses to replace a file another process has open at that
	// moment (e.g. the launcher polling instance.json): retry briefly.
	for i := 0; err != nil && runtime.GOOS == "windows" && i < 25; i++ {
		time.Sleep(20 * time.Millisecond)
		err = os.Rename(name, path)
	}
	if err != nil {
		os.Remove(name)
	}
	return err
}
