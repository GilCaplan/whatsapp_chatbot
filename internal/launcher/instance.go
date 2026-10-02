// Package launcher implements single-instance handling: instance.json
// (who is serving, on which port, with which token), the server.lock flock,
// health probing, and the detached spawn used when the .app is double-clicked.
package launcher

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"time"

	"whatsappdoppel/internal/config"
)

// Instance is the content of <data>/instance.json while a server is running.
type Instance struct {
	PID       int       `json:"pid"`
	Port      int       `json:"port"`
	Token     string    `json:"token"`
	StartedAt time.Time `json:"startedAt"`
	Version   string    `json:"version,omitempty"`
}

// URL is the browser address of the instance.
func (i Instance) URL() string { return fmt.Sprintf("http://127.0.0.1:%d/", i.Port) }

// ReadInstance loads instance.json (os.ErrNotExist when absent).
func ReadInstance(p config.Paths) (Instance, error) {
	var inst Instance
	b, err := os.ReadFile(p.InstanceFile())
	if err != nil {
		return inst, err
	}
	if err := json.Unmarshal(b, &inst); err != nil {
		return inst, fmt.Errorf("instance.json: %w", err)
	}
	return inst, nil
}

// WriteInstance atomically writes instance.json (mode 0600: it holds the token).
func WriteInstance(p config.Paths, inst Instance) error {
	return config.WriteJSONAtomic(p.InstanceFile(), inst, 0o600)
}

// RemoveInstance deletes instance.json; a missing file is not an error.
func RemoveInstance(p config.Paths) error {
	err := os.Remove(p.InstanceFile())
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}

// Health is the subset of GET /api/health the launcher cares about.
type Health struct {
	OK      bool   `json:"ok"`
	Version string `json:"version"`
	Port    int    `json:"port"`
	Token   string `json:"token"`
	DataDir string `json:"dataDir"`
}

var probeClient = &http.Client{Timeout: time.Second}

// Probe checks that inst answers /api/health within 1 s with the same token
// (so a different program that happens to own the port is not mistaken for us).
func Probe(ctx context.Context, inst Instance) (Health, bool) {
	var h Health
	if inst.Port <= 0 || inst.Token == "" {
		return h, false
	}
	ctx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fmt.Sprintf("http://127.0.0.1:%d/api/health", inst.Port), nil)
	if err != nil {
		return h, false
	}
	resp, err := probeClient.Do(req)
	if err != nil {
		return h, false
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return h, false
	}
	if err := json.NewDecoder(resp.Body).Decode(&h); err != nil {
		return h, false
	}
	return h, h.OK && h.Token == inst.Token
}

// Running returns the live instance for this data dir, if any.
func Running(ctx context.Context, p config.Paths) (Instance, bool) {
	inst, err := ReadInstance(p)
	if err != nil {
		return inst, false
	}
	_, ok := Probe(ctx, inst)
	return inst, ok
}

// Quit asks the running instance to shut down and waits (up to timeout) for it to go away.
func Quit(ctx context.Context, p config.Paths, timeout time.Duration) error {
	inst, ok := Running(ctx, p)
	if !ok {
		return ErrNotRunning
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, inst.URL()+"api/system/quit", nil)
	if err != nil {
		return err
	}
	req.Header.Set("X-Doppel-Token", inst.Token)
	resp, err := (&http.Client{Timeout: 5 * time.Second}).Do(req)
	if err != nil {
		return err
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("quit: server answered %s", resp.Status)
	}
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if _, ok := Probe(ctx, inst); !ok {
			return nil
		}
		time.Sleep(150 * time.Millisecond)
	}
	return errors.New("server did not stop in time")
}

// ErrNotRunning is returned when no live instance exists for the data dir.
var ErrNotRunning = errors.New("WhatsApp Doppel is not running")
