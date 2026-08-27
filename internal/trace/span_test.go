//
// Tencent is pleased to support the open source community by making this
// software available.
//
// Copyright (C) 2025 Tencent.  All rights reserved.
//
// trpc-agent-go is licensed under the Apache License Version 2.0.
//

package trace

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/attribute"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"trpc.group/trpc-go/trpc-agent-go/agent"
	"trpc.group/trpc-go/trpc-agent-go/session"
	semconvtrace "trpc.group/trpc-go/trpc-agent-go/telemetry/semconv/trace"
	telemetrytrace "trpc.group/trpc-go/trpc-agent-go/telemetry/trace"
)

func TestStartSpanSetsInvocationAppName(t *testing.T) {
	tests := []struct {
		name       string
		invocation *agent.Invocation
		fromCtx    bool
		want       string
	}{
		{
			name: "run option",
			invocation: &agent.Invocation{
				RunOptions: agent.RunOptions{AppName: " app-override "},
				Session:    &session.Session{AppName: "session-app"},
			},
			want: "app-override",
		},
		{
			name: "session fallback",
			invocation: &agent.Invocation{
				Session: &session.Session{AppName: " session-app "},
			},
			want: "session-app",
		},
		{
			name: "context invocation",
			invocation: &agent.Invocation{
				Session: &session.Session{AppName: "context-app"},
			},
			fromCtx: true,
			want:    "context-app",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			recorder := tracetest.NewSpanRecorder()
			provider := sdktrace.NewTracerProvider(
				sdktrace.WithSpanProcessor(recorder),
			)
			previous := telemetrytrace.Tracer
			telemetrytrace.Tracer = provider.Tracer("test")
			t.Cleanup(func() {
				telemetrytrace.Tracer = previous
				require.NoError(t, provider.Shutdown(context.Background()))
			})

			ctx := context.Background()
			invocation := test.invocation
			if test.fromCtx {
				ctx = agent.NewInvocationContext(ctx, invocation)
				invocation = nil
			}
			_, span, started := StartSpan(ctx, invocation, "test")
			require.True(t, started)
			span.End()

			ended := recorder.Ended()
			require.Len(t, ended, 1)
			assert.Equal(t, test.want, stringAttribute(
				ended[0].Attributes(),
				semconvtrace.KeyTRPCAgentGoAppName,
			))
		})
	}
}

func stringAttribute(attrs []attribute.KeyValue, key string) string {
	for _, attr := range attrs {
		if string(attr.Key) == key && attr.Value.Type() == attribute.STRING {
			return attr.Value.AsString()
		}
	}
	return ""
}
