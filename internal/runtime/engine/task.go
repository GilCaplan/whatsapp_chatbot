package engine

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"whatsappdoppel/internal/core/events"
	"whatsappdoppel/internal/core/model"
	"whatsappdoppel/internal/core/store"
	"whatsappdoppel/internal/logic/calendar"
	"whatsappdoppel/internal/logic/goals"
	"whatsappdoppel/internal/logic/guard"
	"whatsappdoppel/internal/logic/handoff"
	"whatsappdoppel/internal/logic/task"
	"whatsappdoppel/internal/runtime/llm"
	"whatsappdoppel/internal/runtime/prompt"
)

// Tasks (internal/logic/task, model.TaskRun): an errand in a chat, like agreeing
// on a meeting time. While a task runs it is the chat's goal: each reply
// cycle first reads their new messages into JSON (prompt.TaskRead), code
// picks the rule and works out the times (task.Decide: the calendar stays
// in code), the reply prompt gets only what to say (prompt.TaskSection) and
// every draft passes task.Leak before it can go out. Confirming needs your
// approval unless the task auto-confirms free times; booking goes to
// MACalendar's API or an .ics file. A timer per chat nudges a silent chat
// and hands over to you when the task runs out of nudges, messages or days.

const (
	taskReadTimeout = 45 * time.Second
	taskReadContext = 6 // earlier messages the reading step sees
	taskRetryBusy   = 30 * time.Second
	taskRetryLater  = time.Hour // after a failed nudge or confirmation
)

var (
	// ErrTaskRunning means the chat already has a running task.
	ErrTaskRunning = task.ErrRunning
	// ErrTaskInvalid wraps a spec that doesn't pass task.Normalize.
	ErrTaskInvalid = task.ErrInvalid
	// ErrTaskState means the run isn't in a state for that action.
	ErrTaskState = task.ErrState
)

func taskJob(key string) string { return "task:" + key }

// taskDefaults are settings.tasks for internal/logic/task.
func (e *Engine) taskDefaults() task.Settings {
	t := e.cfg.Get().Tasks
	return task.Settings{Privacy: t.Privacy, Limits: t.Limits, Commit: t.Commit}
}

func (e *Engine) publishTasks(chatKey string) {
	if e.hub != nil {
		e.hub.Publish(events.TypeTasksChanged, map[string]string{"chatKey": chatKey})
	}
}

func taskMeta(run model.TaskRun, kind string, notify bool) map[string]any {
	m := map[string]any{"taskId": run.ID, "kind": kind, "status": run.Status}
	if notify {
		m["notify"] = true
	}
	return m
}

// editRun changes the stored run in place, atomically (fn returns false to
// leave it as it is); ok=false when it is gone or fn declined. Every write
// goes through here or through a reply's delivery, so it always works on the
// latest run.
func (e *Engine) editRun(id string, fn func(r *model.TaskRun) bool) (model.TaskRun, bool) {
	now := e.clock.Now()
	out, ok, err := e.store.EditTaskRun(id, func(r *model.TaskRun) bool {
		if !fn(r) {
			return false
		}
		r.UpdatedAt = now
		return true
	})
	if err != nil {
		if c, found := e.store.Chat(out.ChatKey); found {
			e.act(model.ActError, c, "", "Couldn't save the task: "+err.Error(), nil)
		}
		return out, false
	}
	if ok {
		e.publishTasks(out.ChatKey)
	}
	return out, ok
}

// cloneRun is a deep copy (a rollback must not share maps or slices).
func cloneRun(r model.TaskRun) model.TaskRun {
	var out model.TaskRun
	b, _ := json.Marshal(r)
	_ = json.Unmarshal(b, &out)
	return out
}

func (e *Engine) cancelTaskJobs(chatKey string) {
	e.cancelJob(taskJob(chatKey))
	e.cancelJob(taskJob(chatKey) + ":speak")
}

// StartTask starts a task in a private chat. from: their messages after it
// are read (zero = from now on). open = write first now.
func (e *Engine) StartTask(ctx context.Context, chatKey string, spec model.TaskSpec, open bool) (model.TaskRun, error) {
	return e.startTask(ctx, chatKey, spec, open, time.Time{})
}

func (e *Engine) startTask(ctx context.Context, chatKey string, spec model.TaskSpec, open bool, from time.Time) (model.TaskRun, error) {
	c, ok := e.store.Chat(chatKey)
	if !ok {
		return model.TaskRun{}, fmt.Errorf("chat %q: %w", chatKey, ErrNotFound)
	}
	if isGroupChat(c) {
		return model.TaskRun{}, fmt.Errorf("%w: tasks run in private chats for now", ErrTaskInvalid)
	}
	if !c.Enabled {
		return model.TaskRun{}, ErrChatDisabled
	}
	if _, ok := e.store.ActiveTask(chatKey); ok {
		return model.TaskRun{}, ErrTaskRunning
	}
	if err := task.Normalize(&spec); err != nil {
		return model.TaskRun{}, fmt.Errorf("%w: %s", ErrTaskInvalid, err.Error())
	}
	spec = task.Resolve(spec, e.taskDefaults())
	s := e.cfg.Get().Tasks
	owner := s.Owner
	if _, sec := e.secretaryOf(c); sec != nil && strings.TrimSpace(owner.Name) == "" {
		owner.Name = sec.OwnerName
	}
	now := e.clock.Now()
	if from.IsZero() {
		from = now
	}
	run := model.TaskRun{
		ID: store.NewID(), ChatKey: chatKey, ChatName: c.Name, Spec: spec, Owner: owner, Tone: s.Tone,
		Status: model.TaskActive, Step: model.StepOpen, Values: map[string]model.TaskValue{}, Fired: map[string]int{},
		Offered: []time.Time{}, LastOffer: []time.Time{}, Answers: []model.TaskAnswer{},
		Log:    []model.TaskEvent{{At: now, Kind: "started", Text: "Started: " + spec.Title}},
		ReadTo: from, StartedAt: now, UpdatedAt: now,
	}
	if err := e.store.AddTaskRun(run); err != nil { // checks again under the lock: one task per chat
		if errors.Is(err, store.ErrTaskRunning) {
			return model.TaskRun{}, ErrTaskRunning
		}
		return model.TaskRun{}, err
	}
	e.act(model.ActTask, c, "", "Task started: "+spec.Title, taskMeta(run, "started", false))
	e.publishTasks(chatKey)
	e.armTask(run)
	if open {
		e.taskSpeak(ctx, chatKey)
	}
	return run, nil
}

// taskSpeak starts a reply cycle for the task (an opener, a nudge, the
// confirmation); if the persona is busy it tries again shortly.
func (e *Engine) taskSpeak(ctx context.Context, chatKey string) {
	if !e.taskRunning(chatKey) {
		return // stopped or ended meanwhile: nothing to say
	}
	err := e.Initiate(ctx, chatKey, "")
	if errors.Is(err, ErrReplying) {
		e.schedule(taskJob(chatKey)+":speak", taskRetryBusy, func(ctx context.Context) { e.taskSpeak(ctx, chatKey) })
		return
	}
	if err != nil {
		if c, ok := e.store.Chat(chatKey); ok {
			e.act(model.ActError, c, "", "Couldn't write for the task: "+err.Error(), nil)
		}
		if run, ok := e.store.ActiveTask(chatKey); ok {
			e.armTaskAfter(run, taskRetryLater) // keep the timer, but not right away (the chat may be off)
		}
	}
}

// StopTask ends a running task (you stopped it).
func (e *Engine) StopTask(runID string) (model.TaskRun, error) {
	run, ok := e.store.TaskRun(runID)
	if !ok {
		return model.TaskRun{}, fmt.Errorf("task %q: %w", runID, ErrNotFound)
	}
	if !run.Active() {
		return run, nil
	}
	run, ok = e.editRun(runID, func(r *model.TaskRun) bool {
		if !r.Active() {
			return false
		}
		e.endRun(r, model.TaskStopped, "You stopped it")
		return true
	})
	if !ok {
		run, _ = e.store.TaskRun(runID)
		return run, nil
	}
	e.cancelTaskJobs(run.ChatKey)
	if c, ok := e.store.Chat(run.ChatKey); ok {
		e.act(model.ActTask, c, "", "Task stopped: "+run.Spec.Title, taskMeta(run, "stopped", false))
	}
	return run, nil
}

func (e *Engine) endRun(run *model.TaskRun, status, why string) {
	now := e.clock.Now()
	run.Status, run.Step, run.EndedAt = status, model.StepEnded, &now
	run.Summary = task.Summary(run, run.Tone, time.Local, why)
	run.Log = append(run.Log, model.TaskEvent{At: now, Kind: "ended", Text: why})
}

// ApproveTask answers a confirmation waiting for you: yes books it and
// confirms in the chat; no hands the chat over to you.
func (e *Engine) ApproveTask(ctx context.Context, runID string, yes bool) (model.TaskRun, error) {
	run, ok := e.store.TaskRun(runID)
	if !ok {
		return model.TaskRun{}, fmt.Errorf("task %q: %w", runID, ErrNotFound)
	}
	if run.Status != model.TaskApproval || run.Proposed == nil {
		return run, ErrTaskState
	}
	c, ok := e.store.Chat(run.ChatKey)
	if !ok {
		return run, fmt.Errorf("chat %q: %w", run.ChatKey, ErrNotFound)
	}
	now := e.clock.Now()
	at := *run.Proposed
	waiting := func(r *model.TaskRun) bool {
		return r.Status == model.TaskApproval && r.Proposed != nil && r.Proposed.Equal(at)
	}
	if !yes {
		run, ok = e.editRun(runID, func(r *model.TaskRun) bool {
			if !waiting(r) {
				return false
			}
			e.endRun(r, model.TaskHandedOff, "You didn't approve "+task.When(at.In(time.Local), r.Tone, ""))
			return true
		})
		if !ok {
			return run, ErrTaskState
		}
		e.cancelTaskJobs(run.ChatKey)
		e.taskPause(c, run)
		return run, nil
	}
	a, _ := e.taskAvail(ctx, run)
	switch {
	case !a.Known:
		return run, errors.New("couldn't read your calendar right now: try again in a moment")
	case !a.Works(at):
		return run, errors.New("that time isn't free any more: the calendar changed")
	}
	booked := cloneRun(run)
	if err := e.taskBook(ctx, &booked); err != nil {
		e.editRun(runID, func(r *model.TaskRun) bool {
			r.Log = append(r.Log, model.TaskEvent{At: now, Kind: "note", Text: "Booking failed: " + err.Error()})
			return true
		})
		return run, err
	}
	run, ok = e.editRun(runID, func(r *model.TaskRun) bool {
		if !waiting(r) {
			return false
		}
		r.Result = booked.Result
		r.Log = append(r.Log, booked.Log[len(run.Log):]...)
		r.Log = append(r.Log, model.TaskEvent{At: now, Kind: "commit", Text: "You approved it"})
		r.Status, r.Next, r.NextTimes = model.TaskActive, task.SayConfirmed, []time.Time{at}
		return true
	})
	if !ok {
		return run, ErrTaskState
	}
	e.act(model.ActTask, c, "", "Approved: "+task.When(run.Proposed.In(time.Local), run.Tone, "")+" — confirming in the chat", taskMeta(run, "approved", false))
	e.taskSpeak(ctx, run.ChatKey)
	return run, nil
}

// taskBook books the agreed time as the task says and fills its result.
func (e *Engine) taskBook(ctx context.Context, run *model.TaskRun) error {
	at := *run.Proposed
	end := at.Add(time.Duration(run.Spec.Meeting.Minutes) * time.Minute)
	res := &model.TaskResult{OK: true, When: &at, End: &end, Values: map[string]string{}}
	for _, s := range run.Spec.Slots {
		if v, ok := run.Values[s.Key]; ok && s.Kind != model.SlotWhen {
			res.Values[s.Label] = v.Value
		}
	}
	switch run.Spec.Commit.Book {
	case model.BookICS:
		res.Booked = model.BookICS
	case model.BookMACalendar:
		src, ok := e.bookingCalendar(run.Spec.Commit.CalendarID)
		if !ok {
			return errors.New("no MACalendar calendar is connected (Settings › Calendars)")
		}
		if e.cal == nil {
			return errors.New("calendars are not available")
		}
		id, err := e.cal.BookMACalendar(ctx, src, taskEventTitle(*run), at, end, run.ID)
		if err != nil {
			return err
		}
		res.Booked, res.EventID = model.BookMACalendar, id
	}
	run.Result = res
	run.Log = append(run.Log, model.TaskEvent{At: e.clock.Now(), Kind: "booked", Text: "Agreed on " + task.When(at.In(time.Local), run.Tone, "") + bookedText(res.Booked)})
	return nil
}

func bookedText(b string) string {
	switch b {
	case model.BookMACalendar:
		return ", booked in MACalendar"
	case model.BookICS:
		return ", calendar file ready"
	}
	return ""
}

func (e *Engine) bookingCalendar(id string) (model.CalendarSource, bool) {
	for _, s := range e.store.Calendars() {
		if s.Kind == model.CalendarMACalendar && s.Enabled && (id == "" || s.ID == id) {
			return s, true
		}
	}
	return model.CalendarSource{}, false
}

func taskEventTitle(run model.TaskRun) string {
	what := run.Spec.About
	if what == "" {
		what = "Meeting"
	}
	if run.ChatName != "" {
		what += " with " + run.ChatName
	}
	return guard.Truncate(what, 120)
}

// TaskICS is the calendar file of a booked run.
func (e *Engine) TaskICS(runID string) ([]byte, error) {
	run, ok := e.store.TaskRun(runID)
	if !ok {
		return nil, fmt.Errorf("task %q: %w", runID, ErrNotFound)
	}
	if run.Result == nil || run.Result.When == nil || run.Result.End == nil {
		return nil, ErrTaskState
	}
	return calendar.EventICS(run.ID, taskEventTitle(run), run.Spec.Place, *run.Result.When, *run.Result.End, e.clock.Now()), nil
}

// taskAvail is when the run's meeting can happen, and (for the leak check
// only) the titles of your events in that window.
func (e *Engine) taskAvail(ctx context.Context, run model.TaskRun) (task.Availability, []string) {
	now := e.clock.Now()
	priv := model.TaskPrivacy{}
	if run.Spec.Privacy != nil {
		priv = *run.Spec.Privacy
	}
	a := task.For(run.Spec.Meeting, priv, nil, true, now, time.Local)
	if !run.Spec.Meeting.Calendars || e.cal == nil {
		return a, nil
	}
	srcs := e.store.Calendars()
	cctx, cancel := context.WithTimeout(ctx, calendarWait)
	defer cancel()
	from, to := a.Start.Add(-24*time.Hour), a.End
	busy, sts := e.cal.Busy(cctx, srcs, from, to, true)
	if len(sts) > 0 && !anyOK(sts) {
		a.Known = false
	}
	a.Busy = busy
	return a, e.cal.Titles(cctx, srcs, from, to)
}

// taskUse is a task's part of one reply: what code decided and what the
// leak check needs.
type taskUse struct {
	run     model.TaskRun // after the turn, saved once its message goes out
	before  model.TaskRun // as read (rev, status: what a delivery checks against)
	turn    task.Turn
	prompt  prompt.TaskTurn
	allowed []time.Time
	titles  []string
	exempt  []string
	code    string
}

// theirNew are their messages after the last one read (by id: WhatsApp
// times have one-second steps), else from the run's read time on.
func theirNew(hist []model.Message, run model.TaskRun) []model.Message {
	start := 0
	if i := slices.IndexFunc(hist, func(m model.Message) bool { return run.ReadMsg != "" && m.ID == run.ReadMsg }); i >= 0 {
		start = i + 1
	}
	var out []model.Message
	for _, m := range hist[start:] {
		if m.Speaker == "them" && (start > 0 || !m.TS.Before(run.ReadTo)) && strings.TrimSpace(m.Text) != "" {
			out = append(out, m)
		}
	}
	return out
}

// taskTurn runs the task step of a reply cycle; nil when the chat has no
// running task. code is the reply language. Nothing is saved here: the turn
// is saved when its message goes out (taskDelivered); a draft that is
// dropped leaves the run as it was (taskDropped), so the next cycle reads
// their messages again. A booking made here is safe to repeat (MACalendar
// books one event per run id).
func (e *Engine) taskTurn(ctx context.Context, c model.ChatAssignment, p model.Persona, hist []model.Message, proactive bool, code string) *taskUse {
	stored, ok := e.store.ActiveTask(c.Key)
	if !ok {
		return nil
	}
	run := cloneRun(stored)
	now := e.clock.Now()
	a, titles := e.taskAvail(ctx, run)
	var t task.Turn
	fresh := theirNew(hist, run)
	switch {
	case proactive && run.Next != "":
		t = task.Turn{Say: run.Next, Times: run.NextTimes, Rule: -1}
		if run.Next == task.SayConfirmed {
			t.Effects = []string{task.DoFinishOK}
		}
		run.Next, run.NextTimes = "", nil
	case run.Status == model.TaskApproval:
		t = task.Turn{Say: task.SayHold, Rule: -1}
		if run.Proposed != nil {
			t.Times = []time.Time{*run.Proposed}
		}
	case proactive && run.Step == model.StepOpen:
		t = task.Decide(&run, task.Input{Event: task.EvStarted, Now: now}, a, e.rng)
	case len(fresh) == 0:
		t = task.Turn{Say: task.SayChat, Rule: -1}
	case !a.Known:
		// The calendar can't be read: don't judge their times (nothing is
		// marked as not working), and read these messages again next time.
		t = task.Turn{Say: task.SayCheck, Rule: -1}
		run.Log = append(run.Log, model.TaskEvent{At: now, Kind: "note", Text: "Couldn't read your calendar, so it said it would check"})
		e.act(model.ActError, c, p.Name, "The task couldn't read your calendar: it says it will get back to them", taskMeta(run, "note", false))
	default:
		texts := make([]string, len(fresh))
		for i, m := range fresh {
			texts[i] = m.Text
		}
		reading := e.taskRead(ctx, c, p, run, hist, fresh)
		last := fresh[len(fresh)-1]
		run.Log = append(run.Log, model.TaskEvent{At: now, Kind: "read", MsgID: last.ID,
			Text: "Read their message as \"" + reading.Intent + "\"" + readTimes(reading)})
		t = task.Decide(&run, task.Input{Event: task.EvMessage, Reading: reading, Their: texts, MsgID: last.ID, Now: now}, a, e.rng)
		run.ReadTo, run.ReadMsg, run.LastTheirAt = last.TS, last.ID, last.TS
	}
	if t.Commit {
		e.taskCommit(ctx, c, &run, &t, a)
	}
	if t.Say != "" {
		run.Sent++
		run.LastOursAt = now
	}
	u := &taskUse{run: run, before: stored, turn: t, titles: titles, code: code, allowed: task.Allowed(&run, t)}
	u.exempt = append([]string{run.Spec.About, run.Spec.Place, run.Spec.Title, run.Owner.About}, run.Spec.Privacy.CoverLines...)
	u.prompt = e.taskPrompt(run, t, code)
	return u
}

// taskDropped: a turn's message never went out (new messages while
// thinking, an error, the reply check). The run is untouched; a message the
// engine started itself (a nudge, the confirmation) is tried again later.
func (e *Engine) taskDropped(u *taskUse) {
	if u == nil {
		return
	}
	run, ok := e.store.TaskRun(u.run.ID)
	if !ok || !run.Active() {
		return
	}
	if run.Next != "" {
		key := run.ChatKey
		e.schedule(taskJob(key)+":speak", taskRetryBusy, func(ctx context.Context) { e.taskSpeak(ctx, key) })
		return
	}
	e.armTask(run)
}

func readTimes(r model.TaskReading) string {
	var ts []string
	for _, p := range r.Times {
		ts = append(ts, strings.TrimSpace(p.Date+" "+cmp.Or(p.Time, p.Part)))
	}
	if len(ts) == 0 {
		return ""
	}
	return " (" + strings.Join(ts, ", ") + ")"
}

// actTurn logs what the task decided (activity text: plain words, the
// times it will say; never anything else from the calendar).
func (e *Engine) actTurn(c model.ChatAssignment, run model.TaskRun, t task.Turn) {
	what := map[string]string{
		task.SayOffer: "offering times", task.SayWindows: "offering parts of days", task.SayAskTimes: "asking when suits them",
		task.SayAnswerNo: "saying that time doesn't work", task.SayAskConfirm: "asking them to confirm", task.SayAskWhich: "asking which time",
		task.SayAskSlot: "asking for a detail", task.SayCover: "not saying why", task.SayChat: "answering, then back to the task",
		task.SayNudge: "a friendly nudge", task.SayWait: "giving them time", task.SayConfirmed: "confirming",
		task.SayHold: "holding until you approve", task.SayHandoff: "handing over to you", task.SayGoodbye: "closing for now",
	}[t.Say]
	if what == "" {
		return
	}
	var ts []string
	for _, x := range t.Times {
		ts = append(ts, task.When(x.In(time.Local), run.Tone, ""))
	}
	text := run.Spec.Title + ": " + what
	if len(ts) > 0 {
		text += " (" + strings.Join(ts, ", ") + ")"
	}
	meta := taskMeta(run, "turn", false)
	meta["say"] = t.Say
	if t.Rule >= 0 {
		meta["rule"] = t.Rule + 1
	}
	e.act(model.ActTask, c, "", text, meta)
}

// taskRead runs the reading step; any failure reads as "other" (the task
// then just answers and steers back).
func (e *Engine) taskRead(ctx context.Context, c model.ChatAssignment, p model.Persona, run model.TaskRun, hist, fresh []model.Message) model.TaskReading {
	other := model.TaskReading{Intent: "other"}
	prov, mdl, err := e.llm.Resolve(p.LLM)
	if err != nil {
		return other
	}
	var earlier []model.Message
	if i := slices.IndexFunc(hist, func(m model.Message) bool { return m.ID == fresh[0].ID }); i > 0 {
		earlier = hist[max(0, i-taskReadContext):i]
	}
	var offered []string
	for _, t := range run.LastOffer {
		offered = append(offered, task.When(t.In(time.Local), run.Tone, ""))
	}
	in := prompt.TaskReadInput{About: run.Spec.About, Step: run.Step, Offered: offered, Earlier: earlier, Zone: time.Local, Now: e.clock.Now()}
	if run.Proposed != nil {
		in.Proposed = task.When(run.Proposed.In(time.Local), run.Tone, "")
	}
	for _, s := range run.Spec.Slots {
		if s.Kind != model.SlotWhen {
			in.Slots = append(in.Slots, s)
		}
	}
	for _, m := range fresh {
		in.New = append(in.New, guard.Sanitize(m.Text))
	}
	req := prompt.TaskRead(in)
	req.Model = mdl
	cctx, cancel := context.WithTimeout(ctx, taskReadTimeout)
	defer cancel()
	resp, err := prov.Chat(cctx, req)
	if err != nil {
		if ctx.Err() == nil {
			e.act(model.ActError, c, p.Name, "The task couldn't read their message: "+err.Error(), nil)
		}
		return other
	}
	r, err := prompt.ParseTaskRead(resp.Text)
	if err != nil {
		return other
	}
	return r
}

// taskCommit confirms the proposed time: still free → auto-confirm (book,
// confirm) or wait for your approval; taken meanwhile → say so and offer
// another.
func (e *Engine) taskCommit(ctx context.Context, c model.ChatAssignment, run *model.TaskRun, t *task.Turn, a task.Availability) {
	at := *run.Proposed
	now := e.clock.Now()
	if a.Known && !a.Works(at) {
		run.Answers = append(run.Answers, model.TaskAnswer{Time: at, Works: false})
		run.Step, run.Proposed = model.StepNegotiate, nil
		t.Commit, t.Say, t.Their, t.Times = false, task.SayAnswerNo, &at, nil
		if alt, ok := a.Alternative(at, run.Offered, e.rng); ok {
			t.Times = []time.Time{alt}
			run.LastOffer = []time.Time{alt}
			run.Offered = append(run.Offered, alt)
		}
		run.Log = append(run.Log, model.TaskEvent{At: now, Kind: "note", Text: "That time was taken meanwhile"})
		return
	}
	t.Times = []time.Time{at}
	if !a.Known {
		run.Log = append(run.Log, model.TaskEvent{At: now, Kind: "note", Text: "Couldn't read your calendar to check it once more: it waits for you"})
	} else if run.Spec.Commit.Mode == model.CommitAutoFree {
		if err := e.taskBook(ctx, run); err == nil {
			t.Say = task.SayConfirmed
			t.Effects = append(t.Effects, task.DoFinishOK)
			return
		} else {
			run.Log = append(run.Log, model.TaskEvent{At: now, Kind: "note", Text: "Booking failed, waiting for you: " + err.Error()})
		}
	}
	run.Status = model.TaskApproval
	t.Say = task.SayHold
	run.Log = append(run.Log, model.TaskEvent{At: now, Kind: "commit", Text: "Waiting for your OK"})
}

// taskPrompt writes a turn out for the reply prompt.
func (e *Engine) taskPrompt(run model.TaskRun, t task.Turn, code string) prompt.TaskTurn {
	when := func(x time.Time) string { return task.When(x.In(time.Local), run.Tone, code) }
	pt := prompt.TaskTurn{Say: t.Say, Voice: run.Spec.Voice, Owner: run.Owner.Name, OwnerAbout: run.Owner.About,
		About: run.Spec.About, Place: run.Spec.Place, Minutes: run.Spec.Meeting.Minutes, Cover: t.Cover, Tone: run.Tone}
	for _, x := range t.Times {
		pt.Times = append(pt.Times, when(x))
	}
	for _, w := range t.Windows {
		pt.Windows = append(pt.Windows, task.WindowText(w, run.Tone, code))
	}
	if t.Their != nil {
		pt.Their = when(*t.Their)
	}
	if t.TheirDay != nil {
		pt.TheirDay = task.Day(*t.TheirDay, run.Tone, code)
		if t.TheirPart != "" {
			pt.TheirDay += " " + t.TheirPart
		}
	}
	if t.Slot != nil {
		pt.Slot, pt.Choices = t.Slot.Label, t.Slot.Choices
	}
	return pt
}

// taskGuard checks a draft against the task's rules: a leaky draft is
// written again once, then replaced by a safe message from code.
func (e *Engine) taskGuard(c model.ChatAssignment, u *taskUse, res genResult, retry func(note string) (genResult, error)) genResult {
	in := task.LeakInput{Reply: res.Text, Allowed: u.allowed, Days: task.AllowedDays(u.turn), Loc: time.Local, Titles: u.titles,
		Exempt: u.exempt, Reasons: u.turn.Say != task.SayConfirmed}
	problems := task.Leak(in)
	if len(problems) == 0 {
		return res
	}
	meta := taskMeta(u.run, "leak", false)
	meta["problems"] = problems
	e.act(model.ActTask, c, "", "The draft broke the task rules ("+strings.Join(problems, "; ")+") — writing it again", meta)
	if again, err := retry(prompt.TaskRetryNote(problems)); err == nil {
		in.Reply = again.Text
		if len(task.Leak(in)) == 0 {
			again.Drafts = nil
			return again
		}
	}
	res.Text, res.Drafts = task.Fallback(u.turn, u.run.Tone, u.code, time.Local), nil
	e.act(model.ActTask, c, "", "Sent a safe message instead", taskMeta(u.run, "fallback", false))
	return res
}

// taskDelivered saves a turn once its message went out (or waits in
// Approvals), then applies its effects: done, failed or handed over to you;
// else the timer is armed again. If the run changed meanwhile, a Stop or an
// ended run wins, and so does your approval (the message only counts).
func (e *Engine) taskDelivered(c model.ChatAssignment, u *taskUse) {
	if u == nil {
		return
	}
	now := e.clock.Now()
	run, ok := e.editRun(u.run.ID, func(r *model.TaskRun) bool {
		switch {
		case !r.Active():
			return false
		case r.Rev != u.before.Rev && r.Status != u.before.Status:
			r.Sent++
			r.LastOursAt = now
		default:
			next := cloneRun(u.run)
			next.Rev = r.Rev
			*r = next
		}
		return true
	})
	if !ok {
		return
	}
	e.lastTask.Store(c.Key, u) // Regenerate on a pending approval writes this turn again
	e.actTurn(c, run, u.turn)
	if run.Status == model.TaskApproval && u.before.Status != model.TaskApproval && run.Proposed != nil {
		e.act(model.ActTask, c, "", c.Name+" agreed to "+task.When(run.Proposed.In(time.Local), run.Tone, "")+" — approve it to confirm",
			taskMeta(run, "approval", true))
	}
	var status, why, kind string
	switch {
	case u.turn.Has(task.DoFinishOK):
		status, why, kind = model.TaskDone, "Done", "done"
	case u.turn.Has(task.DoHandoff):
		status, why, kind = model.TaskHandedOff, handoffWhy(u.turn), "handoff"
	case u.turn.Has(task.DoFinishFail):
		status, why, kind = model.TaskFailed, "Ended without a time", "failed"
	}
	if status == "" {
		if u.turn.Has(task.DoNotify) {
			e.act(model.ActTask, c, "", run.Spec.Title+": "+task.Summary(&run, run.Tone, time.Local, ""), taskMeta(run, "note", true))
		}
		e.armTask(run)
		return
	}
	run, ok = e.editRun(u.run.ID, func(r *model.TaskRun) bool {
		if !r.Active() {
			return false
		}
		e.endRun(r, status, why)
		return true
	})
	if !ok {
		return
	}
	e.cancelTaskJobs(c.Key)
	switch kind {
	case "done":
		text := "Task done: " + run.Spec.Title
		if r := run.Result; r != nil && r.When != nil {
			text += " — " + task.When(r.When.In(time.Local), run.Tone, "") + bookedText(r.Booked)
		}
		e.act(model.ActTask, c, "", text, taskMeta(run, "done", true))
	case "handoff":
		e.taskPause(c, run)
	case "failed":
		e.act(model.ActTask, c, "", "Task ended without a time: "+run.Spec.Title, taskMeta(run, "failed", true))
	}
}

func handoffWhy(t task.Turn) string {
	switch {
	case slices.Contains(t.Facts, "intent:money"):
		return "They brought up money"
	case slices.Contains(t.Facts, task.CondProbeLimit):
		return "They kept asking about times that don't work"
	case slices.Contains(t.Facts, task.CondLimit):
		return "It reached the message limit"
	case slices.Contains(t.Facts, task.CondOutOfNudges):
		return "They didn't answer"
	case slices.Contains(t.Facts, "intent:bot_menu"):
		return "The other side is an automated menu"
	}
	return "It needs you"
}

// taskPause hands the chat over to you: paused like a hand-off, with the
// task's summary as the reason.
func (e *Engine) taskPause(c model.ChatAssignment, run model.TaskRun) {
	now := e.clock.Now()
	hs := &model.HandoffState{Category: model.HandoffTask, Excerpt: guard.Truncate(run.Summary, 160), Sender: c.Name, At: now, How: model.HandoffHowTask}
	if _, err := e.store.UpdateChat(c.Key, func(x *model.ChatAssignment) { x.Handoff = hs }); err != nil {
		e.act(model.ActError, c, "", "Couldn't save the pause: "+err.Error(), nil)
	}
	if r := e.runner(c.Key); r != nil {
		r.mu.Lock()
		r.cfg.Handoff = hs
		r.handoffNoted = time.Time{}
		r.mu.Unlock()
	}
	meta := taskMeta(run, "handoff", false)
	meta["category"], meta["excerpt"], meta["how"], meta["sender"] = model.HandoffTask, hs.Excerpt, model.HandoffHowTask, c.Name
	e.act(model.ActHandoff, c, "", run.Spec.Title+": "+strings.ToLower(firstSentence(run.Summary))+" — paused so you can take over", meta)
	if e.hub != nil {
		e.hub.Publish(events.TypeChatsChanged, struct{}{})
	}
}

func firstSentence(s string) string {
	if i := strings.Index(s, "."); i > 0 {
		return s[:i]
	}
	return s
}

// armTask (re)arms the chat's task timer: the next nudge while we wait for
// them, and the task's last day.
func (e *Engine) armTask(run model.TaskRun) { e.armTaskAfter(run, 0) }

// armTaskAfter is armTask, but not sooner than min from now.
func (e *Engine) armTaskAfter(run model.TaskRun, min time.Duration) {
	if !run.Active() {
		return
	}
	lim := model.TaskLimits{MaxDays: 7, NudgeAfterHours: 24}
	if run.Spec.Limits != nil {
		lim = *run.Spec.Limits
	}
	now := e.clock.Now()
	due := run.StartedAt.AddDate(0, 0, lim.MaxDays)
	if run.Status == model.TaskActive && run.LastOursAt.After(run.LastTheirAt) {
		if nudge := run.LastOursAt.Add(time.Duration(lim.NudgeAfterHours) * time.Hour); nudge.Before(due) {
			due = nudge
		}
	}
	key, id := run.ChatKey, run.ID
	e.schedule(taskJob(key), max(due.Sub(now), min), func(ctx context.Context) { e.taskTimer(ctx, id) })
}

// taskTimer fires for a silent chat or the task's last day.
func (e *Engine) taskTimer(ctx context.Context, runID string) {
	run, ok := e.store.TaskRun(runID)
	if !ok || !run.Active() {
		return
	}
	c, ok := e.store.Chat(run.ChatKey)
	if !ok {
		return
	}
	now := e.clock.Now()
	lim := *run.Spec.Limits
	end := func(status, why string) (model.TaskRun, bool) {
		return e.editRun(runID, func(r *model.TaskRun) bool {
			if !r.Active() {
				return false
			}
			e.endRun(r, status, why)
			return true
		})
	}
	if !now.Before(run.StartedAt.AddDate(0, 0, lim.MaxDays)) {
		if run, ok := end(model.TaskHandedOff, "It ran out of time"); ok {
			e.cancelTaskJobs(c.Key)
			e.taskPause(c, run)
		}
		return
	}
	if run.Status != model.TaskActive || !run.LastOursAt.After(run.LastTheirAt) {
		e.armTask(run)
		return
	}
	rev := run.Rev
	a, _ := e.taskAvail(ctx, run)
	t := task.Decide(&run, task.Input{Event: task.EvNoReply, Now: now}, a, e.rng)
	switch {
	case t.Has(task.DoHandoff):
		if run, ok := end(model.TaskHandedOff, handoffWhy(t)); ok {
			e.cancelTaskJobs(c.Key)
			e.taskPause(c, run)
		}
	case t.Has(task.DoFinishFail):
		if run, ok := end(model.TaskFailed, "They didn't answer"); ok {
			e.cancelTaskJobs(c.Key)
			e.act(model.ActTask, c, "", "Task ended without a time: "+run.Spec.Title, taskMeta(run, "failed", true))
		}
	case t.Say != "":
		run.Next, run.NextTimes, run.UpdatedAt = t.Say, t.Times, now
		if _, err := e.store.SwapTaskRun(run, rev); err != nil {
			if fresh, ok := e.store.TaskRun(runID); ok {
				e.armTask(fresh) // it changed meanwhile (they wrote, you acted): look again later
			}
			return
		}
		e.publishTasks(c.Key)
		e.taskSpeak(ctx, run.ChatKey)
	default:
		e.armTask(run) // no rule for silence: keep the timer for the last day
	}
}

// armTasks re-arms every running task's timer (app start).
func (e *Engine) armTasks() {
	for _, r := range e.store.TaskRuns() {
		if r.Active() {
			e.armTask(r)
		}
	}
}

// taskRunning reports whether the chat has a running task (the task is its
// goal: no goal planning, no check-ins or scheduled messages meanwhile).
func (e *Engine) taskRunning(key string) bool {
	_, ok := e.store.ActiveTaskID(key)
	return ok
}

// secretaryMeeting starts a Find a time task when someone asks a secretary
// for a meeting (its meeting days, hours and length; you confirm).
func (e *Engine) secretaryMeeting(c model.ChatAssignment, in model.Incoming, text string) {
	_, s := e.secretaryOf(c)
	if s == nil || !s.Meetings || isGroupChat(c) || c.Handoff != nil || e.taskRunning(c.Key) {
		return
	}
	if _, ok := handoff.Detect(text, handoff.Categories{model.HandoffMeeting: true}); !ok && !task.MeetingRequest(text) {
		return
	}
	spec := task.FindTime()
	spec.Title, spec.Voice = "Meeting with "+cmp.Or(c.Name, "them"), model.VoiceAssistant
	spec.Meeting.Days, spec.Meeting.From, spec.Meeting.To = s.MeetDays, s.MeetFrom, s.MeetTo
	spec.Meeting.Minutes, spec.Meeting.Calendars = s.MeetingMinutes, s.UseCalendars
	from := in.Timestamp
	if from.IsZero() {
		from = e.clock.Now()
	}
	if _, err := e.startTask(e.ctx, c.Key, spec, false, from.Add(-time.Second)); err != nil {
		e.act(model.ActError, c, "", "Couldn't start finding a time: "+err.Error(), nil)
	}
}

// TaskTry runs a task in the sandbox: an AI plays the other side for a few
// turns (style: cooperative|vague|difficult|prober). Nothing is sent, saved
// or booked; the calendar is only read.
func (e *Engine) TaskTry(ctx context.Context, personaID string, spec model.TaskSpec, style string, turns int) (model.TaskTry, error) {
	p, ok := e.store.Persona(personaID)
	if !ok {
		return model.TaskTry{}, fmt.Errorf("persona %q: %w", personaID, ErrNotFound)
	}
	if err := task.Normalize(&spec); err != nil {
		return model.TaskTry{}, fmt.Errorf("%w: %s", ErrTaskInvalid, err.Error())
	}
	turns = min(max(turns, 1), 10)
	prov, mdl, err := e.llm.Resolve(p.LLM)
	if err != nil {
		return model.TaskTry{}, err
	}
	s := e.cfg.Get().Tasks
	run := model.TaskRun{ID: "try", ChatName: "Sandbox", Spec: task.Resolve(spec, e.taskDefaults()), Owner: s.Owner, Tone: s.Tone,
		Status: model.TaskActive, Step: model.StepOpen, Values: map[string]model.TaskValue{}, Fired: map[string]int{}}
	chat := model.ChatAssignment{Key: "try", Kind: "dm", Name: "Sandbox", PersonaID: p.ID, Enabled: true}
	var hist []model.Message
	out := model.TaskTry{Turns: []model.TaskTryTurn{}}
	now := e.clock.Now()
	write := func(t task.Turn) (string, error) {
		_, titles := e.taskAvail(ctx, run)
		u := &taskUse{run: run, turn: t, titles: titles, allowed: task.Allowed(&run, t), prompt: e.taskPrompt(run, t, "")}
		u.exempt = append([]string{run.Spec.About, run.Spec.Place, run.Spec.Title, run.Owner.About}, run.Spec.Privacy.CoverLines...)
		build := func(note string) llm.Request {
			req := prompt.Compose(p, chat, hist, false, prompt.Options{Task: u.prompt, Now: now, MacZone: time.Local})
			if len(hist) == 0 {
				req.Messages = []llm.Message{{Role: llm.RoleUser, Content: prompt.InitiateTurn}}
			}
			req.System += note
			req.Model = mdl
			return req
		}
		gen := func(note string) (genResult, error) {
			resp, err := prov.Chat(ctx, build(note))
			if err != nil {
				return genResult{}, err
			}
			return genResult{Text: strings.TrimSpace(resp.Text)}, nil
		}
		res, err := gen("")
		if err != nil {
			return "", err
		}
		res = e.taskGuard(chat, u, res, gen)
		return res.Text, nil
	}
	for i := 0; i < turns && run.Active(); i++ {
		var t task.Turn
		a, _ := e.taskAvail(ctx, run)
		if i == 0 {
			t = task.Decide(&run, task.Input{Event: task.EvStarted, Now: now}, a, e.rng)
		} else {
			last := hist[len(hist)-1]
			reading := e.taskRead(ctx, chat, p, run, hist, []model.Message{last})
			t = task.Decide(&run, task.Input{Event: task.EvMessage, Reading: reading, Their: []string{last.Text}, MsgID: last.ID, Now: now}, a, e.rng)
			out.Turns[len(out.Turns)-1].Note = "Read as \"" + reading.Intent + "\"" + readTimes(reading)
		}
		note := ""
		if t.Commit {
			if a.Works(*run.Proposed) {
				t.Say, t.Times, t.Commit = task.SayConfirmed, []time.Time{*run.Proposed}, false
				t.Effects = append(t.Effects, task.DoFinishOK)
				note = "Would confirm here (with your approval, unless auto-confirm is on); nothing is booked in the sandbox"
			} else {
				t.Say, t.Commit = task.SayAnswerNo, false
			}
		}
		text, err := write(t)
		if err != nil {
			return out, err
		}
		run.Sent++
		hist = append(hist, model.Message{ID: fmt.Sprint("m", len(hist)), TS: now, Speaker: "me", Text: text, FromBot: true})
		out.Turns = append(out.Turns, model.TaskTryTurn{Speaker: "me", Text: text, Action: t.Say, Note: note})
		switch {
		case t.Has(task.DoFinishOK):
			e.endRun(&run, model.TaskDone, "Done")
			end := run.Proposed.Add(time.Duration(run.Spec.Meeting.Minutes) * time.Minute)
			run.Result = &model.TaskResult{OK: true, When: run.Proposed, End: &end}
		case t.Has(task.DoHandoff):
			e.endRun(&run, model.TaskHandedOff, handoffWhy(t))
		case t.Has(task.DoFinishFail):
			e.endRun(&run, model.TaskFailed, "Ended without a time")
		}
		if !run.Active() {
			break
		}
		req := prompt.TaskPartner(style, spec.About, hist, now)
		req.Model = mdl
		resp, err := prov.Chat(ctx, req)
		if err != nil {
			return out, err
		}
		reply := strings.TrimSpace(resp.Text)
		now = now.Add(5 * time.Minute)
		hist = append(hist, model.Message{ID: fmt.Sprint("m", len(hist)), TS: now, Speaker: "them", Text: reply})
		out.Turns = append(out.Turns, model.TaskTryTurn{Speaker: "them", Text: reply})
	}
	out.Status, out.Result, out.Log = run.Status, run.Result, run.Log
	return out, nil
}

// lastTaskUse is the chat's latest task turn while its task still runs.
func (e *Engine) lastTaskUse(key string) *taskUse {
	v, ok := e.lastTask.Load(key)
	if !ok {
		return nil
	}
	u := v.(*taskUse)
	if id, ok := e.store.ActiveTaskID(key); !ok || id != u.run.ID {
		return nil
	}
	return u
}

// CoverLines drafts lines for "why not then?" from your description, with
// the default AI.
func (e *Engine) CoverLines(ctx context.Context, description string) ([]string, error) {
	if strings.TrimSpace(description) == "" {
		return nil, fmt.Errorf("%w: describe what the lines should say", ErrTaskInvalid)
	}
	prov, mdl, err := e.llm.Resolve(nil)
	if err != nil {
		return nil, err
	}
	req := prompt.CoverLines(description, 5)
	req.Model = mdl
	cctx, cancel := context.WithTimeout(ctx, taskReadTimeout)
	defer cancel()
	resp, err := prov.Chat(cctx, req)
	if err != nil {
		return nil, err
	}
	lines, err := prompt.ParseCoverLines(resp.Text)
	if err != nil {
		return nil, errors.New("the AI didn't write readable lines, try again")
	}
	return lines, nil
}

// noteRetry writes a reply again with a note added to its system prompt
// (the reply check and the task's leak check).
func (e *Engine) noteRetry(ctx context.Context, p model.Persona, bp model.BehaviorProfile, hist []model.Message, opening bool,
	cfg goals.Config, turn prompt.GoalTurn, cross *crossUse, build buildFn) func(note string) (genResult, error) {
	return func(note string) (genResult, error) {
		return e.replyFor(ctx, false, p, bp, hist, opening, cfg, turn, cross, func(t prompt.GoalTurn, x prompt.CrossTurn) llm.Request {
			r := build(t, x)
			r.System += note
			return r
		})
	}
}
