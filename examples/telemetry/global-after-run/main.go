//
// Tencent is pleased to support the open source community by making trpc-agent-go available.
//
// Copyright (C) 2025 Tencent.  All rights reserved.
//
// trpc-agent-go is licensed under the Apache License Version 2.0.
//

// Package main demonstrates observing a real LLMAgent run through a global
// Runner AfterRun hook.
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	oteltrace "go.opentelemetry.io/otel/trace"

	"trpc.group/trpc-go/trpc-agent-go/agent"
	"trpc.group/trpc-go/trpc-agent-go/agent/llmagent"
	agenttrace "trpc.group/trpc-go/trpc-agent-go/agent/trace"
	"trpc.group/trpc-go/trpc-agent-go/event"
	"trpc.group/trpc-go/trpc-agent-go/model"
	"trpc.group/trpc-go/trpc-agent-go/model/openai"
	"trpc.group/trpc-go/trpc-agent-go/plugin"
	"trpc.group/trpc-go/trpc-agent-go/runner"
	"trpc.group/trpc-go/trpc-agent-go/session/inmemory"
	telemetrytrace "trpc.group/trpc-go/trpc-agent-go/telemetry/trace"
)

var (
	modelName = flag.String(
		"model",
		envOrDefault("MODEL_NAME", "gpt-4o-mini"),
		"OpenAI-compatible model name",
	)
	prompt = flag.String(
		"prompt",
		"Explain in two sentences why execution traces are useful for AI agents.",
		"Message sent to the agent",
	)
	otelEndpoint = flag.String(
		"otel-endpoint",
		"localhost:4318",
		"OTLP HTTP endpoint in host:port form",
	)
)

func main() {
	flag.Parse()
	if strings.TrimSpace(os.Getenv("OPENAI_API_KEY")) == "" {
		log.Fatal("OPENAI_API_KEY is required")
	}

	ctx := context.Background()
	shutdownTelemetry, err := telemetrytrace.Start(
		ctx,
		telemetrytrace.WithProtocol("http"),
		telemetrytrace.WithEndpoint(*otelEndpoint),
		telemetrytrace.WithServiceName("global-after-run-example"),
	)
	if err != nil {
		log.Fatalf("start trace telemetry: %v", err)
	}
	defer func() {
		if err := shutdownTelemetry(); err != nil {
			log.Printf("shutdown trace telemetry: %v", err)
		}
	}()

	observations := make(chan runObservation, 1)
	if err := runner.RegisterGlobalAfterRunHook(
		"example.global-after-run",
		observeCompletedRun(observations),
	); err != nil {
		log.Fatalf("register global AfterRun hook: %v", err)
	}

	if err := run(ctx, observations); err != nil {
		log.Fatalf("run example: %v", err)
	}
}

func run(ctx context.Context, observations <-chan runObservation) error {
	agt := llmagent.New(
		"execution-trace-assistant",
		llmagent.WithModel(openai.New(*modelName)),
		llmagent.WithDescription(
			"A concise assistant used by the global AfterRun hook example.",
		),
		llmagent.WithInstruction(
			"Answer accurately and concisely. Follow the requested output length.",
		),
		llmagent.WithGenerationConfig(model.GenerationConfig{Stream: false}),
	)
	r := runner.NewRunner(
		"global-after-run-example",
		agt,
		runner.WithSessionService(inmemory.NewSessionService()),
	)
	defer func() {
		if err := r.Close(); err != nil {
			log.Printf("close runner: %v", err)
		}
	}()

	events, err := r.Run(
		ctx,
		"example-user",
		fmt.Sprintf("session-%d", time.Now().UnixNano()),
		model.NewUserMessage(*prompt),
		// ExecutionTrace remains opt-in even when a global hook is registered.
		agent.WithExecutionTraceEnabled(true),
	)
	if err != nil {
		return fmt.Errorf("start Runner run: %w", err)
	}

	answer, err := collectAnswer(events)
	if err != nil {
		return err
	}
	fmt.Printf("Assistant: %s\n\n", answer)

	// Draining the event channel guarantees that the AfterRun hook has run.
	select {
	case observation := <-observations:
		printObservation(observation)
		return nil
	default:
		return fmt.Errorf("global AfterRun hook did not produce an observation")
	}
}

func collectAnswer(events <-chan *event.Event) (string, error) {
	var answer strings.Builder
	for evt := range events {
		if evt == nil || evt.IsRunnerCompletion() {
			continue
		}
		if evt.Error != nil {
			return "", fmt.Errorf("agent event: %s", evt.Error.Message)
		}
		if evt.Response == nil || len(evt.Response.Choices) == 0 {
			continue
		}
		choice := evt.Response.Choices[0]
		if choice.Message.Role == model.RoleAssistant {
			answer.WriteString(choice.Message.Content)
		}
	}
	return strings.TrimSpace(answer.String()), nil
}

type runObservation struct {
	spanContext oteltrace.SpanContext
	trace       *agenttrace.Trace
	problem     string
}

func observeCompletedRun(
	observations chan<- runObservation,
) plugin.AfterRunHook {
	return func(ctx context.Context, args *plugin.AfterRunArgs) error {
		observation := runObservation{
			spanContext: oteltrace.SpanContextFromContext(ctx),
		}
		if args == nil || args.CompletionEvent == nil {
			observation.problem = "completion event is unavailable"
		} else {
			observation.trace = args.CompletionEvent.ExecutionTrace
			if observation.trace == nil {
				observation.problem = "execution trace is unavailable"
			}
		}

		// A production integration should enqueue without blocking Runner
		// completion. This example performs one run and uses a one-slot queue.
		select {
		case observations <- observation:
			return nil
		default:
			return fmt.Errorf("observation queue is full")
		}
	}
}

func printObservation(observation runObservation) {
	fmt.Println("Global AfterRun observation")
	fmt.Println(strings.Repeat("=", 72))
	fmt.Printf("SpanContext valid: %t\n", observation.spanContext.IsValid())
	if observation.spanContext.IsValid() {
		fmt.Printf("Trace ID: %s\n", observation.spanContext.TraceID())
		fmt.Printf("Root invoke_agent Span ID: %s\n", observation.spanContext.SpanID())
	}
	if observation.problem != "" {
		fmt.Printf("Problem: %s\n", observation.problem)
		return
	}

	executionTrace := observation.trace
	fmt.Printf("Root agent: %s\n", executionTrace.RootAgentName)
	fmt.Printf("Invocation ID: %s\n", executionTrace.RootInvocationID)
	fmt.Printf("Session ID: %s\n", executionTrace.SessionID)
	fmt.Printf("Status: %s\n", executionTrace.Status)
	fmt.Printf("Duration: %s\n", executionTrace.EndedAt.Sub(executionTrace.StartedAt))
	fmt.Printf("Steps: %d\n", len(executionTrace.Steps))
	if executionTrace.Usage != nil {
		fmt.Printf("Total tokens: %d\n", executionTrace.Usage.TotalTokens)
	}
	if executionTrace.Input != nil {
		fmt.Printf("Input snapshot: %s\n", executionTrace.Input.Text)
	}
	if executionTrace.Output != nil {
		fmt.Printf("Output snapshot: %s\n", executionTrace.Output.Text)
	}
}

func envOrDefault(name, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(name)); value != "" {
		return value
	}
	return fallback
}
