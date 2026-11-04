package store

import (
	"errors"
	"slices"
	"sync"

	"whatsappdoppel/internal/core/config"
	"whatsappdoppel/internal/core/model"
)

// Tasks: the tasks you saved (tasks.json) and every run (taskruns.json,
// oldest first, the newest MaxTaskRuns kept; active runs are never dropped).
// Read on demand under their own mutex, like missions.

// MaxTaskRuns is how many runs are kept.
const MaxTaskRuns = 300

var taskMu sync.Mutex

// ErrTaskConflict means a run changed since it was read.
var ErrTaskConflict = errors.New("the task changed meanwhile")

// activeIdx maps a data dir's chat keys to their running task's id; built on
// first use, rebuilt on every write of taskruns.json. Guarded by taskMu.
var activeIdx = map[string]map[string]string{}

func indexRuns(all []model.TaskRun) map[string]string {
	idx := map[string]string{}
	for _, r := range all {
		if r.Active() {
			idx[r.ChatKey] = r.ID
		}
	}
	return idx
}

// ActiveTaskID is the chat's running task id, from memory (cheap: the hot
// paths ask for every message and timer).
func (s *Store) ActiveTaskID(chatKey string) (string, bool) {
	taskMu.Lock()
	defer taskMu.Unlock()
	idx, ok := activeIdx[s.paths.Dir]
	if !ok {
		all, _ := readList[model.TaskRun](s.paths.TaskRunsFile())
		idx = indexRuns(all)
		activeIdx[s.paths.Dir] = idx
	}
	id, ok := idx[chatKey]
	return id, ok
}

// TaskSpecs returns the tasks you saved, oldest first.
func (s *Store) TaskSpecs() []model.TaskSpec {
	taskMu.Lock()
	defer taskMu.Unlock()
	out, _ := readList[model.TaskSpec](s.paths.TasksFile())
	return nonNil(out)
}

// UpdateTaskSpecs runs fn on the saved tasks and saves the result.
func (s *Store) UpdateTaskSpecs(fn func([]model.TaskSpec) ([]model.TaskSpec, error)) ([]model.TaskSpec, error) {
	taskMu.Lock()
	defer taskMu.Unlock()
	cur, err := readList[model.TaskSpec](s.paths.TasksFile())
	if err != nil {
		return nil, err
	}
	next, err := fn(cur)
	if err != nil {
		return nil, err
	}
	next = nonNil(next)
	return next, config.WriteJSONAtomic(s.paths.TasksFile(), next, 0o600)
}

// TaskRuns returns every run, newest first.
func (s *Store) TaskRuns() []model.TaskRun {
	taskMu.Lock()
	defer taskMu.Unlock()
	all, _ := readList[model.TaskRun](s.paths.TaskRunsFile())
	slices.Reverse(all)
	return nonNil(all)
}

// TaskRun returns one run.
func (s *Store) TaskRun(id string) (model.TaskRun, bool) {
	for _, r := range s.TaskRuns() {
		if r.ID == id {
			return r, true
		}
	}
	return model.TaskRun{}, false
}

// ActiveTask returns the chat's running task, if any.
func (s *Store) ActiveTask(chatKey string) (model.TaskRun, bool) {
	id, ok := s.ActiveTaskID(chatKey)
	if !ok {
		return model.TaskRun{}, false
	}
	r, ok := s.TaskRun(id)
	return r, ok && r.Active()
}

// UpdateTaskRuns runs fn on every run (oldest first) and saves them.
func (s *Store) UpdateTaskRuns(fn func([]model.TaskRun) ([]model.TaskRun, error)) error {
	taskMu.Lock()
	defer taskMu.Unlock()
	all, err := readList[model.TaskRun](s.paths.TaskRunsFile())
	if err != nil {
		return err
	}
	if all, err = fn(all); err != nil {
		return err
	}
	for len(all) > MaxTaskRuns {
		i := slices.IndexFunc(all, func(r model.TaskRun) bool { return !r.Active() })
		if i < 0 {
			break
		}
		all = slices.Delete(all, i, i+1)
	}
	if err := config.WriteJSONAtomic(s.paths.TaskRunsFile(), nonNil(all), 0o600); err != nil {
		return err
	}
	activeIdx[s.paths.Dir] = indexRuns(all)
	return nil
}

// SwapTaskRun saves run only if the stored one is still at rev (it is then
// at rev+1): ErrTaskConflict otherwise, ErrNotFound when it is gone.
func (s *Store) SwapTaskRun(run model.TaskRun, rev int) (model.TaskRun, error) {
	var out model.TaskRun
	err := s.UpdateTaskRuns(func(all []model.TaskRun) ([]model.TaskRun, error) {
		for i := range all {
			if all[i].ID != run.ID {
				continue
			}
			if all[i].Rev != rev {
				return nil, ErrTaskConflict
			}
			run.Rev = rev + 1
			all[i], out = run, run
			return all, nil
		}
		return nil, ErrNotFound
	})
	return out, err
}

// errNoChange lets an edit leave the file alone.
var errNoChange = errors.New("no change")

// EditTaskRun runs fn on the stored run under the lock; it is saved (at the
// next rev) only when fn returns true. ok=false when it is gone or fn
// declined.
func (s *Store) EditTaskRun(id string, fn func(*model.TaskRun) bool) (model.TaskRun, bool, error) {
	var out model.TaskRun
	err := s.UpdateTaskRuns(func(all []model.TaskRun) ([]model.TaskRun, error) {
		for i := range all {
			if all[i].ID != id {
				continue
			}
			out = all[i]
			if !fn(&all[i]) {
				return nil, errNoChange
			}
			all[i].Rev++
			out = all[i]
			return all, nil
		}
		return nil, ErrNotFound
	})
	switch {
	case errors.Is(err, errNoChange):
		return out, false, nil
	case errors.Is(err, ErrNotFound):
		return out, false, nil
	}
	return out, err == nil, err
}

// AddTaskRun appends a run unless its chat already has a running one.
func (s *Store) AddTaskRun(run model.TaskRun) error {
	return s.UpdateTaskRuns(func(all []model.TaskRun) ([]model.TaskRun, error) {
		for _, r := range all {
			if r.ChatKey == run.ChatKey && r.Active() {
				return nil, ErrTaskRunning
			}
		}
		return append(all, run), nil
	})
}

// ErrTaskRunning means the chat already has a running task.
var ErrTaskRunning = errors.New("this chat already has a task running")

// UpdateTaskRun runs fn on one run and saves it; ok=false when it is gone.
func (s *Store) UpdateTaskRun(id string, fn func(*model.TaskRun)) (model.TaskRun, bool, error) {
	var out model.TaskRun
	found := false
	err := s.UpdateTaskRuns(func(all []model.TaskRun) ([]model.TaskRun, error) {
		for i := range all {
			if all[i].ID == id {
				fn(&all[i])
				all[i].Rev++
				out, found = all[i], true
			}
		}
		return all, nil
	})
	return out, found, err
}
