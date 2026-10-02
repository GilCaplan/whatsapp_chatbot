package app

import (
	"log"
	"strings"

	"whatsappdoppel/internal/config"
	"whatsappdoppel/internal/model"
	"whatsappdoppel/internal/store"
)

// Clone yourself (wave 3, Engineer A): samples of your own messages are kept
// only while Settings › Data › "Keep a private sample of my messages" is on.

// selfSampleConsent reports the current consent (read on every message).
func selfSampleConsent(cfg *config.Manager) func() bool {
	return func() bool { return cfg.Get().SelfClone.CollectSamples }
}

// selfSampleSink stores samples (FIFO, settings clone.maxSamples). Trigger
// messages ("1 hey") are commands to a persona, not how you text: skipped.
func selfSampleSink(cfg *config.Manager, st *store.Store) func([]model.SelfSample) {
	return func(xs []model.SelfSample) {
		s := cfg.Get()
		if !s.SelfClone.CollectSamples {
			return
		}
		prefix := strings.TrimSpace(s.Behavior.TriggerPrefix)
		keep := xs[:0:0]
		for _, x := range xs {
			t := strings.TrimSpace(x.Text)
			if prefix != "" && (t == prefix || strings.HasPrefix(t, prefix+" ")) {
				continue
			}
			keep = append(keep, x)
		}
		if err := st.AppendSelfSamples(keep, s.SelfClone.MaxSamples); err != nil {
			log.Printf("clone: couldn't store samples: %v", err)
		}
	}
}
