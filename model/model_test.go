package model_test

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/jamoowen/ai/model"
)

type streamClient func(context.Context, model.Model, model.Request) (model.EventStream, error)

func (client streamClient) Stream(ctx context.Context, selectedModel model.Model, request model.Request) (model.EventStream, error) {
	return client(ctx, selectedModel, request)
}

func TestNewTextMessages(t *testing.T) {
	tests := []struct {
		name string
		got  model.Message
		want model.Message
	}{
		{
			name: "user",
			got:  model.NewUserMessage("hello"),
			want: model.Message{
				Role:    model.RoleUser,
				Content: []model.Content{model.TextContent{Text: "hello"}},
			},
		},
		{
			name: "assistant",
			got:  model.NewAssistantMessage("hello back"),
			want: model.Message{
				Role:    model.RoleAssistant,
				Content: []model.Content{model.TextContent{Text: "hello back"}},
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if !reflect.DeepEqual(test.got, test.want) {
				t.Errorf("message = %#v, want %#v", test.got, test.want)
			}
		})
	}
}

func TestCompleteReturnsCompletedResponse(t *testing.T) {
	selectedModel := model.Model{Provider: "test", ID: "test-model"}
	request := model.Request{
		SystemPrompt: "Be concise.",
		Messages:     []model.Message{model.NewUserMessage("hello")},
	}
	want := model.Response{
		Message:      model.NewAssistantMessage("hello back"),
		FinishReason: model.FinishReasonStop,
		Usage:        model.Usage{InputTokens: 4, OutputTokens: 2},
	}

	client := streamClient(func(ctx context.Context, gotModel model.Model, gotRequest model.Request) (model.EventStream, error) {
		if ctx != t.Context() {
			t.Error("Complete() did not forward its context")
		}
		if gotModel != selectedModel {
			t.Errorf("model = %#v, want %#v", gotModel, selectedModel)
		}
		if !reflect.DeepEqual(gotRequest, request) {
			t.Errorf("request = %#v, want %#v", gotRequest, request)
		}

		return func(yield func(model.Event, error) bool) {
			if !yield(model.TextDelta{Text: "hello "}, nil) {
				return
			}
			yield(model.ResponseCompleted{Response: want}, nil)
		}, nil
	})

	got, err := model.Complete(t.Context(), client, selectedModel, request)
	if err != nil {
		t.Fatalf("Complete() error = %v", err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Complete() = %#v, want %#v", got, want)
	}
}

func TestCompleteReturnsStreamSetupError(t *testing.T) {
	want := errors.New("start stream")
	client := streamClient(func(context.Context, model.Model, model.Request) (model.EventStream, error) {
		return nil, want
	})

	_, err := model.Complete(t.Context(), client, model.Model{}, model.Request{})
	if !errors.Is(err, want) {
		t.Errorf("Complete() error = %v, want %v", err, want)
	}
}

func TestCompleteReturnsMidStreamError(t *testing.T) {
	want := errors.New("read stream")
	client := streamClient(func(context.Context, model.Model, model.Request) (model.EventStream, error) {
		return func(yield func(model.Event, error) bool) {
			if !yield(model.TextDelta{Text: "partial"}, nil) {
				return
			}
			yield(nil, want)
		}, nil
	})

	_, err := model.Complete(t.Context(), client, model.Model{}, model.Request{})
	if !errors.Is(err, want) {
		t.Errorf("Complete() error = %v, want %v", err, want)
	}
}

func TestCompleteRejectsIncompleteStream(t *testing.T) {
	tests := []struct {
		name   string
		stream model.EventStream
	}{
		{name: "nil", stream: nil},
		{
			name: "no completed response",
			stream: func(yield func(model.Event, error) bool) {
				yield(model.TextDelta{Text: "partial"}, nil)
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			client := streamClient(func(context.Context, model.Model, model.Request) (model.EventStream, error) {
				return test.stream, nil
			})

			_, err := model.Complete(t.Context(), client, model.Model{}, model.Request{})
			if !errors.Is(err, model.ErrIncompleteStream) {
				t.Errorf("Complete() error = %v, want %v", err, model.ErrIncompleteStream)
			}
		})
	}
}
