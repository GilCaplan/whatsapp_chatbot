package engine

import (
	"context"
	"fmt"
	"strings"

	"whatsappdoppel/internal/llm"
	"whatsappdoppel/internal/model"
	"whatsappdoppel/internal/prompt"
)

// Clone yourself (wave 3, owner: Engineer A; WAVE3_PLAN §3.11). With your
// consent Doppel keeps a private sample of your own messages
// (cache/self-samples.jsonl); the builder turns them into a persona that
// texts like you. The draft is never saved here — the UI opens it in the
// persona editor to review first.

// CloneDraft writes a persona that texts like you from the stored samples of
// your own messages; the result is not saved.
func (b builder) CloneDraft(ctx context.Context, req model.CloneRequest) (model.CloneDraft, error) {
	samples, err := b.e.store.SelfSamples(0)
	if err != nil {
		return model.CloneDraft{}, err
	}
	texts := make([]string, 0, len(samples))
	for _, s := range samples {
		if t := strings.TrimSpace(s.Text); t != "" {
			texts = append(texts, t)
		}
	}
	if len(texts) < model.MinCloneSamples {
		return model.CloneDraft{SampleCount: len(texts)}, fmt.Errorf("need at least %d of your messages, have %d", model.MinCloneSamples, len(texts))
	}
	var choice *model.LLMChoice
	if req.Provider != "" || req.Model != "" {
		choice = &model.LLMChoice{Provider: req.Provider, Model: req.Model}
	}
	prov, mdl, err := b.e.llm.Resolve(choice)
	if err != nil {
		return model.CloneDraft{}, err
	}
	r := prompt.CloneBuilder(req.Name, texts, req.Extra)
	r.Model = mdl
	ctx, cancel := context.WithTimeout(ctx, llm.TimeoutBuilder)
	defer cancel()
	resp, err := prov.Chat(ctx, r)
	if err != nil {
		return model.CloneDraft{}, err
	}
	p, err := prompt.ParseDraft(resp.Text)
	if err != nil {
		return model.CloneDraft{Raw: resp.Text}, err
	}
	if name := strings.TrimSpace(req.Name); name != "" {
		p.Name = name
	}
	n := min(len(texts), prompt.MaxCloneSamples)
	return model.CloneDraft{Persona: p, Raw: resp.Text, SampleCount: n}, nil
}
