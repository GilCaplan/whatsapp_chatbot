package engine

import (
	"context"
	"sync"
	"time"
)

// Background jobs (wave 3, Engineer A; WAVE3_PLAN G3): memory extraction
// (and anything else that may call the model off the reply path) runs here.
// Jobs are named (scheduling a name again replaces its timer), wait on the
// engine clock, and run one at a time so a local model never gets two
// background calls at once. They yield to replies: when any chat is
// generating, a due job re-arms itself and tries again later. Jobs scheduled
// with scheduleYield also give way mid-call: a reply starting cancels them and
// they run again later, so they must save nothing until they have succeeded.

// jobRetryBusy is how long a job waits when a reply is being generated.
const jobRetryBusy = 2 * time.Minute

type jobRunner struct {
	mu      sync.Mutex
	timers  map[string]jobTimer
	seq     uint64
	stopped bool
	slot    chan struct{}                 // one background model call at a time
	yield   map[uint64]context.CancelFunc // running jobs a reply may interrupt
}

type jobTimer struct {
	t  Timer
	id uint64
}

func (j *jobRunner) ensure() {
	j.mu.Lock()
	defer j.mu.Unlock()
	if j.timers == nil {
		j.timers = map[string]jobTimer{}
		j.slot = make(chan struct{}, 1)
		j.yield = map[uint64]context.CancelFunc{}
	}
}

// schedule runs fn after d (on the engine clock), replacing any pending job
// with the same name. fn runs on its own goroutine, counted as busy, while
// holding the job slot; it is skipped (re-armed) while a reply is being
// written.
func (e *Engine) schedule(name string, d time.Duration, fn func(ctx context.Context)) {
	e.scheduleJob(name, d, false, fn)
}

// scheduleYield is schedule for work a reply may interrupt (see above).
func (e *Engine) scheduleYield(name string, d time.Duration, fn func(ctx context.Context)) {
	e.scheduleJob(name, d, true, fn)
}

func (e *Engine) scheduleJob(name string, d time.Duration, yields bool, fn func(ctx context.Context)) {
	j := &e.jobs
	j.ensure()
	j.mu.Lock()
	defer j.mu.Unlock()
	if j.stopped {
		return
	}
	if old, ok := j.timers[name]; ok {
		old.t.Stop()
	}
	j.seq++
	id := j.seq
	t := e.clock.AfterFunc(max(d, 0), func() {
		j.mu.Lock()
		if cur, ok := j.timers[name]; j.stopped || !ok || cur.id != id {
			j.mu.Unlock()
			return
		}
		delete(j.timers, name)
		j.mu.Unlock()
		e.goBusy(func() {
			if e.anyGenerating() {
				e.scheduleJob(name, jobRetryBusy, yields, fn)
				return
			}
			if !yields {
				e.withJobSlot(e.ctx, func() { fn(e.ctx) })
			} else if e.yieldingSlot(e.ctx, fn) {
				e.scheduleJob(name, jobRetryBusy, yields, fn)
			}
		})
	})
	j.timers[name] = jobTimer{t: t, id: id}
}

// yieldingSlot runs fn holding the background slot, with a context a starting
// reply cancels; true when that happened and the work should run again.
func (e *Engine) yieldingSlot(parent context.Context, fn func(ctx context.Context)) (interrupted bool) {
	j := &e.jobs
	e.withJobSlot(parent, func() {
		ctx, cancel := context.WithCancel(parent)
		defer cancel()
		j.mu.Lock()
		j.seq++
		id := j.seq
		j.yield[id] = cancel
		j.mu.Unlock()
		if !e.anyGenerating() { // a reply may have started while waiting for the slot
			fn(ctx)
		} else {
			cancel()
		}
		j.mu.Lock()
		delete(j.yield, id)
		j.mu.Unlock()
		interrupted = ctx.Err() != nil && parent.Err() == nil
	})
	return interrupted
}

// preemptJobs cancels the running jobs that yield to replies.
func (e *Engine) preemptJobs() {
	j := &e.jobs
	j.mu.Lock()
	defer j.mu.Unlock()
	for _, cancel := range j.yield {
		cancel()
	}
}

// withJobSlot runs fn holding the background slot; false when ctx ended first.
func (e *Engine) withJobSlot(ctx context.Context, fn func()) bool {
	j := &e.jobs
	j.ensure()
	select {
	case j.slot <- struct{}{}:
	case <-ctx.Done():
		return false
	}
	defer func() { <-j.slot }()
	fn()
	return true
}

// cancelJob drops a pending job.
func (e *Engine) cancelJob(name string) {
	j := &e.jobs
	j.mu.Lock()
	defer j.mu.Unlock()
	if old, ok := j.timers[name]; ok {
		old.t.Stop()
		delete(j.timers, name)
	}
}

// jobPending reports whether a job is scheduled.
func (e *Engine) jobPending(name string) bool {
	j := &e.jobs
	j.mu.Lock()
	defer j.mu.Unlock()
	_, ok := j.timers[name]
	return ok
}

// stopJobs cancels every pending job (Engine.Stop).
func (e *Engine) stopJobs() {
	j := &e.jobs
	j.mu.Lock()
	defer j.mu.Unlock()
	j.stopped = true
	for name, jt := range j.timers {
		jt.t.Stop()
		delete(j.timers, name)
	}
}

// anyGenerating (recap.go) reports whether some chat is writing a reply.
