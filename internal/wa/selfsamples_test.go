package wa

import (
	"strings"
	"testing"
	"time"

	"whatsappdoppel/internal/model"
)

func TestSelfSampleRedaction(t *testing.T) {
	in := model.Incoming{IsFromMe: true, Text: "call me on +972 54-123-4567 or a.b@x.com, pics at https://x.co/abc", Timestamp: time.Unix(100, 0)}
	s, ok := selfSample(in)
	if !ok || s.Text != "call me on [number] or [email], pics at [link]" || s.Kind != "dm" || !s.TS.Equal(time.Unix(100, 0)) {
		t.Fatalf("sample = %+v %v", s, ok)
	}
	if _, ok := selfSample(model.Incoming{Text: "https://x.com/y"}); ok {
		t.Error("a bare link is not a sample")
	}
	if s, _ := selfSample(model.Incoming{Text: "mine", IsGroup: true}); s.Text != "mine" || s.Kind != "group" {
		t.Errorf("short text = %+v", s)
	}
	if _, ok := selfSample(model.Incoming{Text: "[photo]", Media: model.MediaImage}); ok {
		t.Error("a bare photo is not a sample")
	}
	if s, _ := selfSample(model.Incoming{Text: "[photo] look at this", Media: model.MediaImage}); s.Text != "look at this" {
		t.Errorf("caption = %+v", s)
	}
	long, _ := selfSample(model.Incoming{Text: strings.Repeat("a", 500)})
	if len([]rune(long.Text)) != maxSampleRunes {
		t.Error("cap")
	}
}

func TestSampleLiveConsentAndOwnSends(t *testing.T) {
	var got []model.SelfSample
	consent := false
	m := &Manager{collectSelf: func() bool { return consent }, onSelf: func(xs []model.SelfSample) { got = append(got, xs...) }}
	mine := model.Incoming{IsFromMe: true, MessageID: "A1", Text: "on my way"}
	m.sampleLive(mine)
	if len(got) != 0 {
		t.Fatal("collected without consent")
	}
	consent = true
	m.sampleLive(model.Incoming{IsFromMe: false, Text: "from them"})
	m.sent.add("BOT1")
	m.sampleLive(model.Incoming{IsFromMe: true, MessageID: "BOT1", Text: "persona reply"})
	m.sampleLive(mine)
	if len(got) != 1 || got[0].Text != "on my way" {
		t.Fatalf("got = %+v", got)
	}
	for i := 0; i < sentIDsKept+10; i++ {
		m.sent.add(string(rune('a' + i%26)))
	}
	if len(m.sent.ids) != sentIDsKept {
		t.Errorf("sent ids not capped: %d", len(m.sent.ids))
	}
}
