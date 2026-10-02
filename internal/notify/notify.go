// Package notify shows desktop notifications for activity that needs you: a
// reply waiting for your OK, a goal reached, a chat that needs you
// (hand-off), WhatsApp disconnecting, the daily recap. Backends:
//
//   - macOS: terminal-notifier when installed (clickable, grouped per chat),
//     otherwise AppleScript's "display notification" via osascript;
//   - Linux: notify-send (libnotify; not clickable);
//   - Windows: a toast shown through Windows PowerShell (clickable, opens the
//     browser; labelled "Windows PowerShell").
//
// Notifications follow settings.notifications (master switch, per-event
// switches, sound), are rate limited per event type and chat, and a
// WhatsApp disconnect is only reported when it lasts (debounced).
package notify

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/binary"
	"encoding/xml"
	"errors"
	"fmt"
	"hash/fnv"
	"log"
	"os/exec"
	"runtime"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf16"

	"whatsappdoppel/internal/config"
	"whatsappdoppel/internal/model"
	"whatsappdoppel/internal/platform"
)

// Backends.
const (
	BackendTerminalNotifier = "terminal-notifier"
	BackendOSAScript        = "osascript"
	BackendNotifySend       = "notify-send" // Linux (libnotify)
	BackendPowerShell       = "powershell"  // Windows toast via Windows PowerShell
	BackendDryRun           = "dry-run"     // logs instead of showing (tests, smoke runs)
	BackendNone             = "none"
)

const (
	// Title is the notification title.
	Title = "WhatsApp Doppel"
	// RateLimit: at most one notification per (event type, chat) this often.
	RateLimit = 60 * time.Second
	// WADebounce: WhatsApp has to stay disconnected this long to notify.
	WADebounce  = 30 * time.Second
	sendTimeout = 10 * time.Second
	maxMessage  = 180
)

// ErrUnavailable means this server can't show notifications.
var ErrUnavailable = errors.New("notifications aren't available on this computer")

// Note is one notification.
type Note struct {
	Subtitle string
	Message  string
	Group    string // replaces an earlier note of the same group (terminal-notifier, notify-send, toast)
	Open     string // URL opened on click (terminal-notifier, toast)
	Sound    bool
	Urgent   bool // notify-send: critical urgency (stays until dismissed)
}

// Timer is the part of *time.Timer the debounce needs.
type Timer interface{ Stop() bool }

// Options configure a Notifier. Zero values pick the real thing.
type Options struct {
	Config *config.Manager
	// URL is the app's base URL ("http://127.0.0.1:7788/"), for click-through.
	URL func() string
	// Events: notify about activity. false = only "Send a test" works (the
	// app runs with fake WhatsApp, e.g. a demo or a test run).
	Events bool
	// Backend forces a backend ("" = Detect()).
	Backend string
	// Run executes a backend command (nil = os/exec).
	Run func(ctx context.Context, name string, args ...string) error
	// Now and AfterFunc are the clock (nil = real time).
	Now       func() time.Time
	AfterFunc func(d time.Duration, f func()) Timer
	// Logf logs failures and dry runs (nil = log.Printf).
	Logf func(format string, args ...any)
}

// Notifier turns activity events into desktop notifications.
type Notifier struct {
	o       Options
	backend string

	mu      sync.Mutex
	last    map[string]time.Time // "type|chat" → last shown
	waTimer Timer
	waState string
	closed  bool
	wg      sync.WaitGroup
}

// New picks the backend and returns the notifier.
func New(o Options) *Notifier {
	if o.Run == nil {
		o.Run = func(ctx context.Context, name string, args ...string) error {
			cmd := exec.CommandContext(ctx, name, args...)
			platform.HideWindow(cmd) // no console flash for powershell.exe
			out, err := cmd.CombinedOutput()
			if err != nil && len(out) > 0 {
				return fmt.Errorf("%w: %s", err, strings.TrimSpace(string(out)))
			}
			return err
		}
	}
	if o.Now == nil {
		o.Now = time.Now
	}
	if o.AfterFunc == nil {
		o.AfterFunc = func(d time.Duration, f func()) Timer { return time.AfterFunc(d, f) }
	}
	if o.Logf == nil {
		o.Logf = log.Printf
	}
	if o.URL == nil {
		o.URL = func() string { return "" }
	}
	n := &Notifier{o: o, backend: o.Backend, last: map[string]time.Time{}}
	if n.backend == "" {
		n.backend = Detect()
	}
	return n
}

// Detect finds the best available backend on this computer.
func Detect() string {
	has := func(bin string) bool { _, err := exec.LookPath(bin); return err == nil }
	switch runtime.GOOS {
	case "darwin":
		if has("terminal-notifier") {
			return BackendTerminalNotifier
		}
		if has("osascript") {
			return BackendOSAScript
		}
	case "windows":
		// Windows PowerShell 5.1 (not pwsh, which cannot load the WinRT toast types).
		if has(powerShellExe) {
			return BackendPowerShell
		}
	default:
		if has("notify-send") {
			return BackendNotifySend
		}
	}
	return BackendNone
}

// Backend names how notifications are shown.
func (n *Notifier) Backend() string { return n.backend }

// EventsEnabled reports whether activity notifications are on for this
// server (false in fake-WhatsApp runs).
func (n *Notifier) EventsEnabled() bool { return n.o.Events }

// Test shows "Notifications are working" right away (ignoring the settings
// switches) and reports whether the backend ran.
func (n *Notifier) Test(ctx context.Context) error {
	sound := true
	if n.o.Config != nil {
		sound = n.o.Config.Get().Notifications.Sound
	}
	return n.show(ctx, Note{
		Subtitle: "Test",
		Message:  "Notifications are working. You'll see one like this when a chat needs you.",
		Group:    "doppel:test",
		Open:     n.o.URL(),
		Sound:    sound,
	})
}

// Close stops the WhatsApp debounce timer and waits for notes in flight.
func (n *Notifier) Close() {
	n.mu.Lock()
	n.closed = true
	if n.waTimer != nil {
		n.waTimer.Stop()
		n.waTimer = nil
	}
	n.mu.Unlock()
	n.wg.Wait()
}

// OnActivity maps an activity event to a notification (when the settings
// allow it) and shows it in the background.
func (n *Notifier) OnActivity(a model.ActivityEvent) {
	if !n.o.Events || n.o.Config == nil {
		return
	}
	s := n.o.Config.Get().Notifications
	if !s.Enabled && a.Type != model.ActWAStatus {
		return
	}
	switch a.Type {
	case model.ActWAStatus:
		n.waStatus(a, s)
		return
	}
	note, kind, ok := n.noteFor(a, s)
	if !ok || !n.allow(kind+"|"+a.ChatKey) {
		return
	}
	n.async(note)
}

// noteFor builds the notification for an event; ok is false when the event
// doesn't notify (or its switch is off).
func (n *Notifier) noteFor(a model.ActivityEvent, s config.NotificationSettings) (Note, string, bool) {
	chat := a.ChatName
	if chat == "" {
		chat = "a chat"
	}
	base := strings.TrimRight(n.o.URL(), "/") + "/"
	chatURL := base + "#/chats/" + a.ChatKey
	persona := a.PersonaName
	if persona == "" {
		persona = "The persona"
	}
	switch a.Type {
	case model.ActApprovalQueued:
		if !s.Approvals || a.Meta["regenerated"] == true {
			return Note{}, "", false
		}
		msg := persona + " wrote: " + quoted(a.Text)
		if d, ok := a.Meta["drafts"].(int); ok && d > 0 {
			msg = fmt.Sprintf("%s has %d ideas for you to pick from.", persona, d)
		}
		return Note{Subtitle: "Reply waiting for your OK · " + chat, Message: msg, Group: "doppel:approval:" + a.ChatKey,
			Open: base + "#/approvals", Sound: s.Sound}, a.Type, true
	case model.ActGoalReached:
		if !s.Goals {
			return Note{}, "", false
		}
		return Note{Subtitle: "Mission accomplished · " + chat, Message: a.Text, Group: "doppel:goal:" + a.ChatKey,
			Open: chatURL, Sound: s.Sound}, a.Type, true
	case model.ActHandoff:
		if !s.Handoff {
			return Note{}, "", false
		}
		msg := strings.TrimSuffix(a.Text, " — paused so you can take over")
		if ex, _ := a.Meta["excerpt"].(string); ex != "" {
			msg += ": " + quoted(ex)
		}
		return Note{Subtitle: "Needs you · " + chat, Message: msg + ". " + persona + " is paused there.",
			Group: "doppel:handoff:" + a.ChatKey, Open: chatURL, Sound: true, Urgent: true}, a.Type, true
	case model.ActRecap:
		if !s.Recap || a.Meta["onDemand"] == true {
			return Note{}, "", false
		}
		msg := a.Text
		if h, _ := a.Meta["headline"].(string); h != "" {
			msg += ". " + h
		}
		return Note{Subtitle: "Daily recap", Message: msg, Group: "doppel:recap", Open: base, Sound: s.Sound}, a.Type, true
	}
	return Note{}, "", false
}

// waDown are the WhatsApp states worth a notification.
var waDown = map[string]string{
	model.WADisconnected: "WhatsApp disconnected. Replies are paused until it reconnects.",
	model.WALoggedOut:    "WhatsApp logged out. Scan the QR code again to relink.",
	model.WAReplaced:     "WhatsApp was opened in another session. Relink to keep replying.",
	model.WABanned:       "WhatsApp blocked this linked device.",
	model.WAError:        "WhatsApp hit an error and stopped. Open Doppel to reconnect.",
	model.WAOutdated:     "WhatsApp needs an update of Doppel before it can connect.",
}

// waStatus notifies about WhatsApp going down only if it stays down for
// WADebounce; coming back (connected) cancels it.
func (n *Notifier) waStatus(a model.ActivityEvent, s config.NotificationSettings) {
	state, _ := a.Meta["state"].(string)
	n.mu.Lock()
	defer n.mu.Unlock()
	if n.waTimer != nil && (state == model.WAConnected || waDown[state] != "") {
		n.waTimer.Stop()
		n.waTimer = nil
	}
	msg, down := waDown[state]
	if !down || n.closed || !s.Enabled || !s.WhatsApp {
		return
	}
	n.waState = state
	n.waTimer = n.o.AfterFunc(WADebounce, func() {
		n.mu.Lock()
		if n.closed || n.waState != state {
			n.mu.Unlock()
			return
		}
		n.waTimer = nil
		n.mu.Unlock()
		cur := n.o.Config.Get().Notifications
		if !cur.Enabled || !cur.WhatsApp || !n.allow(model.ActWAStatus+"|"+state) {
			return
		}
		n.async(Note{Subtitle: "WhatsApp", Message: msg, Group: "doppel:whatsapp", Open: strings.TrimRight(n.o.URL(), "/") + "/", Sound: cur.Sound})
	})
}

// allow applies the rate limit for key.
func (n *Notifier) allow(key string) bool {
	now := n.o.Now()
	n.mu.Lock()
	defer n.mu.Unlock()
	if t, ok := n.last[key]; ok && now.Sub(t) < RateLimit {
		return false
	}
	n.last[key] = now
	return true
}

func (n *Notifier) async(note Note) {
	n.mu.Lock()
	if n.closed {
		n.mu.Unlock()
		return
	}
	n.wg.Add(1)
	n.mu.Unlock()
	go func() {
		defer n.wg.Done()
		ctx, cancel := context.WithTimeout(context.Background(), sendTimeout)
		defer cancel()
		if err := n.show(ctx, note); err != nil {
			n.o.Logf("notify: %v", err)
		}
	}()
}

// show runs the backend for one note.
func (n *Notifier) show(ctx context.Context, note Note) error {
	switch n.backend {
	case BackendDryRun:
		n.o.Logf("notify (dry run): %s — %s", note.Subtitle, note.Message)
		return nil
	case BackendTerminalNotifier, BackendOSAScript, BackendNotifySend, BackendPowerShell:
		bin, args := Args(n.backend, note)
		return n.o.Run(ctx, bin, args...)
	}
	return ErrUnavailable
}

// Args returns the command line that shows note with backend.
func Args(backend string, note Note) (string, []string) {
	sub, msg := clean(note.Subtitle, 80), clean(note.Message, maxMessage)
	if msg == "" {
		msg = " "
	}
	switch backend {
	case BackendNotifySend:
		return notifySendArgs(sub, msg, note)
	case BackendPowerShell:
		return powerShellExe, powerShellArgs(sub, msg, note)
	}
	if backend == BackendTerminalNotifier {
		// terminal-notifier reads a leading "-" or "[" as an option.
		if strings.HasPrefix(msg, "-") || strings.HasPrefix(msg, "[") {
			msg = " " + msg
		}
		args := []string{"-title", Title, "-message", msg}
		if sub != "" {
			args = append(args, "-subtitle", sub)
		}
		if note.Group != "" {
			args = append(args, "-group", note.Group)
		}
		if note.Open != "" {
			args = append(args, "-open", note.Open)
		}
		if note.Sound {
			args = append(args, "-sound", "default")
		}
		return "terminal-notifier", args
	}
	script := "display notification " + appleString(msg) + " with title " + appleString(Title)
	if sub != "" {
		script += " subtitle " + appleString(sub)
	}
	if note.Sound {
		script += ` sound name "default"`
	}
	return "osascript", []string{"-e", script}
}

// appleString quotes s as an AppleScript string literal.
func appleString(s string) string {
	return `"` + strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(s) + `"`
}

// clean makes s one line of printable text of at most n characters.
func clean(s string, n int) string {
	s = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return ' '
		}
		return r
	}, s)
	s = strings.Join(strings.Fields(s), " ")
	if r := []rune(s); len(r) > n {
		s = strings.TrimSpace(string(r[:n-1])) + "…"
	}
	return s
}

func quoted(s string) string {
	s = clean(s, 120)
	if s == "" {
		return ""
	}
	return "“" + s + "”"
}

// notifySendArgs: summary "WhatsApp Doppel · <subtitle>", body = message.
// "--" ends the options so a message starting with "-" is not read as one;
// the body is escaped because most notification daemons render markup.
// notify-send has no click action and plays no sound of its own.
func notifySendArgs(sub, msg string, note Note) (string, []string) {
	summary := Title
	if sub != "" {
		summary += " · " + sub
	}
	urgency := "normal"
	if note.Urgent {
		urgency = "critical"
	}
	args := []string{"-a", Title, "-i", "whatsapp-doppel", "-u", urgency,
		"-h", "string:desktop-entry:whatsapp-doppel"}
	if note.Group != "" {
		args = append(args, "-h", "string:x-canonical-private-synchronous:"+note.Group)
	}
	body := strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;").Replace(msg)
	return "notify-send", append(args, "--", summary, body)
}

// powerShellExe is Windows PowerShell 5.1, on PATH on every Windows 10/11.
const powerShellExe = "powershell.exe"

// powerShellAppID is the AppUserModelID Windows PowerShell registers; a toast
// needs a registered app id, so it is shown as coming from "Windows PowerShell".
const powerShellAppID = `{1AC14E77-02E7-4E5D-B744-2EB1AE5198B7}\WindowsPowerShell\v1.0\powershell.exe`

// powerShellArgs builds a toast (XML-escaped text, protocol activation opening
// note.Open) and passes the script as -EncodedCommand (UTF-16LE base64), so no
// shell quoting is involved. The XML is one line (clean() collapsed the text),
// so it cannot end the '@ here-string early.
func powerShellArgs(sub, msg string, note Note) []string {
	esc := func(s string) string {
		var b bytes.Buffer
		_ = xml.EscapeText(&b, []byte(s))
		return b.String()
	}
	var x strings.Builder
	x.WriteString(`<toast`)
	if note.Open != "" {
		x.WriteString(` activationType="protocol" launch="` + esc(note.Open) + `"`)
	}
	if note.Urgent {
		x.WriteString(` scenario="reminder"`)
	}
	x.WriteString(`><visual><binding template="ToastGeneric"><text>` + esc(Title) + `</text>`)
	if sub != "" {
		x.WriteString(`<text>` + esc(sub) + `</text>`)
	}
	x.WriteString(`<text>` + esc(msg) + `</text></binding></visual>`)
	if note.Urgent && note.Open != "" {
		// A reminder toast needs a button; it opens the chat like a click does.
		x.WriteString(`<actions><action content="Open" activationType="protocol" arguments="` + esc(note.Open) + `"/></actions>`)
	}
	if !note.Sound {
		x.WriteString(`<audio silent="true"/>`)
	}
	x.WriteString(`</toast>`)

	tag := ""
	if note.Group != "" {
		h := fnv.New64a()
		h.Write([]byte(note.Group))
		tag = fmt.Sprintf("$t.Tag = '%016x'; $t.Group = 'doppel'\n", h.Sum64()) // tags are limited to 64 (old: 16) chars
	}
	script := "$ErrorActionPreference = 'Stop'\n" +
		"[void][Windows.UI.Notifications.ToastNotificationManager, Windows.UI.Notifications, ContentType = WindowsRuntime]\n" +
		"[void][Windows.Data.Xml.Dom.XmlDocument, Windows.Data.Xml.Dom.XmlDocument, ContentType = WindowsRuntime]\n" +
		"$x = New-Object Windows.Data.Xml.Dom.XmlDocument\n" +
		"$x.LoadXml(@'\n" + x.String() + "\n'@)\n" +
		"$t = New-Object Windows.UI.Notifications.ToastNotification $x\n" +
		tag +
		"[Windows.UI.Notifications.ToastNotificationManager]::CreateToastNotifier('" + powerShellAppID + "').Show($t)\n"
	return []string{"-NoProfile", "-NonInteractive", "-WindowStyle", "Hidden", "-EncodedCommand", encodePowerShell(script)}
}

// encodePowerShell returns s as base64 of UTF-16LE, the -EncodedCommand format.
func encodePowerShell(s string) string {
	u := utf16.Encode([]rune(s))
	b := make([]byte, 2*len(u))
	for i, c := range u {
		binary.LittleEndian.PutUint16(b[2*i:], c)
	}
	return base64.StdEncoding.EncodeToString(b)
}
