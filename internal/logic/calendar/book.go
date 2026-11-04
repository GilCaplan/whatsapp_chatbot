package calendar

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"whatsappdoppel/internal/core/model"
)

// Booking a meeting a task agreed on. The calendars are only ever read,
// except here, when you (or your auto-confirm rule) confirmed a time:
// MACalendar's own POST /events, unchanged, or an .ics file you open.

// BookMACalendar creates the event in MACalendar (POST {base}/events with
// its X-API-Key). token makes a retry safe: MACalendar books one event per
// client_token. It returns MACalendar's event id.
func (s *Service) BookMACalendar(ctx context.Context, src model.CalendarSource, title string, start, end time.Time, token string) (string, error) {
	if src.Kind != model.CalendarMACalendar {
		return "", errors.New("that calendar isn't MACalendar")
	}
	base := strings.TrimRight(strings.TrimSpace(src.URL), "/")
	if base == "" {
		base = "http://127.0.0.1:8080"
	}
	pu, err := url.Parse(base)
	if err != nil || (pu.Scheme != "http" && pu.Scheme != "https") || pu.Host == "" {
		return "", errors.New("MACalendar's address must start with http:// or https://")
	}
	start, end = start.In(time.Local), end.In(time.Local) // MACalendar's times are local
	body, _ := json.Marshal(map[string]string{
		"title": title, "date": start.Format("2006-01-02"), "start_time": start.Format("15:04"), "end_time": end.Format("15:04"),
		"client_token": token,
	})
	ctx, cancel := context.WithTimeout(ctx, fetchTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, base+"/events", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	if k := strings.TrimSpace(src.APIKey); k != "" {
		req.Header.Set("X-API-Key", k)
	}
	resp, err := s.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("couldn't reach MACalendar (%s)", pu.Host)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
			return "", fmt.Errorf("MACalendar refused access (%d): check the key", resp.StatusCode)
		}
		return "", fmt.Errorf("MACalendar answered %d", resp.StatusCode)
	}
	var v struct {
		ID json.RawMessage `json:"id"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&v); err != nil || len(v.ID) == 0 {
		return "", errors.New("MACalendar didn't say which event it made")
	}
	s.mu.Lock()
	delete(s.cache, cacheKey(src)) // read it again next time: the new event is busy now
	s.mu.Unlock()
	return strings.Trim(string(v.ID), `"`), nil
}

func icsEscape(s string) string {
	r := strings.NewReplacer(`\`, `\\`, ";", `\;`, ",", `\,`, "\r\n", `\n`, "\n", `\n`, "\r", `\n`)
	return r.Replace(s)
}

// EventICS is a one-event calendar file (UTC times) for a booked meeting.
func EventICS(uid, title, place string, start, end, stamp time.Time) []byte {
	f := "20060102T150405Z"
	var b strings.Builder
	b.WriteString("BEGIN:VCALENDAR\r\nVERSION:2.0\r\nPRODID:-//WhatsApp Doppel//Tasks//EN\r\nCALSCALE:GREGORIAN\r\nMETHOD:PUBLISH\r\n")
	b.WriteString("BEGIN:VEVENT\r\n")
	b.WriteString("UID:" + icsEscape(uid) + "@whatsapp-doppel\r\n")
	b.WriteString("DTSTAMP:" + stamp.UTC().Format(f) + "\r\n")
	b.WriteString("DTSTART:" + start.UTC().Format(f) + "\r\n")
	b.WriteString("DTEND:" + end.UTC().Format(f) + "\r\n")
	b.WriteString("SUMMARY:" + icsEscape(title) + "\r\n")
	if place != "" {
		b.WriteString("LOCATION:" + icsEscape(place) + "\r\n")
	}
	b.WriteString("END:VEVENT\r\nEND:VCALENDAR\r\n")
	return []byte(b.String())
}

// Titles are the titles of the enabled sources' events in [from, to), for
// the task leak check only (never shown to a model).
func (s *Service) Titles(ctx context.Context, srcs []model.CalendarSource, from, to time.Time) []string {
	var out []string
	for _, src := range srcs {
		if !src.Enabled {
			continue
		}
		evs, _ := s.Events(ctx, src, false)
		for _, e := range evs {
			if e.Title != "" && e.End.After(from) && e.Start.Before(to) {
				out = append(out, e.Title)
			}
		}
	}
	return out
}
