package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/jamoowen/ai/internal/agents/simpleagent"
	"github.com/jamoowen/ai/internal/providers"
)

func main() {
	model := flag.String("model", "", "model to call, for example openai/gpt-4.1-mini")
	// this is the system prompt?
	instruction := flag.String("instruction", "You are a helpful, concise assistant.", "agent instructions")
	flag.Parse()

	prompt := strings.TrimSpace(strings.Join(flag.Args(), " "))
	if prompt == "" {
		fmt.Fprintln(os.Stderr, "usage: simple-agent -model <model> [flags] <prompt>")
		flag.PrintDefaults()
		os.Exit(2)
	}

	// only openrouter for now
	apiKey := os.Getenv("OPENROUTER_API_KEY")
	if apiKey == "" {
		fmt.Fprintln(os.Stderr, "OPENROUTER_API_KEY is required")
		os.Exit(2)
	}

	agent := simpleagent.New(providers.NewOpenRouter(apiKey), *model, *instruction)
	answer, err := agent.Run(context.Background(), prompt)
	if err != nil {
		fmt.Fprintln(os.Stderr, "agent error:", err)
		os.Exit(1)
	}

	fmt.Println(answer)
}
