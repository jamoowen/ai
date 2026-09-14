package main

import (
	"bytes"
	"context"
	"errors"
	"io"
	"reflect"
	"strings"
	"testing"
)

type recordingAgent struct {
	answers []string
	prompts []string
}

func (a *recordingAgent) Run(_ context.Context, prompt string) (string, error) {
	a.prompts = append(a.prompts, prompt)
	return a.answers[len(a.prompts)-1], nil
}

func TestRunProcessesPromptsUntilQuit(t *testing.T) {
	agent := &recordingAgent{answers: []string{"first answer"}}
	output := &bytes.Buffer{}

	err := run(context.Background(), strings.NewReader("first prompt\n/quit\n"), output, agent)
	if err != nil {
		t.Fatalf("run() error = %v", err)
	}
	if !reflect.DeepEqual(agent.prompts, []string{"first prompt"}) {
		t.Errorf("prompts = %#v, want %#v", agent.prompts, []string{"first prompt"})
	}
	if !strings.Contains(output.String(), "first answer") {
		t.Errorf("output = %q, want it to contain %q", output.String(), "first answer")
	}
}

type errorReader struct {
	err error
}

func (r errorReader) Read([]byte) (int, error) {
	return 0, r.err
}

func TestRunStopsAtEndOfInput(t *testing.T) {
	agent := &recordingAgent{}

	err := run(context.Background(), strings.NewReader(""), io.Discard, agent)
	if err != nil {
		t.Fatalf("run() error = %v", err)
	}
	if len(agent.prompts) != 0 {
		t.Errorf("prompt count = %d, want 0", len(agent.prompts))
	}
}

func TestRunReturnsInputError(t *testing.T) {
	readError := errors.New("input failed")

	err := run(context.Background(), errorReader{err: readError}, io.Discard, &recordingAgent{})
	if !errors.Is(err, readError) {
		t.Errorf("run() error = %v, want error wrapping %v", err, readError)
	}
}
