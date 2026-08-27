# Global Runner AfterRun Hook Example

This example runs a real `llmagent.LLMAgent` against an OpenAI-compatible model
and observes the completed run through `runner.RegisterGlobalAfterRunHook`.

It demonstrates the framework-side contract intended for process-wide
observability integrations:

- registration is independent of a particular `Runner` instance;
- `ExecutionTrace` is enabled explicitly for the run;
- the hook receives the finalized execution trace;
- the hook context carries the root `invoke_agent` `SpanContext`; and
- the hook uses a non-blocking queue so observability work does not delay Runner
  completion.

The example prints the hook payload locally. A real observability integration
would convert the same data into its own log or event representation and export
it outside the hook's critical path.

## Prerequisites

- Go 1.24 or later, as required by the examples module
- an OpenAI-compatible model endpoint and API key
- an OTLP HTTP collector listening on `localhost:4318`, or another endpoint
  supplied with `-otel-endpoint`

The collector stack in
[`../jaeger-prometheus`](../jaeger-prometheus/README.md) can be used locally:

```bash
cd examples/telemetry/jaeger-prometheus
docker compose up -d
```

## Run

From the repository root:

```bash
export OPENAI_API_KEY="your-api-key"
export OPENAI_BASE_URL="https://api.openai.com/v1"
export MODEL_NAME="gpt-4o-mini"

cd examples
go run ./telemetry/global-after-run
```

For another OTLP HTTP collector or prompt:

```bash
go run ./telemetry/global-after-run \
  -otel-endpoint localhost:4318 \
  -model gpt-4o-mini \
  -prompt "Describe the lifecycle of an AI agent run in two sentences."
```

`-otel-endpoint` accepts `host:port` without a URL scheme. The model client
reads `OPENAI_API_KEY` and `OPENAI_BASE_URL` from the environment.

## Expected output

The exact response and identifiers vary by run. The important fields are:

```text
Assistant: ...

Global AfterRun observation
========================================================================
SpanContext valid: true
Trace ID: ...
Root invoke_agent Span ID: ...
Root agent: execution-trace-assistant
Status: completed
Steps: 1
Total tokens: ...
Input snapshot: ...
Output snapshot: ...
```

Removing `agent.WithExecutionTraceEnabled(true)` leaves `ExecutionTrace` nil;
registering the global hook does not enable execution tracing automatically.
