package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/jamoowen/ai/internal/agents/simpleagentwithloop"
	"github.com/jamoowen/ai/internal/providers"
)

type agentRunner interface {
	Run(context.Context, string) (string, error)
}

func main() {
	model := flag.String("model", "", "model to call, for example openai/gpt-4.1-mini")
	instruction := flag.String("instruction", "You are a helpful, concise assistant.", "agent instructions")
	help := flag.Bool("help", false, "show usage")
	flag.Usage = func() {
		fmt.Fprintf(flag.CommandLine.Output(), "usage: %s [flags]\n", os.Args[0])
		flag.PrintDefaults()
	}
	flag.Parse()

	if *help {
		flag.Usage()
		return
	}
	if flag.NArg() != 0 {
		flag.Usage()
		os.Exit(2)
	}

	apiKey := os.Getenv("OPENROUTER_API_KEY")
	if apiKey == "" {
		fmt.Fprintln(os.Stderr, "OPENROUTER_API_KEY is required")
		os.Exit(2)
	}

	agent := simpleagentwithloop.New(providers.NewOpenRouter(apiKey), *model, *instruction)
	if err := run(context.Background(), os.Stdin, os.Stdout, agent); err != nil {
		fmt.Fprintln(os.Stderr, "agent error:", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, input io.Reader, output io.Writer, agent agentRunner) error {
	scanner := bufio.NewScanner(input)
	for {
		if _, err := fmt.Fprint(output, "> "); err != nil {
			return fmt.Errorf("write prompt: %w", err)
		}
		if !scanner.Scan() {
			if err := scanner.Err(); err != nil {
				return fmt.Errorf("read prompt: %w", err)
			}
			return nil
		}

		prompt := strings.TrimSpace(scanner.Text())
		if prompt == "" {
			continue
		}
		if prompt == "/quit" {
			return nil
		}

		answer, err := agent.Run(ctx, prompt)
		if err != nil {
			return fmt.Errorf("run agent: %w", err)
		}
		if _, err := fmt.Fprintln(output, answer); err != nil {
			return fmt.Errorf("write answer: %w", err)
		}
	}
}
