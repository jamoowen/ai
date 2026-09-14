package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"

	"github.com/google/jsonschema-go/jsonschema"
)

type ToolHandler func(
	ctx context.Context,
	args json.RawMessage,
) (string, error)

type Tool struct {
	Name        string             `json:"name"`
	Description string             `json:"description"`
	Parameters  *jsonschema.Schema `json:"parameters"`

	// Local implementation; never included in the API request.
	Handler ToolHandler `json:"-"`
}

// NewTool creates a tool whose arguments are described by T.
// T must be a struct with ordinary JSON-serializable fields.
func NewTool[T any](
	name string,
	description string,
	handler func(context.Context, T) (string, error),
) (Tool, error) {
	if strings.TrimSpace(name) == "" {
		return Tool{}, fmt.Errorf("tool name must not be empty")
	}
	if handler == nil {
		return Tool{}, fmt.Errorf("tool %q: handler must not be nil", name)
	}
	if reflect.TypeFor[T]().Kind() != reflect.Struct {
		return Tool{}, fmt.Errorf(
			"tool %q: input type must be a struct",
			name,
		)
	}

	// Generate the model-facing schema once, at registration time.
	schema, err := jsonschema.For[T](nil)
	if err != nil {
		return Tool{}, fmt.Errorf(
			"tool %q: generate schema: %w",
			name,
			err,
		)
	}
	if schema.Type != "object" {
		return Tool{}, fmt.Errorf(
			"tool %q: input schema must describe an object",
			name,
		)
	}

	// Prepare the schema for repeated validation.
	// Do not modify schema after this call.
	resolved, err := schema.Resolve(nil)
	if err != nil {
		return Tool{}, fmt.Errorf(
			"tool %q: resolve schema: %w",
			name,
			err,
		)
	}

	return Tool{
		Name:        name,
		Description: description,
		Parameters:  schema,

		Handler: func(
			ctx context.Context,
			raw json.RawMessage,
		) (string, error) {
			if err := ctx.Err(); err != nil {
				return "", err
			}

			// Preserve which properties were actually supplied.
			var value any
			if err := json.Unmarshal(raw, &value); err != nil {
				return "", fmt.Errorf(
					"tool %q: invalid argument JSON: %w",
					name,
					err,
				)
			}

			if err := resolved.Validate(value); err != nil {
				return "", fmt.Errorf(
					"tool %q: arguments violate schema: %w",
					name,
					err,
				)
			}

			// Decode into the type expected by the implementation.
			var input T
			if err := json.Unmarshal(raw, &input); err != nil {
				return "", fmt.Errorf(
					"tool %q: decode arguments: %w",
					name,
					err,
				)
			}

			return handler(ctx, input)
		},
	}, nil
}
