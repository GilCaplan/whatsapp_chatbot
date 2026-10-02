package wa

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"

	"go.mau.fi/whatsmeow/proto/waHistorySync"
	"go.mau.fi/whatsmeow/types"
)

// recentEntry is one chat in the "recent chats" index, keyed by canonical chat key.
type recentEntry struct {
	Key      string `json:"key"`
	Kind     string `json:"kind"`
	JID      string `json:"jid"`
	AltJID   string `json:"altJid,omitempty"`
	Name     string `json:"name,omitempty"`
	TS       int64  `json:"ts"` // unix seconds of the last message, 0 if unknown
	Archived bool   `json:"archived,omitempty"`
}

// recentIndex is built from history sync + live messages and persisted
// (debounced) to paths.RecentChatsFile().
type recentIndex struct {
	path string

	mu      sync.Mutex
	entries map[string]recentEntry
	timer   *time.Timer
	closed  bool
}

const recentSaveDelay = 2 * time.Second

func newRecentIndex(path string) *recentIndex {
	r := &recentIndex{path: path, entries: map[string]recentEntry{}}
	if b, err := os.ReadFile(path); err == nil {
		var list []recentEntry
		if json.Unmarshal(b, &list) == nil {
			for _, e := range list {
				if e.Key != "" {
					r.entries[e.Key] = e
				}
			}
		}
	}
	return r
}

// upsert merges e into the index. The newer timestamp wins; empty fields
// never overwrite known ones. fromSync marks e.Archived as authoritative.
func (r *recentIndex) upsert(e recentEntry, fromSync bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	old, ok := r.entries[e.Key]
	if ok {
		if e.Name == "" {
			e.Name = old.Name
		}
		if e.AltJID == "" {
			e.AltJID = old.AltJID
		}
		if e.JID == "" {
			e.JID = old.JID
		}
		if old.TS > e.TS {
			e.TS = old.TS
		}
		if !fromSync {
			e.Archived = old.Archived && old.TS >= e.TS
		}
	}
	r.entries[e.Key] = e
	r.scheduleSaveLocked()
}

func (r *recentIndex) get(key string) (recentEntry, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	e, ok := r.entries[key]
	return e, ok
}

func (r *recentIndex) list() []recentEntry {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]recentEntry, 0, len(r.entries))
	for _, e := range r.entries {
		out = append(out, e)
	}
	return out
}

func (r *recentIndex) clear() {
	r.mu.Lock()
	r.entries = map[string]recentEntry{}
	r.scheduleSaveLocked()
	r.mu.Unlock()
}

func (r *recentIndex) scheduleSaveLocked() {
	if r.closed || r.timer != nil {
		return
	}
	r.timer = time.AfterFunc(recentSaveDelay, r.save)
}

func (r *recentIndex) save() {
	r.mu.Lock()
	r.timer = nil
	list := make([]recentEntry, 0, len(r.entries))
	for _, e := range r.entries {
		list = append(list, e)
	}
	r.mu.Unlock()
	b, err := json.Marshal(list)
	if err != nil {
		return
	}
	_ = writeFileAtomic(r.path, b)
}

// close flushes pending changes and stops further saves.
func (r *recentIndex) close() {
	r.mu.Lock()
	pending := r.timer != nil && r.timer.Stop()
	r.timer = nil
	r.closed = true
	r.mu.Unlock()
	if pending {
		r.save()
	}
}

// entryFromConversation maps a history-sync conversation to an index entry.
// The conversation is matched exactly by its JID (never by substring).
func entryFromConversation(conv *waHistorySync.Conversation, r resolver) (recentEntry, bool) {
	jid, err := types.ParseJID(conv.GetID())
	if err != nil || jid.User == "" || skippedChat(jid) {
		return recentEntry{}, false
	}
	jid = jid.ToNonAD()
	ts := conv.GetConversationTimestamp()
	if ts == 0 {
		ts = conv.GetLastMsgTimestamp()
	}
	name := conv.GetName()
	if name == "" {
		name = conv.GetDisplayName()
	}
	e := recentEntry{Name: name, TS: int64(ts), Archived: conv.GetArchived()}
	switch {
	case jid.Server == types.GroupServer:
		e.Key, e.Kind, e.JID = prefixGroup+jid.User, "group", jid.String()
	case isUserServer(jid):
		var hint types.JID
		if jid.Server == types.HiddenUserServer {
			hint, _ = types.ParseJID(conv.GetPnJID())
		} else {
			hint, _ = types.ParseJID(conv.GetLidJID())
		}
		key, pn, lid := r.dm(jid, hint)
		e.Key, e.Kind = key, "dm"
		if pn.User != "" {
			e.JID, e.AltJID = pn.String(), jidString(lid)
		} else {
			e.JID = lid.String()
		}
	default:
		return recentEntry{}, false
	}
	return e, true
}

func writeFileAtomic(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), filepath.Base(path)+".tmp-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}
