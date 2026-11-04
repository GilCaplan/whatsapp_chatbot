package calendar

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"whatsappdoppel/internal/model"
)

const feed = "BEGIN:VCALENDAR\r\nBEGIN:VEVENT\r\nUID:a\r\nSUMMARY:Standup\r\nDTSTART;TZID=Europe/London:20261102T090000\r\nDTEND;TZID=Europe/London:20261102T093000\r\n" +
	"RRULE:FREQ=WEEKLY;BYDAY=MO,WE;COUNT=6\r\nEXDATE;TZID=Europe/London:20261104T090000\r\nEND:VEVENT\r\n" +
	"BEGIN:VEVENT\r\nUID:a\r\nRECURRENCE-ID;TZID=Europe/London:20261109T090000\r\nSUMMARY:Standup (moved)\r\nDTSTART;TZID=Europe/London:20261109T110000\r\nDTEND;TZID=Europe/London:20261109T113000\r\nEND:VEVENT\r\n" +
	"BEGIN:VEVENT\r\nUID:b\r\nSUMMARY:Holiday\r\nDTSTART;VALUE=DATE:20261105\r\nDTEND;VALUE=DATE:20261107\r\nEND:VEVENT\r\n" +
	"BEGIN:VEVENT\r\nUID:c\r\nSUMMARY:Lunch with a very long\r\n  title folded\r\nDTSTART:20261103T120000Z\r\nDURATION:PT1H30M\r\nEND:VEVENT\r\n" +
	"BEGIN:VEVENT\r\nUID:d\r\nSUMMARY:Cancelled\r\nSTATUS:CANCELLED\r\nDTSTART:20261103T150000Z\r\nEND:VEVENT\r\n" +
	"BEGIN:VEVENT\r\nUID:e\r\nSUMMARY:Monthly review\r\nDTSTART:20260930T140000Z\r\nDTEND:20260930T150000Z\r\nRRULE:FREQ=MONTHLY;BYDAY=-1WE\r\nEND:VEVENT\r\nEND:VCALENDAR\r\n"

func TestParseICS(t *testing.T) {
	lon, _ := time.LoadLocation("Europe/London")
	from := time.Date(2026, 11, 1, 0, 0, 0, 0, lon)
	evs, err := ParseICS(strings.NewReader(feed), from, from.AddDate(0, 0, 30), lon)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, e := range evs {
		got = append(got, e.Start.Format("Jan 2 15:04")+" "+e.Title)
	}
	want := []string{
		"Nov 2 09:00 Standup", "Nov 3 12:00 Lunch with a very long title folded", "Nov 5 00:00 Holiday",
		"Nov 9 11:00 Standup (moved)", "Nov 11 09:00 Standup", "Nov 16 09:00 Standup", "Nov 18 09:00 Standup",
		"Nov 25 14:00 Monthly review",
	}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Errorf("events:\n got %q\nwant %q", got, want)
	}
	for _, e := range evs {
		if e.Title == "Holiday" && (!e.AllDay || e.End.Sub(e.Start) != 48*time.Hour) {
			t.Errorf("holiday = %+v", e)
		}
		if strings.HasPrefix(e.Title, "Lunch") && e.End.Sub(e.Start) != 90*time.Minute {
			t.Errorf("lunch length = %v", e.End.Sub(e.Start))
		}
	}
}

func TestServiceAndFreeSlots(t *testing.T) {
	day := time.Date(2026, 11, 4, 0, 0, 0, 0, time.Local) // a Wednesday
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-API-Key") != "k" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		if r.URL.Query().Get("week_start") != "2026-11-02" {
			_ = json.NewEncoder(w).Encode([]any{})
			return
		}
		_ = json.NewEncoder(w).Encode([]map[string]any{
			{"title": "Dentist", "date": "2026-11-04", "start_time": "10:00", "end_time": "11:00"},
			{"title": "Call", "date": "2026-11-04", "start_time": "10:30", "end_time": "12:00"},
			{"title": "Birthday", "date": "2026-11-05", "start_time": nil},
		})
	}))
	defer srv.Close()
	s := New(func() time.Time { return day.Add(8 * time.Hour) })
	src := model.CalendarSource{ID: "m", Name: "MACalendar", Kind: model.CalendarMACalendar, URL: srv.URL, APIKey: "k", Enabled: true}
	busy, sts := s.Busy(context.Background(), []model.CalendarSource{src}, day, day.AddDate(0, 0, 2), true)
	if len(sts) != 1 || !sts[0].OK || len(busy) != 1 || busy[0].Start.Hour() != 10 || busy[0].End.Hour() != 12 {
		t.Fatalf("busy = %+v %+v", busy, sts)
	}
	free := FreeSlots(busy, day.Add(8*time.Hour), day.AddDate(0, 0, 1), []time.Weekday{time.Wednesday}, "09:00", "13:00", 30*time.Minute, time.Local)
	if len(free) != 2 || free[0].Start.Hour() != 9 || free[0].End.Hour() != 10 || free[1].Start.Hour() != 12 {
		t.Errorf("free = %+v", free)
	}
	src.APIKey = "wrong"
	if _, st := s.Events(context.Background(), src, true); st.OK || !strings.Contains(st.Error, "refused") {
		t.Errorf("bad key status = %+v", st)
	}
}
