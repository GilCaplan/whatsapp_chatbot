package engine

import (
	"context"
	"errors"
	"testing"
	"time"

	"whatsappdoppel/internal/model"
)

// Phase 0 contract tests for wave 3: co-pilot draft selection and the stubs
// that answer ErrNotImplemented until their feature lands.

func TestSendApprovedPicksDraft(t *testing.T) {
	c := dmChat()
	c.ApprovalMode = true
	h := newHarness(t, nil, c)
	h.simulate(dmKey, "dinner tonight?", false)
	h.clk.Advance(9 * time.Second)
	eventually(t, "pending reply", func() bool { return len(h.st.Approvals()) == 1 })
	h.idle()
	p := h.st.Approvals()[0]
	p.Drafts = []model.Draft{
		{Tone: model.DraftToneBrief, Text: "sure"},
		{Tone: model.DraftToneWarm, Text: "yes please, I'm starving"},
		{Tone: model.DraftTonePlayful, Text: "only if you're cooking"},
	}
	p.Text = p.Drafts[0].Text
	if err := h.st.UpsertApproval(p); err != nil {
		t.Fatal(err)
	}
	if err := h.e.SendApproved(context.Background(), p.ID, "", 3); !errors.Is(err, ErrNoSuchDraft) {
		t.Fatalf("draft 3: %v", err)
	}
	if err := h.e.SendApproved(context.Background(), p.ID, "", 1); err != nil {
		t.Fatal(err)
	}
	if sent := h.wa.Sent(); len(sent) != 1 || sent[0].text != "yes please, I'm starving" || sent[0].id == "" {
		t.Fatalf("sent = %+v", sent)
	}
}

func TestWave3StubsNotImplemented(t *testing.T) {
	h := newHarness(t, nil, dmChat())
	ctx := context.Background()
	// Every wave 3 engine feature has landed: ResumeHandoff, Reveal and
	// GenerateRecap (wave3b_test.go), ExtractMemories and CloneDraft
	// (realism_test.go). None may answer ErrNotImplemented any more.
	if _, err := h.e.ExtractMemories(ctx, dmKey); errors.Is(err, ErrNotImplemented) {
		t.Errorf("ExtractMemories: %v", err)
	}
	if _, err := h.e.Builder().CloneDraft(ctx, model.CloneRequest{}); errors.Is(err, ErrNotImplemented) || err == nil {
		t.Errorf("CloneDraft without samples: %v", err)
	}
}
