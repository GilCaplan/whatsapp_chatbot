package wa

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

const (
	avatarTTL         = 24 * time.Hour
	avatarNegativeTTL = 6 * time.Hour
	avatarMaxBytes    = 5 << 20
	avatarConcurrency = 2
)

// errNoAvatar marks a chat without a (visible) profile picture.
var errNoAvatar = errors.New("no profile picture")

// avatarCache is a disk cache: <sha1(jid)>.img holds the picture, <sha1>.none
// marks a negative result; file mtimes provide the TTLs.
type avatarCache struct {
	dir  string
	http *http.Client
	sem  chan struct{}
}

func newAvatarCache(dir string) *avatarCache {
	return &avatarCache{
		dir:  dir,
		http: &http.Client{Timeout: 20 * time.Second},
		sem:  make(chan struct{}, avatarConcurrency),
	}
}

func (a *avatarCache) files(jid string) (img, none string) {
	h := sha1.Sum([]byte(jid))
	base := filepath.Join(a.dir, hex.EncodeToString(h[:]))
	return base + ".img", base + ".none"
}

// lookup returns cached bytes (fresh reports TTL validity) or negative=true when a
// fresh "no picture" marker exists.
func (a *avatarCache) lookup(jid string) (data []byte, fresh, negative bool) {
	img, none := a.files(jid)
	if st, err := os.Stat(none); err == nil && time.Since(st.ModTime()) < avatarNegativeTTL {
		return nil, true, true
	}
	st, err := os.Stat(img)
	if err != nil {
		return nil, false, false
	}
	data, err = os.ReadFile(img)
	if err != nil || len(data) == 0 {
		return nil, false, false
	}
	return data, time.Since(st.ModTime()) < avatarTTL, false
}

func (a *avatarCache) storeImage(jid string, data []byte) {
	img, none := a.files(jid)
	_ = writeFileAtomic(img, data)
	_ = os.Remove(none)
}

func (a *avatarCache) storeNegative(jid string) {
	img, none := a.files(jid)
	_ = os.Remove(img)
	_ = os.WriteFile(none, nil, 0o600)
}

func (a *avatarCache) invalidate(jids ...string) {
	for _, j := range jids {
		if j == "" {
			continue
		}
		img, none := a.files(j)
		_ = os.Remove(img)
		_ = os.Remove(none)
	}
}

func (a *avatarCache) acquire(ctx context.Context) error {
	select {
	case a.sem <- struct{}{}:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (a *avatarCache) release() { <-a.sem }

// download fetches a picture URL; errNoAvatar on 404.
func (a *avatarCache) download(ctx context.Context, url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := a.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	switch {
	case resp.StatusCode == http.StatusNotFound:
		return nil, errNoAvatar
	case resp.StatusCode != http.StatusOK:
		return nil, fmt.Errorf("avatar download: HTTP %d", resp.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, avatarMaxBytes+1))
	if err != nil {
		return nil, err
	}
	if len(data) > avatarMaxBytes {
		return nil, errors.New("avatar download: image too large")
	}
	return data, nil
}

func imageContentType(data []byte) string {
	return http.DetectContentType(data)
}
