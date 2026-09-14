package simpleagentwithloop

import (
	"context"

	"github.com/jamoowen/ai/internal/providers"
)

type Agent struct {
	provider providers.Provider
	model    string
	history  []providers.Message
}

func New(provider providers.Provider, model string, instruction string) *Agent {
	return &Agent{
		provider: provider,
		model:    model,
		history: []providers.Message{
			{Role: providers.RoleSystem, Content: instruction},
		},
	}
}

func (a *Agent) Run(ctx context.Context, prompt string) (string, error) {
	messages := make([]providers.Message, len(a.history), len(a.history)+2)
	copy(messages, a.history)
	messages = append(messages, providers.Message{Role: providers.RoleUser, Content: prompt})

	answer, err := a.provider.Complete(ctx, providers.CompletionRequest{
		Model:    a.model,
		Messages: messages,
	})
	if err != nil {
		return "", err
	}

	a.history = append(messages, providers.Message{Role: providers.RoleAssistant, Content: answer})
	return answer, nil
}
