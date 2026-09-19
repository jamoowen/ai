package model

import (
	"context"
	"iter"
)

type Event interface {
	isEvent()
}

type TextDelta struct {
	Text string
}

func (TextDelta) isEvent() {}

type ResponseCompleted struct {
	Response Response
}

func (ResponseCompleted) isEvent() {}

type EventStream = iter.Seq2[Event, error]

type Client interface {
	Stream(context.Context, Model, Request) (EventStream, error)
}
