package simpleagent

import (
	"context"
	"testing"

	"github.com/jamoowen/ai/internal/providers"
)

type recordingProvider struct {
	request providers.CompletionRequest
}

func (p *recordingProvider) Complete(_ context.Context, request providers.CompletionRequest) (string, error) {
	p.request = request
	return "agent answer", nil
}

func TestRunSendsInstructionAndPromptToProvider(t *testing.T) {
	provider := &recordingProvider{}
	agent := New(provider, "test-model", "Be concise.")

	answer, err := agent.Run(context.Background(), "What is Go?")
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if answer != "agent answer" {
		t.Errorf("Run() answer = %q, want %q", answer, "agent answer")
	}

	want := providers.CompletionRequest{
		Model: "test-model",
		Messages: []providers.Message{
			{Role: "system", Content: "Be concise."},
			{Role: "user", Content: "What is Go?"},
		},
	}
	if provider.request.Model != want.Model {
		t.Errorf("request model = %q, want %q", provider.request.Model, want.Model)
	}
	if len(provider.request.Messages) != len(want.Messages) {
		t.Fatalf("request message count = %d, want %d", len(provider.request.Messages), len(want.Messages))
	}
	for index, message := range provider.request.Messages {
		if message != want.Messages[index] {
			t.Errorf("request message %d = %#v, want %#v", index, message, want.Messages[index])
		}
	}
}
