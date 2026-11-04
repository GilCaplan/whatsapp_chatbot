package engine

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"whatsappdoppel/internal/core/model"
	"whatsappdoppel/internal/runtime/llm"
)

func TestConnectionRatings(t *testing.T) {
	h := newHarness(t, nil, groupChat())
	h.llm.SetFunc(func(req llm.Request) (llm.Response, error) {
		if strings.Contains(req.System, "rate how connected you felt") {
			return llm.Response{Text: `{"people":[{"name":"Avi","score":55,"why":"mostly talked to Dana"},` +
				`{"name":"Dana","score":"88","why":"we laughed all day"},{"name":"Ghost","score":99,"why":"made up"}]}`}, nil
		}
		return llm.Response{Text: "ok"}, nil
	})
	seed := func(n int) {
		for i := range n {
			m := model.Message{TS: h.clk.Now().Add(-time.Duration(n-i) * time.Minute), Speaker: "them", Text: fmt.Sprintf("m%d", i)}
			if i%2 == 0 {
				m.Name, m.SenderJID = "Dana", danaLID
			} else {
				m.Name, m.SenderJID = "Avi", aviPN
			}
			if err := h.st.AppendHistory(groupKey, m, 0); err != nil {
				t.Fatal(err)
			}
		}
	}
	seed(10)
	c, _ := h.st.Chat(groupKey)
	if _, err := h.e.rateChat(t.Context(), c, false); err != ErrNothingToRate {
		t.Fatalf("10 messages: err = %v, want nothing to rate", err)
	}
	// Yesterday's ratings, for the change.
	yesterday := h.clk.Now().Add(-24 * time.Hour).In(time.Local).Format("2006-01-02")
	if err := h.st.SaveRating(model.RatingDay{ChatKey: groupKey, Date: yesterday, Scores: []model.PersonScore{{Name: "Dana", JID: danaLID, Score: 80}}}); err != nil {
		t.Fatal(err)
	}
	seed(12)
	d, err := h.e.rateChat(t.Context(), c, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(d.Scores) != 2 || d.Scores[0].Name != "Dana" || d.Scores[0].Score != 88 || d.Scores[1].Name != "Avi" {
		t.Fatalf("scores = %+v", d.Scores)
	}
	if ch := d.Scores[0].Change; ch == nil || *ch != 8 || d.Scores[1].Change != nil {
		t.Errorf("changes = %v / %v", d.Scores[0].Change, d.Scores[1].Change)
	}
	if _, err := h.e.rateChat(t.Context(), c, false); err != ErrNothingToRate {
		t.Errorf("rated twice in a day: %v", err)
	}
	if got, _ := h.st.Ratings(groupKey); len(got) != 2 || got[0].Date == yesterday {
		t.Errorf("stored = %+v", got)
	}
	if len(h.acts(model.ActRating)) != 1 {
		t.Error("no rating activity")
	}
}
