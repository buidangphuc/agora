# ADR-0004 — Observability (OpenTelemetry, exporter-swappable)

**Status:** Accepted · **Date:** 2026-08-30

## Context

The current product repo has **no distributed tracing** — only an in-process
`X-Request-ID` correlation id (`app/core/request_context.py`), not propagated across
services, plus Langfuse for LLM calls. A polyrepo needs cross-service tracing so a
request through Frontend → Gateway → N services can be followed.

Datadog was considered (native `dd-trace` + LLM Observability, and a Datadog MCP is
already connected). It is excellent but is SaaS: it needs an Agent + API key and phones
home, which conflicts with the local-only, cost-conscious posture of these new projects.

## Decision

- **Instrument with OpenTelemetry** (SDK + gRPC interceptors) in every language.
- **Propagate W3C `traceparent`** on every gRPC hop, and **bridge `x-request-id`**
  (keep the human-facing correlation id from the existing convention).
- **Local export is free and vendor-neutral**: an `otel-collector` (opt-in compose
  profile, OFF by default) prints traces via the debug exporter. `make dev` skips it;
  `make dev-obs` starts it.
- **No vendor lock-in**: shipping to Datadog / Grafana Tempo / Jaeger later is an
  **exporter/collector config change only** — service code does not change. This mirrors
  the platform's "swap infra by changing config, not code" principle.

## Consequences

- Free and simple locally; upgrade path to any backend (including Datadog) is config.
- Each service template ships a **trace interceptor stub** + OTLP exporter wiring.
- LLM-specific tracing (Langfuse today) is revisited when `team-ai` is built; it can
  coexist with OTel or be replaced by an OTel-native LLM semconv exporter.
