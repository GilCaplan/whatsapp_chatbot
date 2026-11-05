// Package translate runs the translator add-on: dedicated en→target models,
// not an LLM, so the same text always gives the same translation. The app
// stays one pure Go binary: the add-on is a small Python process (server.py)
// in the data directory, installed from Settings, with one language pack per
// target language. Packs come from the Argos Translate index.
package translate

import (
	"archive/zip"
	"bufio"
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"sync"
	"time"

	"whatsappdoppel/internal/core/model"
)

//go:embed server.py
var serverPy []byte

const (
	indexURL = "https://raw.githubusercontent.com/argosopentech/argospm-index/main/index.json"
	spacyWhl = "https://github.com/explosion/spacy-models/releases/download/en_core_web_sm-3.8.0/en_core_web_sm-3.8.0-py3-none-any.whl"
)

// ErrNotReady: the add-on or the language pack isn't there (yet).
var ErrNotReady = errors.New("translator not ready")

// Service owns the add-on's files and process.
type Service struct {
	dir string

	mu         sync.Mutex // status fields
	installing bool
	step       string
	lastErr    string
	fetching   map[string]bool

	procMu sync.Mutex // the process: one request at a time
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	stdout *bufio.Reader
}

// New uses <dataDir>/translator.
func New(dataDir string) *Service {
	return &Service{dir: filepath.Join(dataDir, "translator"), fetching: map[string]bool{}}
}

func (s *Service) python() string {
	if runtime.GOOS == "windows" {
		return filepath.Join(s.dir, "venv", "Scripts", "python.exe")
	}
	return filepath.Join(s.dir, "venv", "bin", "python")
}

func (s *Service) packDir(code string) string { return filepath.Join(s.dir, "packs", "en_"+code) }

func exists(p string) bool { _, err := os.Stat(p); return err == nil }

// Status reports what is installed.
func (s *Service) Status() model.TranslatorStatus {
	s.mu.Lock()
	defer s.mu.Unlock()
	st := model.TranslatorStatus{Installed: exists(filepath.Join(s.dir, "ready")), Installing: s.installing, Step: s.step, Error: s.lastErr, Packs: []string{}}
	dirs, _ := os.ReadDir(filepath.Join(s.dir, "packs"))
	for _, d := range dirs {
		if code, ok := strings.CutPrefix(d.Name(), "en_"); ok && exists(filepath.Join(s.packDir(code), "ready")) {
			st.Packs = append(st.Packs, code)
		}
	}
	for code := range s.fetching {
		st.Fetching = append(st.Fetching, code)
	}
	slices.Sort(st.Fetching)
	return st
}

func (s *Service) setStep(step string) {
	s.mu.Lock()
	s.step = step
	s.mu.Unlock()
}

// Install sets the add-on up in the background, then fetches langs.
func (s *Service) Install(langs []string) {
	s.mu.Lock()
	if s.installing {
		s.mu.Unlock()
		return
	}
	s.installing, s.lastErr = true, ""
	s.mu.Unlock()
	go func() {
		err := s.install()
		s.mu.Lock()
		s.installing, s.step = false, ""
		if err != nil {
			s.lastErr = err.Error()
		}
		s.mu.Unlock()
		if err == nil {
			for _, c := range langs {
				s.EnsurePack(c)
			}
		}
	}()
}

// findPython picks a Python the translator's libraries have builds for.
func findPython() (string, error) {
	for _, name := range []string{"python3.12", "python3.11", "python3.10", "python3", "python"} {
		if p, err := exec.LookPath(name); err == nil {
			return p, nil
		}
	}
	return "", errors.New("Python 3 isn't installed on this computer")
}

func (s *Service) install() error {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Minute)
	defer cancel()
	py, err := findPython()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(s.dir, 0o700); err != nil {
		return err
	}
	run := func(step, bin string, args ...string) error {
		s.setStep(step)
		out, err := exec.CommandContext(ctx, bin, args...).CombinedOutput()
		if err != nil {
			text := strings.TrimSpace(string(out))
			return fmt.Errorf("%s failed: %s", step, text[max(0, len(text)-300):])
		}
		return nil
	}
	if err := run("Preparing", py, "-m", "venv", filepath.Join(s.dir, "venv")); err != nil {
		return err
	}
	if err := run("Installing the translator", s.python(), "-m", "pip", "install", "--quiet", "ctranslate2", "sentencepiece", "spacy", spacyWhl); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(s.dir, "ready"), nil, 0o600)
}

// EnsurePack fetches a language pack in the background, once.
func (s *Service) EnsurePack(code string) {
	if code == "" || code == "en" || exists(filepath.Join(s.packDir(code), "ready")) {
		return
	}
	s.mu.Lock()
	if s.fetching[code] {
		s.mu.Unlock()
		return
	}
	s.fetching[code] = true
	s.mu.Unlock()
	go func() {
		err := s.fetchPack(code)
		s.mu.Lock()
		delete(s.fetching, code)
		if err != nil {
			s.lastErr = fmt.Sprintf("Language pack %s: %v", code, err)
		}
		s.mu.Unlock()
	}()
}

func (s *Service) fetchPack(code string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Minute)
	defer cancel()
	link, err := packLink(ctx, code)
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(s.dir, "pack-*.zip")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	defer tmp.Close()
	if err := download(ctx, link, tmp); err != nil {
		return err
	}
	dest := s.packDir(code)
	_ = os.RemoveAll(dest)
	if err := unpack(tmp.Name(), dest); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dest, "ready"), nil, 0o600)
}

func packLink(ctx context.Context, code string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, indexURL, nil)
	if err != nil {
		return "", err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	var index []struct {
		From  string   `json:"from_code"`
		To    string   `json:"to_code"`
		Links []string `json:"links"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&index); err != nil {
		return "", err
	}
	for _, p := range index {
		if p.From == "en" && p.To == code && len(p.Links) > 0 {
			return p.Links[0], nil
		}
	}
	return "", errors.New("no pack for this language")
}

func download(ctx context.Context, url string, to io.Writer) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download: %s", resp.Status)
	}
	_, err = io.Copy(to, resp.Body)
	return err
}

// unpack extracts a pack without its top folder, keeping only what it needs.
func unpack(zipPath, dest string) error {
	r, err := zip.OpenReader(zipPath)
	if err != nil {
		return err
	}
	defer r.Close()
	for _, f := range r.File {
		_, rel, ok := strings.Cut(filepath.ToSlash(f.Name), "/")
		if !ok || rel == "" || f.FileInfo().IsDir() || !(strings.HasPrefix(rel, "model/") || rel == "sentencepiece.model") {
			continue
		}
		to := filepath.Join(dest, filepath.FromSlash(rel))
		if !strings.HasPrefix(to, filepath.Clean(dest)+string(filepath.Separator)) {
			return fmt.Errorf("bad path in pack: %s", f.Name)
		}
		if err := os.MkdirAll(filepath.Dir(to), 0o700); err != nil {
			return err
		}
		if err := extract(f, to); err != nil {
			return err
		}
	}
	return nil
}

func extract(f *zip.File, to string) error {
	in, err := f.Open()
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(to)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, in)
	return err
}

// startLocked runs the process if it isn't running.
func (s *Service) startLocked() error {
	if s.cmd != nil {
		return nil
	}
	script := filepath.Join(s.dir, "server.py")
	if err := os.WriteFile(script, serverPy, 0o600); err != nil {
		return err
	}
	cmd := exec.Command(s.python(), script)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	if err := cmd.Start(); err != nil {
		return err
	}
	s.cmd, s.stdin, s.stdout = cmd, stdin, bufio.NewReaderSize(stdout, 1<<20)
	return nil
}

func (s *Service) stopLocked() {
	if s.cmd == nil {
		return
	}
	_ = s.stdin.Close()
	_ = s.cmd.Process.Kill()
	_ = s.cmd.Wait()
	s.cmd = nil
}

// Close stops the process.
func (s *Service) Close() {
	s.procMu.Lock()
	defer s.procMu.Unlock()
	s.stopLocked()
}

// Translate turns English text into code. A pack that isn't there yet is
// fetched in the background and ErrNotReady comes back for now.
func (s *Service) Translate(ctx context.Context, text, code string) (string, error) {
	if !exists(filepath.Join(s.dir, "ready")) {
		return "", ErrNotReady
	}
	if !exists(filepath.Join(s.packDir(code), "ready")) {
		s.EnsurePack(code)
		return "", ErrNotReady
	}
	s.procMu.Lock()
	defer s.procMu.Unlock()
	if err := s.startLocked(); err != nil {
		return "", err
	}
	line, _ := json.Marshal(map[string]string{"pack": s.packDir(code), "text": text})
	type answer struct {
		Text  string `json:"text"`
		Error string `json:"error"`
	}
	done := make(chan error, 1)
	var a answer
	go func() {
		if _, err := s.stdin.Write(append(line, '\n')); err != nil {
			done <- err
			return
		}
		raw, err := s.stdout.ReadBytes('\n')
		if err == nil {
			err = json.Unmarshal(raw, &a)
		}
		done <- err
	}()
	select {
	case err := <-done:
		if err != nil {
			s.stopLocked()
			return "", fmt.Errorf("translator: %w", err)
		}
	case <-ctx.Done():
		s.stopLocked() // its answer would land on the next request
		<-done
		return "", ctx.Err()
	}
	if a.Error != "" {
		return "", fmt.Errorf("translator: %s", a.Error)
	}
	return a.Text, nil
}
