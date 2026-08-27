//
// Tencent is pleased to support the open source community by making trpc-agent-go available.
//
// Copyright (C) 2025 Tencent.  All rights reserved.
//
// trpc-agent-go is licensed under the Apache License Version 2.0.
//

// Package trace provides invocation-aware tracing helpers.
package trace

import (
	"context"
	"strings"

	"go.opentelemetry.io/otel/attribute"
	oteltrace "go.opentelemetry.io/otel/trace"
	"go.opentelemetry.io/otel/trace/noop"

	"trpc.group/trpc-go/trpc-agent-go/agent"
	semconvtrace "trpc.group/trpc-go/trpc-agent-go/telemetry/semconv/trace"
	telemetrytrace "trpc.group/trpc-go/trpc-agent-go/telemetry/trace"
)

// StartSpan returns a no-op span when tracing is disabled for the invocation.
func StartSpan(ctx context.Context, invocation *agent.Invocation, spanName string) (context.Context, oteltrace.Span, bool) {
	if invocation != nil && invocation.RunOptions.DisableTracing {
		return ctx, noop.Span{}, false
	}
	ctx, span := telemetrytrace.Tracer.Start(ctx, spanName)
	routingInvocation := invocation
	if routingInvocation == nil {
		routingInvocation, _ = agent.InvocationFromContext(ctx)
	}
	if appName := invocationAppName(routingInvocation); appName != "" {
		span.SetAttributes(attribute.String(
			semconvtrace.KeyTRPCAgentGoAppName,
			appName,
		))
	}
	return ctx, span, true
}

func invocationAppName(invocation *agent.Invocation) string {
	if invocation == nil {
		return ""
	}
	if appName := strings.TrimSpace(invocation.RunOptions.AppName); appName != "" {
		return appName
	}
	if invocation.Session == nil {
		return ""
	}
	return strings.TrimSpace(invocation.Session.AppName)
}
