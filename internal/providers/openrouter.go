package providers

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/jamoowen/ai/internal/tools"
)

const openRouterEndpoint = "https://openrouter.ai/api/v1/chat/completions"

type OpenRouter struct {
	apiKey     string
	endpoint   string
	httpClient *http.Client
}

type completionResponse struct {
	Choices []struct {
		Message Message `json:"message"`
	} `json:"choices"`
}

func NewOpenRouter(apiKey string) *OpenRouter {
	return &OpenRouter{
		apiKey:     apiKey,
		endpoint:   openRouterEndpoint,
		httpClient: http.DefaultClient,
	}
}

func ToOpenRouterTool(t tools.Tool) map[string]any {
	return map[string]any{
		"type": "function",
		"function": map[string]any{
			"name":        t.Name,
			"description": t.Description,
			"parameters":  t.Parameters,
		},
	}
}

func (p *OpenRouter) Complete(ctx context.Context, completion CompletionRequest) (string, error) {
	if completion.Model == "" {
		fmt.Println("No model selected. Falling back to openrouter/free")
		completion.Model = "openrouter/free"
	}
	requestBody, err := json.Marshal(completion)
	if err != nil {
		return "", fmt.Errorf("encode completion request: %w", err)
	}

	request, err := http.NewRequestWithContext(ctx, http.MethodPost, p.endpoint, bytes.NewReader(requestBody))
	if err != nil {
		return "", fmt.Errorf("create completion request: %w", err)
	}
	request.Header.Set("Authorization", "Bearer "+p.apiKey)
	request.Header.Set("Content-Type", "application/json")

	response, err := p.httpClient.Do(request)
	if err != nil {
		return "", fmt.Errorf("error calling openrouter: %w", err)
	}
	defer response.Body.Close()

	responseBody, err := io.ReadAll(response.Body)
	if err != nil {
		return "", fmt.Errorf("read completion response: %w", err)
	}
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return "", fmt.Errorf("OpenRouter returned %s: %s", response.Status, strings.TrimSpace(string(responseBody)))
	}

	var completionResponse completionResponse
	if err := json.Unmarshal(responseBody, &completionResponse); err != nil {
		return "", fmt.Errorf("decode completion response: %w", err)
	}
	if len(completionResponse.Choices) == 0 || completionResponse.Choices[0].Message.Content == "" {
		return "", errors.New("completion response contained no message")
	}

	return completionResponse.Choices[0].Message.Content, nil
}
