## Why

Verified by reading `team-ai`, the real-LLM chat path (`CHAT_BACKEND=llm_router`) has a router that looks
resilient but never receives feedback. `ModelRouter.record_success` / `record_error` have no caller outside
tests, `LLMRouterChatStreamer.astream` calls `chat_model.astream` with no `try/except`, and the chat model is
built once at startup into a frozen `LLMInstance`, so even a recorded error could never switch to the fallback
model. The breaker only counts HTTP 400-499 and ignores `status_code=None`, so timeouts, connection errors and
5xx never count; only the first `CHAT_FALLBACK_MODELS` entry is used; and an open breaker never recovers. There
is no timeout or retry on LLM calls, no token/cost accounting (`QuotaService` has no call sites), no gRPC rate
limit, and the prompt is a single bare `HumanMessage` (no system prompt, no session history) whose raw text is
logged. `make eval` exercises only the echo completion handler, so none of this is tested or gated. One provider
429 or hang today surfaces as a broken stream to every buyer.

## What Changes

All changes are in `team-ai`; no proto change is planned.

- **Per-request routing with outcome feedback**: the streamer resolves a target per request through
  `ModelRouter`, builds/reuses the model for that target, and records success or error for every attempt.
- **Fallback only before the first chunk**: an ordered chain (`CHAT_MODEL` then all `CHAT_FALLBACK_MODELS`);
  once a chunk has been emitted a failure ends the stream with an error, never a model switch mid-reply.
- **Breaker semantics**: 429, 5xx, timeouts and connection errors count as failures; open state has a
  cooldown and a half-open single probe; breakers exist per target, not only for the primary.
- **Timeouts and bounded retry**: first-token timeout, bounded retry before the first chunk using the existing
  `RetryPolicy`/`TimeoutPolicy`, and the gRPC deadline (`context.time_remaining()`) caps everything.
- **Usage, quota and rate limit**: capture `usage_metadata`; `QuotaService` reserve/finalize/refund around a
  call when `QUOTA_ENABLED`; gRPC `StreamChat`/`ShoppingAssistant` rate-limited per principal; a production
  config must use the Redis rate-limit backend.
- **Prompt and privacy**: system prompt (Langfuse prompt with static fallback), bounded TTL'd session history,
  `RedactionPolicy` extended with VN phone/national-ID patterns and applied to LLM input and to log lines
  (including the raw-text logs in `AIAssistantService`), Langfuse `trace_config` attached.
- **Eval**: a chat eval set run against the real `LLMRouterChatStreamer` with fake chat models (429/5xx/
  timeout/stream-break); the eval gate fails, not silently skips, when a judge case is present and no judge
  is configured.
- Additive error mapping only: exhausted chain maps to existing gRPC codes (`UNAVAILABLE`,
  `RESOURCE_EXHAUSTED`, `DEADLINE_EXCEEDED`); no new RPC or message.

## Capabilities

### New Capabilities
- `llm-inference-resilience`: how team-ai selects, protects, meters and observes calls to the external LLM
  on the chat streaming path — routing/fallback, breaker, timeouts/retry, usage and quota, rate limiting,
  prompt/session/redaction, and the eval gate that proves it.

### Modified Capabilities
<!-- ai-assistant's requirements (route via gateway, token-by-token streaming, mock default) are unchanged;
     this change only hardens the behaviour behind them, so no delta there. recommendations is unaffected. -->

## Impact

- Repo: `team-ai` only (`app/modules/ai/llm/router.py`, `app/transport/grpc/chat_stream.py`,
  `app/transport/grpc/servicers/chat.py`, `app/transport/grpc/server.py`, `app/core/resilience.py`,
  `app/core/redaction.py`, `app/modules/business/ai_assistant/service.py`, `scripts/run_eval.py`, `evals/`,
  config in `app/core/config/`). `FEATURES.yaml` entries for the new scenarios.
- Contract: none. If a client-visible retry hint proves necessary it must be additive-only and is flagged for
  a platform-core PR (AGENTS.md Rule 4); current plan needs none.
- Data/infra: Redis required for shared rate limit and session history in prod; Langfuse optional.
- Architecture rules: Rule 4 (proto untouched), ADR-0011 (this is the reactive-failover layer the ADR
  notes; capacity-driven routing stays with `platform-modelserve`).

## Non-goals

- No LLM response cache, no output guardrails/moderation, no AI gateway product.
- No vLLM / `platform-modelserve` work (ADR-0011), no capacity-driven routing.
- `AIAssistantService` (unary ShoppingAssistant/MagicListing/ChatCopilot) does not become LLM-driven; only
  its logging is redacted and its RPCs are rate-limited.
- No proto change unless strictly needed, and then additive only.
