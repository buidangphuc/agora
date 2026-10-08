## Why

On agora's `team-ai`, the real-LLM chat path (`CHAT_BACKEND=llm_router`) cannot survive one provider failure. The audit
of 2026-10-08 found:
- `ModelRouter.record_success`/`record_error` have no callers.
- The chat model is built once at startup.
- `LLMRouterChatStreamer` calls `astream` with no error handling, timeout or retry.
- The breaker counts only 4xx and never recovers.
- Quota has no call sites, and the reservation id is client-chosen.
- The prompt is one bare `HumanMessage` with no system prompt or history.
- User text reaches the model and the logs unredacted, with no Vietnamese phone or ID masking.
- `StreamChat` errors leak exception text.
- The eval gate silently skips judge cases.

The carried-over change `llm-path-resilience` specified this against the retired checkout. Its code never came to
agora, and it had no end-to-end coverage, because no local provider could inject failures. This change ports the
behaviour to agora's code. It makes the behaviour verifiable through the public edge with a fault-injecting fake
provider in the e2e stack.

## What Changes

- **team-ai, chat path** (`StreamChat` with `llm_router`):
  - Pick a target per request, and report every attempt's outcome to the router.
  - Use an ordered fallback chain, applied only before the first streamed chunk.
  - Give each target its own breaker. It counts 429, 5xx, timeouts and connection errors, and recovers through one
    half-open probe after a cooldown.
  - Bound each attempt by a first-token timeout and a bounded pre-chunk retry. The whole call is bounded by the
    caller's gRPC deadline.
  - Map errors to gRPC codes with fixed messages (`UNAVAILABLE`, `RESOURCE_EXHAUSTED`, `DEADLINE_EXCEEDED`), never
    with exception text.
- **team-ai, usage and quota:**
  - Capture provider token usage.
  - When `QUOTA_ENABLED`, reserve quota under a server-minted id before the call, finalize it with the actual usage,
    and refund it when nothing was delivered.
- **team-ai, rate limit and prompt:**
  - The per-principal gRPC rate limit covers `StreamChat` and `ShoppingAssistant`. Production refuses a per-process
    limiter.
  - Every request starts with a system prompt (from Langfuse, or a static fallback).
  - Prior turns of the same `session_id` are included, bounded and TTL'd, in Redis when enabled.
- **team-ai, privacy:**
  - PII is redacted in the text sent to the model, always: email, keys, bearer tokens, Vietnamese mobile numbers, and
    CMND/CCCD after an ID keyword.
  - User text in logs and traces follows `LLM_TRACE_CONTENT` (`redacted` by default).
  - Chat attempts carry the Langfuse trace config (session, principal and request ids) when Langfuse is on.
- **team-ai, eval:** a chat resilience eval set runs against the real streamer with fake models. The gate fails when a
  judge case has no judge.
- **platform-e2e:** a fault-injecting, OpenAI-compatible fake provider (`llm-fake`). A compose overlay runs team-ai
  with `CHAT_BACKEND=llm_router` against it. Scenarios exercise the behaviour through the gateway.

Repos touched: team-ai, platform-e2e. **No proto change.** Root compose is unchanged; the overlay lives in
`platform-e2e/compose/`.

## Capabilities

### New Capabilities
- `llm-inference-resilience`: how team-ai selects, protects, meters, prompts, redacts and observes calls to the
  external LLM on the chat path, and the eval gate that proves it.

### Modified Capabilities
- None. `ai-assistant` (routing via the gateway, token streaming, mock default) is unchanged. The local default stays
  `CHAT_BACKEND=mock`, and only the e2e overlay switches to `llm_router`.

## Non-goals

- `ai:use` enforcement (`AI_USE_SCOPE_REQUIRED` stays off, the user's decision of 2026-10-08).
- Recommendation serving safeguards: cold start, payload `listing_id`, `model_version` and memory-backend refusal.
  These belong to the AI-first wave (`recs-serving-safeguards`).
- Making `MagicListing`/`ChatCopilot` LLM-driven. Their logs are redacted and nothing more.
- Response caching, output moderation, capacity-based routing (`platform-modelserve`, ADR-0011).
- A shared-across-replicas rate-limit e2e. The Redis backend is unit-tested; the e2e stack runs one replica.

## Relationship to the carried-over changes

This change replaces `llm-path-resilience` and the team-ai requirements of `gateway-and-ai-hardening`, except for the
recommendation items listed above. Both carried-over changes are retired when this change is archived.

## Impact

- **team-ai settings:**
  - New: `LLM_FIRST_TOKEN_TIMEOUT_SECONDS`, `LLM_MAX_ATTEMPTS`, `LLM_BREAKER_THRESHOLD`,
    `LLM_BREAKER_COOLDOWN_SECONDS`, `CHAT_SYSTEM_PROMPT`, `CHAT_HISTORY_MAX_TURNS`,
    `CHAT_HISTORY_MAX_TOKENS`, `CHAT_HISTORY_TTL_SECONDS`, `LLM_TRACE_CONTENT`.
  - Existing settings that become live: `QUOTA_*`, `GRPC_RATE_LIMIT_*`.
- **Production:**
  - The Redis rate-limit backend is required when the gRPC rate limit is on.
  - Redis is recommended for chat history; without it, history is per process.
- **e2e stack:** a new `llm-fake` container, plus an overlay that switches team-ai to it. The existing assistant
  scenarios must stay green on the overlay.
