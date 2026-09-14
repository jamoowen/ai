package simpleagentwithloop

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"testing"

	"github.com/jamoowen/ai/internal/providers"
)

type completionResult struct {
	answer string
	err    error
}

type scriptedProvider struct {
	results  []completionResult
	requests []providers.CompletionRequest
}

func (p *scriptedProvider) Complete(_ context.Context, request providers.CompletionRequest) (string, error) {
	p.requests = append(p.requests, request)
	result := p.results[len(p.requests)-1]
	return result.answer, result.err
}

func TestRunIncludesPreviousExchangeInLaterRequest(t *testing.T) {
	provider := &scriptedProvider{results: []completionResult{
		{answer: "first answer"},
		{answer: "second answer"},
	}}
	agent := New(provider, "test-model", "Be concise.")

	firstAnswer, err := agent.Run(context.Background(), "first prompt")
	if err != nil {
		t.Fatalf("first Run() error = %v", err)
	}
	if firstAnswer != "first answer" {
		t.Errorf("first Run() answer = %q, want %q", firstAnswer, "first answer")
	}

	secondAnswer, err := agent.Run(context.Background(), "second prompt")
	if err != nil {
		t.Fatalf("second Run() error = %v", err)
	}
	if secondAnswer != "second answer" {
		t.Errorf("second Run() answer = %q, want %q", secondAnswer, "second answer")
	}

	assertCompletionRequests(t, provider.requests, []providers.CompletionRequest{
		{
			Model: "test-model",
			Messages: []providers.Message{
				{Role: providers.RoleSystem, Content: "Be concise."},
				{Role: providers.RoleUser, Content: "first prompt"},
			},
		},
		{
			Model: "test-model",
			Messages: []providers.Message{
				{Role: providers.RoleSystem, Content: "Be concise."},
				{Role: providers.RoleUser, Content: "first prompt"},
				{Role: providers.RoleAssistant, Content: "first answer"},
				{Role: providers.RoleUser, Content: "second prompt"},
			},
		},
	})
}

func TestRunDoesNotRememberPromptWhenProviderFails(t *testing.T) {
	provider := &scriptedProvider{results: []completionResult{
		{err: errors.New("provider unavailable")},
		{answer: "second answer"},
	}}
	agent := New(provider, "test-model", "Be concise.")

	_, err := agent.Run(context.Background(), "failed prompt")
	if err == nil {
		t.Fatal("first Run() error = nil, want provider error")
	}

	answer, err := agent.Run(context.Background(), "second prompt")
	if err != nil {
		t.Fatalf("second Run() error = %v", err)
	}
	if answer != "second answer" {
		t.Errorf("second Run() answer = %q, want %q", answer, "second answer")
	}

	assertCompletionRequests(t, provider.requests, []providers.CompletionRequest{
		{
			Model: "test-model",
			Messages: []providers.Message{
				{Role: providers.RoleSystem, Content: "Be concise."},
				{Role: providers.RoleUser, Content: "failed prompt"},
			},
		},
		{
			Model: "test-model",
			Messages: []providers.Message{
				{Role: providers.RoleSystem, Content: "Be concise."},
				{Role: providers.RoleUser, Content: "second prompt"},
			},
		},
	})
}

func assertCompletionRequests(t *testing.T, got []providers.CompletionRequest, want []providers.CompletionRequest) {
	t.Helper()
	if !reflect.DeepEqual(got, want) {
		t.Errorf("completion requests = %s, want %s", formatRequests(got), formatRequests(want))
	}
}

func formatRequests(requests []providers.CompletionRequest) string {
	return fmt.Sprintf("%#v", requests)
}
