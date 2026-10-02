package notify

import (
	"context"
	"encoding/base64"
	"encoding/binary"
	"runtime"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"whatsappdoppel/internal/config"
	"whatsappdoppel/internal/model"
)

func TestArgsOSAScriptEscaping(t *testing.T) {
	bin, args := Args(BackendOSAScript, Note{Subtitle: `Needs you · "Dana"`, Message: "she said \"hi\" \\ bye\nnew line", Sound: true})
	if bin != "osascript" || len(args) != 2 || args[0] != "-e" {
		t.Fatalf("args = %s %q", bin, args)
	}
	want := `display notification "she said \"hi\" \\ bye new line" with title "WhatsApp Doppel" subtitle "Needs you · \"Dana\"" sound name "default"`
	if args[1] != want {
		t.Errorf("script =\n%s\nwant\n%s", args[1], want)
	}
	_, args = Args(BackendOSAScript, Note{Message: "x"})
	if strings.Contains(args[1], "sound") || strings.Contains(args[1], "subtitle") {
		t.Errorf("no sound / subtitle: %s", args[1])
	}
}

func TestArgsTerminalNotifier(t *testing.T) {
	bin, args := Args(BackendTerminalNotifier, Note{Subtitle: "S", Message: "-rf looks like a flag", Group: "doppel:x", Open: "http://127.0.0.1:7788/#/approvals", Sound: true})
	got := strings.Join(args, "|")
	want := "-title|WhatsApp Doppel|-message| -rf looks like a flag|-subtitle|S|-group|doppel:x|-open|http://127.0.0.1:7788/#/approvals|-sound|default"
	if bin != "terminal-notifier" || got != want {
		t.Errorf("args = %s %s", bin, got)
	}
	_, args = Args(BackendTerminalNotifier, Note{Message: strings.Repeat("a", 500)})
	if n := len([]rune(args[3])); n != maxMessage {
		t.Errorf("message length = %d", n)
	}
}

// ─── fakes ───────────────────────────────────────────────────

type fakeClock struct {
	mu     sync.Mutex
	now    time.Time
	timers []*fakeTimer
}

type fakeTimer struct {
	c       *fakeClock
	at      time.Time
	f       func()
	stopped bool
}

func (t *fakeTimer) Stop() bool {
	t.c.mu.Lock()
	defer t.c.mu.Unlock()
	was := !t.stopped
	t.stopped = true
	return was
}

func (c *fakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *fakeClock) AfterFunc(d time.Duration, f func()) Timer {
	c.mu.Lock()
	defer c.mu.Unlock()
	t := &fakeTimer{c: c, at: c.now.Add(d), f: f}
	c.timers = append(c.timers, t)
	return t
}

func (c *fakeClock) Advance(d time.Duration) {
	c.mu.Lock()
	c.now = c.now.Add(d)
	var due []*fakeTimer
	for _, t := range c.timers {
		if !t.stopped && !t.at.After(c.now) {
			t.stopped = true
			due = append(due, t)
		}
	}
	c.mu.Unlock()
	for _, t := range due {
		t.f()
	}
}

type recorder struct {
	mu   sync.Mutex
	runs [][]string
}

func (r *recorder) run(_ context.Context, name string, args ...string) error {
	r.mu.Lock()
	r.runs = append(r.runs, append([]string{name}, args...))
	r.mu.Unlock()
	return nil
}

func (r *recorder) count() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.runs)
}

func (r *recorder) last() string {
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.runs) == 0 {
		return ""
	}
	return strings.Join(r.runs[len(r.runs)-1], " ")
}

func newTest(t *testing.T, events bool) (*Notifier, *recorder, *fakeClock, *config.Manager) {
	t.Helper()
	paths, err := config.NewPaths(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := config.Load(paths)
	if err != nil {
		t.Fatal(err)
	}
	rec := &recorder{}
	clk := &fakeClock{now: time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)}
	n := New(Options{Config: cfg, URL: func() string { return "http://127.0.0.1:7788/" }, Events: events, Backend: BackendTerminalNotifier,
		Run: rec.run, Now: clk.Now, AfterFunc: clk.AfterFunc, Logf: func(string, ...any) {}})
	return n, rec, clk, cfg
}

func TestEventsAndRateLimit(t *testing.T) {
	n, rec, clk, cfg := newTest(t, true)
	queued := model.ActivityEvent{Type: model.ActApprovalQueued, ChatKey: "dm:1", ChatName: "Dana", PersonaName: "Leo", Text: "sure, 8?"}
	n.OnActivity(queued)
	n.Close()
	if rec.count() != 1 || !strings.Contains(rec.last(), "Reply waiting for your OK · Dana") || !strings.Contains(rec.last(), "Leo wrote: “sure, 8?”") ||
		!strings.Contains(rec.last(), "-open http://127.0.0.1:7788/#/approvals") {
		t.Fatalf("run = %s", rec.last())
	}
	n, rec, clk, cfg = newTest(t, true)
	n.OnActivity(queued)
	n.OnActivity(queued) // within a minute: dropped
	n.OnActivity(model.ActivityEvent{Type: model.ActApprovalQueued, ChatKey: "dm:2", ChatName: "Josh", Text: "x"})
	clk.Advance(61 * time.Second)
	n.OnActivity(queued)
	n.OnActivity(model.ActivityEvent{Type: model.ActApprovalQueued, ChatKey: "dm:3", Text: "y", Meta: map[string]any{"regenerated": true}})
	n.OnActivity(model.ActivityEvent{Type: model.ActIncoming, ChatKey: "dm:1", Text: "hi"})
	n.Close()
	if c := rec.count(); c != 3 {
		t.Fatalf("runs = %d", c)
	}

	// Hand-off always rings; switches and the master switch are honoured.
	n, rec, _, cfg = newTest(t, true)
	if _, err := cfg.Update(func(s *config.Settings) { s.Notifications.Sound = false; s.Notifications.Goals = false }); err != nil {
		t.Fatal(err)
	}
	n.OnActivity(model.ActivityEvent{Type: model.ActHandoff, ChatKey: "dm:1", ChatName: "Dana", PersonaName: "Leo",
		Text: "Dana asked if they're talking to a bot — paused so you can take over", Meta: map[string]any{"excerpt": "are you a bot?"}})
	n.OnActivity(model.ActivityEvent{Type: model.ActGoalReached, ChatKey: "dm:1", Text: "Goal reached"})
	n.Close()
	if rec.count() != 1 || !strings.Contains(rec.last(), "-sound default") || !strings.Contains(rec.last(), "Needs you · Dana") ||
		!strings.Contains(rec.last(), "Dana asked if they're talking to a bot: “are you a bot?”. Leo is paused there.") {
		t.Fatalf("handoff = %s", rec.last())
	}
	n, rec, _, cfg = newTest(t, true)
	if _, err := cfg.Update(func(s *config.Settings) { s.Notifications.Enabled = false }); err != nil {
		t.Fatal(err)
	}
	n.OnActivity(model.ActivityEvent{Type: model.ActHandoff, ChatKey: "dm:1", Text: "x"})
	n.Close()
	if rec.count() != 0 {
		t.Error("master switch off")
	}
	// Recaps: only scheduled ones, only when switched on (off by default).
	n, rec, _, cfg = newTest(t, true)
	n.OnActivity(model.ActivityEvent{Type: model.ActRecap, Text: "Daily recap ready for Dana"})
	if _, err := cfg.Update(func(s *config.Settings) { s.Notifications.Recap = true }); err != nil {
		t.Fatal(err)
	}
	n.OnActivity(model.ActivityEvent{Type: model.ActRecap, Text: "Daily recap ready for Dana", Meta: map[string]any{"onDemand": true}})
	n.OnActivity(model.ActivityEvent{Type: model.ActRecap, Text: "Daily recap ready for Dana", Meta: map[string]any{"headline": "Exam on Friday"}})
	n.Close()
	if rec.count() != 1 || !strings.Contains(rec.last(), "Daily recap ready for Dana. Exam on Friday") {
		t.Errorf("recap = %d %s", rec.count(), rec.last())
	}
}

func TestEventsOffInDemoMode(t *testing.T) {
	n, rec, _, _ := newTest(t, false)
	n.OnActivity(model.ActivityEvent{Type: model.ActHandoff, ChatKey: "dm:1", Text: "x"})
	if err := n.Test(context.Background()); err != nil {
		t.Fatal(err)
	}
	n.Close()
	if rec.count() != 1 || !strings.Contains(rec.last(), "Notifications are working") {
		t.Errorf("only the test shows: %d %s", rec.count(), rec.last())
	}
}

func TestWhatsAppDebounce(t *testing.T) {
	n, rec, clk, _ := newTest(t, true)
	wa := func(state string) {
		n.OnActivity(model.ActivityEvent{Type: model.ActWAStatus, Meta: map[string]any{"state": state}})
	}
	// A blip that recovers within 30 s says nothing.
	wa(model.WADisconnected)
	clk.Advance(10 * time.Second)
	wa(model.WAConnecting)
	clk.Advance(10 * time.Second)
	wa(model.WAConnected)
	clk.Advance(time.Minute)
	if rec.count() != 0 {
		t.Fatalf("blip notified: %s", rec.last())
	}
	// Staying down does.
	wa(model.WADisconnected)
	clk.Advance(29 * time.Second)
	if rec.count() != 0 {
		t.Fatal("too early")
	}
	clk.Advance(2 * time.Second)
	n.Close()
	if rec.count() != 1 || !strings.Contains(rec.last(), "WhatsApp disconnected") {
		t.Fatalf("down = %d %s", rec.count(), rec.last())
	}
	// A later state replaces the pending one (logged out wins).
	n, rec, clk, _ = newTest(t, true)
	wa(model.WADisconnected)
	clk.Advance(5 * time.Second)
	wa(model.WALoggedOut)
	clk.Advance(31 * time.Second)
	n.Close()
	if rec.count() != 1 || !strings.Contains(rec.last(), "logged out") {
		t.Fatalf("logged out = %d %s", rec.count(), rec.last())
	}
}

func TestBackends(t *testing.T) {
	var logged string
	n := New(Options{Backend: BackendDryRun, Logf: func(f string, a ...any) { logged = f }})
	if err := n.Test(context.Background()); err != nil || n.Backend() != BackendDryRun || !strings.Contains(logged, "dry run") {
		t.Errorf("dry run = %v %q", err, logged)
	}
	n = New(Options{Backend: BackendNone})
	if err := n.Test(context.Background()); err != ErrUnavailable {
		t.Errorf("none = %v", err)
	}
	ok := map[string][]string{
		"darwin":  {BackendTerminalNotifier, BackendOSAScript, BackendNone},
		"windows": {BackendPowerShell, BackendNone},
	}[runtime.GOOS]
	if ok == nil {
		ok = []string{BackendNotifySend, BackendNone}
	}
	if b := Detect(); !slices.Contains(ok, b) {
		t.Errorf("Detect = %q on %s, want one of %q", b, runtime.GOOS, ok)
	}
}

func TestArgsNotifySend(t *testing.T) {
	bin, args := Args(BackendNotifySend, Note{Subtitle: "Needs you · Dana", Message: "-rf <b>bold</b> & co\nline", Group: "doppel:handoff:dm:1", Urgent: true})
	got := strings.Join(args, "|")
	want := "-a|WhatsApp Doppel|-i|whatsapp-doppel|-u|critical|-h|string:desktop-entry:whatsapp-doppel" +
		"|-h|string:x-canonical-private-synchronous:doppel:handoff:dm:1" +
		"|--|WhatsApp Doppel · Needs you · Dana|-rf &lt;b&gt;bold&lt;/b&gt; &amp; co line"
	if bin != "notify-send" || got != want {
		t.Errorf("args = %s\n%s\nwant\n%s", bin, got, want)
	}
	_, args = Args(BackendNotifySend, Note{Message: "x"})
	if got := strings.Join(args, "|"); !strings.Contains(got, "-u|normal") || strings.Contains(got, "synchronous") ||
		!strings.HasSuffix(got, "|--|WhatsApp Doppel|x") {
		t.Errorf("plain args = %s", got)
	}
}

// decodePS reverses -EncodedCommand (base64 of UTF-16LE).
func decodePS(t *testing.T, enc string) string {
	t.Helper()
	b, err := base64.StdEncoding.DecodeString(enc)
	if err != nil || len(b)%2 != 0 {
		t.Fatalf("bad base64: %v", err)
	}
	r := make([]rune, 0, len(b)/2)
	for i := 0; i < len(b); i += 2 {
		r = append(r, rune(binary.LittleEndian.Uint16(b[i:])))
	}
	return string(r)
}

func TestArgsPowerShellEscaping(t *testing.T) {
	note := Note{Subtitle: `Needs you · "Dana" <3`, Message: "she said '@ & <script>\n'@ hi", Group: "doppel:approval:dm:972500000000",
		Open: "http://127.0.0.1:7788/#/chats/dm:1?a=1&b=2", Sound: false}
	bin, args := Args(BackendPowerShell, note)
	if bin != "powershell.exe" || len(args) != 6 || args[4] != "-EncodedCommand" || args[0] != "-NoProfile" {
		t.Fatalf("args = %s %q", bin, args)
	}
	script := decodePS(t, args[5])
	start, end := strings.Index(script, "@'\n"), strings.Index(script, "\n'@)")
	if start < 0 || end < start {
		t.Fatalf("no here-string in:\n%s", script)
	}
	toast := script[start+3 : end]
	if strings.Contains(toast, "\n") {
		t.Errorf("toast XML must be one line: %q", toast)
	}
	for _, want := range []string{
		`launch="http://127.0.0.1:7788/#/chats/dm:1?a=1&amp;b=2"`,
		`<text>Needs you · &#34;Dana&#34; &lt;3</text>`,
		`<text>she said &#39;@ &amp; &lt;script&gt; &#39;@ hi</text>`,
		`<audio silent="true"/>`,
	} {
		if !strings.Contains(toast, want) {
			t.Errorf("toast lacks %s:\n%s", want, toast)
		}
	}
	if strings.Contains(toast, "<script>") {
		t.Error("raw markup from the message leaked into the XML")
	}
	if !strings.Contains(script, "$t.Tag = '") || !strings.Contains(script, `CreateToastNotifier('{1AC14E77-02E7-4E5D-B744-2EB1AE5198B7}\WindowsPowerShell\v1.0\powershell.exe')`) {
		t.Errorf("script:\n%s", script)
	}
	_, args = Args(BackendPowerShell, Note{Message: "x", Sound: true, Urgent: true, Open: "http://h/"})
	script = decodePS(t, args[5])
	if strings.Contains(script, "audio") || strings.Contains(script, "Tag") || !strings.Contains(script, `scenario="reminder"`) || !strings.Contains(script, "<actions>") {
		t.Errorf("urgent with sound:\n%s", script)
	}
}
