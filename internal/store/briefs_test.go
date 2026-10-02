package store

import (
	"os"
	"runtime"
	"testing"
	"time"

	"whatsappdoppel/internal/model"
)

func TestBriefsRoundTrip(t *testing.T) {
	s, _ := openTemp(t)
	if _, ok, err := s.Brief("group:1@g.us"); ok || err != nil {
		t.Fatalf("missing: %v %v", ok, err)
	}
	now := time.Now().UTC().Truncate(time.Second)
	b := model.Brief{ChatKey: "group:1@g.us", PersonaID: "leo", Kind: "group", Topics: []string{"brunch"},
		Commitments: []string{"bring wine on Sat 4 Oct"}, People: []model.BriefPerson{{Name: "Dana", Note: "pushed for Saturday"}}, GeneratedAt: now}
	if err := s.SaveBrief(b); err != nil {
		t.Fatal(err)
	}
	got, ok, err := s.Brief("group:1@g.us")
	if !ok || err != nil || got.Commitments[0] != "bring wine on Sat 4 Oct" || !got.GeneratedAt.Equal(now) || got.People[0].Name != "Dana" {
		t.Fatalf("read: %+v %v %v", got, ok, err)
	}
	if st, err := os.Stat(s.briefFile("group:1@g.us")); err != nil || (runtime.GOOS != "windows" && st.Mode().Perm() != 0o600) {
		t.Errorf("file mode: %v %v", st, err)
	}
	if err := s.SaveBrief(model.Brief{}); err == nil {
		t.Error("brief without chat saved")
	}
	for range 2 { // idempotent
		if err := s.DeleteBrief("group:1@g.us"); err != nil {
			t.Fatal(err)
		}
	}
	if _, ok, _ := s.Brief("group:1@g.us"); ok {
		t.Error("still there")
	}
}
