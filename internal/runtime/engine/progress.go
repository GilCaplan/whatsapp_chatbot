package engine

import (
	"context"

	"whatsappdoppel/internal/core/model"
)

// Reply progress: the live steps of writing one reply (ActProgress), shown on
// the chat's Log tab. The context carries the chat, so the steps deep in the
// writing code report only for real replies, never for the playground or
// other previews.

type progressKey struct{}

type progressNote struct {
	e    *Engine
	c    model.ChatAssignment
	name string
}

func (e *Engine) withProgress(ctx context.Context, c model.ChatAssignment, name string) context.Context {
	return context.WithValue(ctx, progressKey{}, progressNote{e, c, name})
}

// progress reports a step of the reply being written in ctx.
func progress(ctx context.Context, text string, meta map[string]any) {
	if n, ok := ctx.Value(progressKey{}).(progressNote); ok {
		n.e.act(model.ActProgress, n.c, n.name, text, meta)
	}
}
