package calendar

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"sync"
	"time"

	"whatsappdoppel/internal/core/model"
)

// Service fetches and caches the owner's calendars (read-only).
type Service struct {
	client *http.Client
	mu     sync.Mutex
	cache  map[string]cached
	now    func() time.Time
}

type cached struct {
	events []Event
	at     time.Time
	err    error
}

const (
	refreshEvery = 10 * time.Minute
	fetchTimeout = 15 * time.Second
	windowBack   = 24 * time.Hour
	windowAhead  = 21 * 24 * time.Hour
	maxICSBytes  = 20 << 20
)

// New makes a Service; now = nil uses time.Now.
func New(now func() time.Time) *Service {
	if now == nil {
		now = time.Now
	}
	return &Service{client: &http.Client{Timeout: fetchTimeout}, cache: map[string]cached{}, now: now}
}

// Status is how one source did on its last fetch.
type Status struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	OK        bool      `json:"ok"`
	Error     string    `json:"error,omitempty"`
	Events    int       `json:"events"` // in the next 3 weeks
	FetchedAt time.Time `json:"fetchedAt"`
}

func cacheKey(s model.CalendarSource) string {
	return s.ID + "\x00" + s.Kind + "\x00" + s.URL + "\x00" + s.APIKey
}

// Events returns a source's events from yesterday to three weeks ahead
// (cached for refreshEvery; force refetches).
func (s *Service) Events(ctx context.Context, src model.CalendarSource, force bool) ([]Event, Status) {
	key := cacheKey(src)
	now := s.now()
	s.mu.Lock()
	c, ok := s.cache[key]
	s.mu.Unlock()
	if !ok || force || now.Sub(c.at) > refreshEvery {
		from, to := now.Add(-windowBack), now.Add(windowAhead)
		evs, err := s.fetch(ctx, src, from, to)
		c = cached{events: evs, at: now, err: err}
		if err != nil && ok && c.events == nil {
			c.events = s.cache[key].events // keep the last good copy
		}
		s.mu.Lock()
		s.cache[key] = c
		s.mu.Unlock()
	}
	st := Status{ID: src.ID, Name: src.Name, OK: c.err == nil, Events: len(c.events), FetchedAt: c.at}
	if c.err != nil {
		st.Error = c.err.Error()
	}
	return c.events, st
}

func (s *Service) fetch(ctx context.Context, src model.CalendarSource, from, to time.Time) ([]Event, error) {
	ctx, cancel := context.WithTimeout(ctx, fetchTimeout)
	defer cancel()
	switch src.Kind {
	case model.CalendarICS:
		u := strings.TrimSpace(src.URL)
		if strings.HasPrefix(strings.ToLower(u), "webcal://") {
			u = "https://" + u[len("webcal://"):]
		}
		body, err := s.get(ctx, u, nil)
		if err != nil {
			return nil, err
		}
		defer body.Close()
		evs, err := ParseICS(io.LimitReader(body, maxICSBytes), from, to, time.Local)
		if err != nil {
			return nil, fmt.Errorf("not a readable calendar: %w", err)
		}
		return evs, nil
	case model.CalendarMACalendar:
		return s.fetchMACalendar(ctx, src, from, to)
	}
	return nil, fmt.Errorf("unknown calendar kind %q", src.Kind)
}

func (s *Service) get(ctx context.Context, u string, header map[string]string) (io.ReadCloser, error) {
	pu, err := url.Parse(u)
	if err != nil || (pu.Scheme != "http" && pu.Scheme != "https") || pu.Host == "" {
		return nil, errors.New("the link must start with http://, https:// or webcal://")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	for k, v := range header {
		req.Header.Set(k, v)
	}
	resp, err := s.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("couldn't reach it (%s)", pu.Host)
	}
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
			return nil, fmt.Errorf("%s refused access (%d): check the link or key", pu.Host, resp.StatusCode)
		}
		return nil, fmt.Errorf("%s answered %d", pu.Host, resp.StatusCode)
	}
	return resp.Body, nil
}

// fetchMACalendar reads GET {base}/events?week_start=YYYY-MM-DD week by week.
// Its times are local ("HH:MM", no zone).
func (s *Service) fetchMACalendar(ctx context.Context, src model.CalendarSource, from, to time.Time) ([]Event, error) {
	base := strings.TrimRight(strings.TrimSpace(src.URL), "/")
	if base == "" {
		base = "http://127.0.0.1:8080"
	}
	header := map[string]string{}
	if k := strings.TrimSpace(src.APIKey); k != "" {
		header["X-API-Key"] = k
	}
	from, to = from.In(time.Local), to.In(time.Local)
	monday := time.Date(from.Year(), from.Month(), from.Day()-(int(from.Weekday())+6)%7, 0, 0, 0, 0, time.Local)
	var out []Event
	for w := monday; w.Before(to); w = w.AddDate(0, 0, 7) {
		body, err := s.get(ctx, base+"/events?week_start="+w.Format("2006-01-02"), header)
		if err != nil {
			return nil, err
		}
		var rows []struct {
			Title     string `json:"title"`
			Date      string `json:"date"`
			StartTime string `json:"start_time"`
			EndTime   string `json:"end_time"`
		}
		err = json.NewDecoder(io.LimitReader(body, maxICSBytes)).Decode(&rows)
		body.Close()
		if err != nil {
			return nil, errors.New("MACalendar answered with something that isn't its event list")
		}
		for _, r := range rows {
			day, err := time.ParseInLocation("2006-01-02", r.Date, time.Local)
			if err != nil {
				continue
			}
			e := Event{Title: r.Title}
			st, okS := clockOn(day, r.StartTime)
			en, okE := clockOn(day, r.EndTime)
			switch {
			case !okS:
				e.Start, e.End, e.AllDay = day, day.AddDate(0, 0, 1), true
			case !okE || !en.After(st):
				e.Start, e.End = st, st // no end: a reminder, takes no time
			default:
				e.Start, e.End = st, en
			}
			if e.End.After(from) && e.Start.Before(to) {
				out = append(out, e)
			}
		}
	}
	slices.SortFunc(out, func(a, b Event) int { return a.Start.Compare(b.Start) })
	return out, nil
}

// clockOn reads "HH:MM" on day.
func clockOn(day time.Time, hhmm string) (time.Time, bool) {
	t, err := time.Parse("15:04", strings.TrimSpace(hhmm))
	if err != nil {
		return time.Time{}, false
	}
	return time.Date(day.Year(), day.Month(), day.Day(), t.Hour(), t.Minute(), 0, 0, day.Location()), true
}

// Interval is a span of time.
type Interval struct {
	Start, End time.Time
}

// Busy merges the busy times of the enabled sources within [from, to).
// All-day events count as busy only when ignoreAllDay is false. Entries that
// take no time (reminders) and entries whose title contains one of the
// source's SkipWords never count.
func (s *Service) Busy(ctx context.Context, srcs []model.CalendarSource, from, to time.Time, ignoreAllDay bool) ([]Interval, []Status) {
	var spans []Interval
	var sts []Status
	for _, src := range srcs {
		if !src.Enabled {
			continue
		}
		evs, st := s.Events(ctx, src, false)
		sts = append(sts, st)
		for _, e := range evs {
			if (e.AllDay && ignoreAllDay) || !e.End.After(e.Start) || Skipped(e.Title, src.SkipWords) {
				continue
			}
			if e.End.After(from) && e.Start.Before(to) {
				spans = append(spans, Interval{maxT(e.Start, from), minT(e.End, to)})
			}
		}
	}
	return merge(spans), sts
}

// Skipped reports whether title contains one of words (any case).
func Skipped(title string, words []string) bool {
	low := strings.ToLower(title)
	for _, w := range words {
		if w = strings.ToLower(strings.TrimSpace(w)); w != "" && strings.Contains(low, w) {
			return true
		}
	}
	return false
}

func merge(spans []Interval) []Interval {
	slices.SortFunc(spans, func(a, b Interval) int { return a.Start.Compare(b.Start) })
	var out []Interval
	for _, sp := range spans {
		if n := len(out); n > 0 && !sp.Start.After(out[n-1].End) {
			out[n-1].End = maxT(out[n-1].End, sp.End)
			continue
		}
		out = append(out, sp)
	}
	return out
}

// FreeSlots are the meeting windows (days, from–to each day, in loc) from
// `from` to `to` minus busy, at least minLen long.
func FreeSlots(busy []Interval, from, to time.Time, days []time.Weekday, dayFrom, dayTo string, minLen time.Duration, loc *time.Location) []Interval {
	if loc == nil {
		loc = time.Local
	}
	f, errF := time.Parse("15:04", dayFrom)
	t, errT := time.Parse("15:04", dayTo)
	if errF != nil || errT != nil || !t.After(f) {
		return nil
	}
	var out []Interval
	start := from.In(loc)
	for d := time.Date(start.Year(), start.Month(), start.Day(), 0, 0, 0, 0, loc); d.Before(to); d = d.AddDate(0, 0, 1) {
		if len(days) > 0 && !slices.Contains(days, d.Weekday()) {
			continue
		}
		ws := maxT(time.Date(d.Year(), d.Month(), d.Day(), f.Hour(), f.Minute(), 0, 0, loc), from)
		we := minT(time.Date(d.Year(), d.Month(), d.Day(), t.Hour(), t.Minute(), 0, 0, loc), to)
		cur := ws
		for _, b := range busy {
			if !b.End.After(cur) || !b.Start.Before(we) {
				continue
			}
			if b.Start.Sub(cur) >= minLen {
				out = append(out, Interval{cur, b.Start})
			}
			cur = maxT(cur, b.End)
		}
		if we.Sub(cur) >= minLen {
			out = append(out, Interval{cur, we})
		}
	}
	return out
}

func maxT(a, b time.Time) time.Time {
	if a.After(b) {
		return a
	}
	return b
}

func minT(a, b time.Time) time.Time {
	if a.Before(b) {
		return a
	}
	return b
}
