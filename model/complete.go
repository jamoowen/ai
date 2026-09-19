package model

import (
	"context"
	"errors"
)

var ErrIncompleteStream = errors.New("model stream ended without a completed response")

func Complete(ctx context.Context, client Client, selectedModel Model, request Request) (Response, error) {
	stream, err := client.Stream(ctx, selectedModel, request)
	if err != nil {
		return Response{}, err
	}
	if stream == nil {
		return Response{}, ErrIncompleteStream
	}

	for event, err := range stream {
		if err != nil {
			return Response{}, err
		}
		if completed, ok := event.(ResponseCompleted); ok {
			return completed.Response, nil
		}
	}

	return Response{}, ErrIncompleteStream
}
