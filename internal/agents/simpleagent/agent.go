package simpleagent

import (
	"context"

	"github.com/jamoowen/ai/internal/providers"
)

type Agent struct {
	provider    providers.Provider
	model       string
	instruction string
}

func New(provider providers.Provider, model string, instruction string) Agent {
	return Agent{
		provider:    provider,
		model:       model,
		instruction: instruction,
	}
}

func (a Agent) Run(ctx context.Context, prompt string) (string, error) {
	return a.provider.Complete(ctx, providers.CompletionRequest{
		Model: a.model,
		Messages: []providers.Message{
			{Role: "system", Content: a.instruction},
			{Role: "user", Content: prompt},
		},
	})
}
