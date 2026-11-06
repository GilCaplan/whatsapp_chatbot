// Package translate runs the translator add-on: dedicated en→target models,
// not an LLM, so the same text always gives the same translation. The app
// stays one pure Go binary: the add-on is a small Python process (server.py)
// in the data directory, installed from Settings, with one language pack per
// target language. Packs are Argos Translate models, pinned in packs.go.
package translate

import (
	"archive/zip"
	"bufio"
	"context"
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"slices"
	"strings"
	"sync"
	"time"

	"whatsappdoppel/internal/core/model"
)

//go:embed server.py
var serverPy []byte

//go:embed pyproject.toml
var pyproject string

var (
	dependencyList = regexp.MustCompile(`(?s)\ndependencies = \[(.*?)\n\]`)
	dependencyName = regexp.MustCompile(`"([^"]+)"`)
)

// pipPackages are the add-on's libraries, pinned in pyproject.toml. pip checks
// the spaCy model against the hash in its link.
func pipPackages() []string {
	var out []string
	for _, m := range dependencyName.FindAllStringSubmatch(dependencyList.FindStringSubmatch(pyproject)[1], -1) {
		out = append(out, m[1])
	}
	return out
}

// addonStamp identifies the pins. It goes in the add-on's ready file, the way a
// pack's checksum goes in the pack's.
func addonStamp() string {
	sum := sha256.Sum256([]byte(strings.Join(pipPackages(), "\n")))
	return hex.EncodeToString(sum[:])
}

// packRetry is how long a failed pack waits before a reply tries it again, so
// a broken download isn't repeated for every message.
const packRetry = time.Hour

// packDownloads is how many packs download at once: "Download all" asks for
// every one, and each has its own time limit that only starts in its turn.
const packDownloads = 3

// ErrNotReady: the add-on or the language pack isn't there (yet).
var ErrNotReady = errors.New("translator not ready")

// Service owns the add-on's files and process.
type Service struct {
	dir   string
	packs map[string]pack
	slots chan struct{} // a place in line to download

	mu         sync.Mutex // status fields
	installing bool
	step       string
	lastErr    string
	fetching   map[string]bool
	failedAt   map[string]time.Time

	procMu sync.Mutex // the process: one request at a time
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	stdout *bufio.Reader
}

// New uses <dataDir>/translator.
func New(dataDir string) *Service {
	return &Service{dir: filepath.Join(dataDir, "translator"), packs: packs, slots: make(chan struct{}, packDownloads), fetching: map[string]bool{}, failedAt: map[string]time.Time{}}
}

func (s *Service) python() string {
	if runtime.GOOS == "windows" {
		return filepath.Join(s.dir, "venv", "Scripts", "python.exe")
	}
	return filepath.Join(s.dir, "venv", "bin", "python")
}

func (s *Service) packDir(code string) string { return filepath.Join(s.dir, "packs", "en_"+code) }

func exists(p string) bool { _, err := os.Stat(p); return err == nil }

// installed: the add-on on disk was set up from the pinned libraries. One from
// before the pins, or from older pins, isn't, so Settings offers Install again
// instead of running server.py on libraries it may not work with.
func (s *Service) installed() bool {
	got, err := os.ReadFile(filepath.Join(s.dir, "ready"))
	return err == nil && string(got) == addonStamp()
}

// packReady: the pack on disk is the pinned one. A pack from before the pins,
// or from an older pin, isn't, so it gets downloaded again.
func (s *Service) packReady(code string) bool {
	p, ok := s.packs[code]
	if !ok {
		return false
	}
	got, err := os.ReadFile(filepath.Join(s.packDir(code), "ready"))
	return err == nil && string(got) == p.sha256
}

// Status reports what is installed.
func (s *Service) Status() model.TranslatorStatus {
	s.mu.Lock()
	defer s.mu.Unlock()
	st := model.TranslatorStatus{Installed: s.installed(), Installing: s.installing, Step: s.step, Error: s.lastErr, Packs: []string{}}
	for code, p := range s.packs {
		st.Available = append(st.Available, model.TranslatorPack{Code: code, SizeMB: int(p.size / 1_000_000)})
		if s.packReady(code) {
			st.Packs = append(st.Packs, code)
		}
	}
	slices.SortFunc(st.Available, func(a, b model.TranslatorPack) int { return strings.Compare(a.Code, b.Code) })
	slices.Sort(st.Packs)
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

// Install sets the add-on up in the background if it isn't yet, then fetches
// langs. Asking again is how a pack that failed gets another try.
func (s *Service) Install(langs []string) {
	s.mu.Lock()
	if s.installing {
		s.mu.Unlock()
		return
	}
	s.installing, s.lastErr = true, ""
	clear(s.failedAt)
	s.mu.Unlock()
	go func() {
		var err error
		if !s.installed() {
			err = s.install()
		}
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

// findPython picks a Python the pinned libraries have builds for: 3.10 or newer.
// A plain python3 can be older, and it is on a stock Mac.
func findPython(ctx context.Context) (string, error) {
	for _, name := range []string{"python3.12", "python3.11", "python3.10", "python3.13", "python3.14", "python3", "python"} {
		p, err := exec.LookPath(name)
		if err == nil && exec.CommandContext(ctx, p, "-c", "import sys; sys.exit(sys.version_info < (3, 10))").Run() == nil {
			return p, nil
		}
	}
	return "", errors.New("the translator needs Python 3.10 or newer on this computer")
}

func (s *Service) install() error {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Minute)
	defer cancel()
	py, err := findPython(ctx)
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
	// --clear: a venv left from an older install would keep its old Python.
	if err := run("Preparing", py, "-m", "venv", "--clear", filepath.Join(s.dir, "venv")); err != nil {
		return err
	}
	if err := run("Installing the translator", s.python(), append([]string{"-m", "pip", "install", "--quiet"}, pipPackages()...)...); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(s.dir, "ready"), []byte(addonStamp()), 0o600)
}

// EnsurePack fetches a language pack in the background, once.
func (s *Service) EnsurePack(code string) {
	if _, ok := s.packs[code]; !ok {
		return
	}
	s.mu.Lock()
	// Under the lock: a fetch that just ended has already written its ready file.
	if s.packReady(code) || s.fetching[code] || time.Since(s.failedAt[code]) < packRetry {
		s.mu.Unlock()
		return
	}
	s.fetching[code] = true
	s.mu.Unlock()
	go func() {
		s.slots <- struct{}{}
		err := s.fetchPack(code)
		<-s.slots
		label := "Language pack " + code + ": "
		s.mu.Lock()
		delete(s.fetching, code)
		if err != nil {
			s.failedAt[code] = time.Now()
			s.lastErr = label + err.Error()
		} else if strings.HasPrefix(s.lastErr, label) {
			s.lastErr = ""
		}
		s.mu.Unlock()
	}()
}

func (s *Service) fetchPack(code string) error {
	p, ok := s.packs[code]
	if !ok {
		return errors.New("no pack for this language")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Minute)
	defer cancel()
	tmp, err := os.CreateTemp(s.dir, "pack-*.zip")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	defer tmp.Close()
	if err := download(ctx, p, tmp); err != nil {
		return err
	}
	dest := s.packDir(code)
	_ = os.RemoveAll(dest)
	if err := unpack(tmp.Name(), dest); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dest, "ready"), []byte(p.sha256), 0o600)
}

// download saves a pack and checks it against its pin. Nothing reads the file
// unless this returns nil, and it stops reading past the pinned size.
func download(ctx context.Context, p pack, to io.Writer) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, p.url, nil)
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
	sum := sha256.New()
	if _, err := io.Copy(io.MultiWriter(to, sum), io.LimitReader(resp.Body, p.size+1)); err != nil {
		return err
	}
	if hex.EncodeToString(sum.Sum(nil)) != p.sha256 {
		return errors.New("the download doesn't match its checksum, so it wasn't used")
	}
	return nil
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

// request is one line to the add-on (Request in server.py).
type request struct {
	Pack string   `json:"pack"`
	Text string   `json:"text"`
	Keep []string `json:"keep,omitempty"`
}

// Translate turns English text into code. A pack that isn't there yet is
// fetched in the background and ErrNotReady comes back for now.
func (s *Service) Translate(ctx context.Context, text, code string, keep []string) (string, error) {
	if !s.installed() {
		return "", ErrNotReady
	}
	if !s.packReady(code) {
		s.EnsurePack(code)
		return "", ErrNotReady
	}
	s.procMu.Lock()
	defer s.procMu.Unlock()
	if err := s.startLocked(); err != nil {
		return "", err
	}
	line, _ := json.Marshal(request{Pack: s.packDir(code), Text: text, Keep: keep})
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
