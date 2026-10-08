## Context

See `proposal.md` — *Why*. Current state relevant to the approach (verified in `team-ai`):

- `ModelRouter` (`app/modules/ai/llm/router.py`) holds one breaker per role, only for the primary;
  `current_target` returns the secondary (first `CHAT_FALLBACK_MODELS` entry) while that breaker is open.
  `record_success`/`record_error` exist but nothing calls them. `CircuitBreaker` (`app/core/resilience.py`)
  has `is_open` and a failure count but no clock, so no cooldown or half-open state; `_counts_as_failure`
  returns False for `status_code=None`.
- `LLMRouterChatStreamer` (`app/transport/grpc/chat_stream.py`) wraps a frozen `LLMInstance` whose
  `chat_model` was built once by `build_llm_instance`. `ChatServicer.StreamChat` forwards `message` and
  `session_id` only and never reads `context.time_remaining()`.
- `RetryPolicy`/`TimeoutPolicy` exist (`resilience.py`) but are used only by outbox/webhook/eval.
  `QuotaService.reserve/finalize/refund` exist with no call site. gRPC interceptors are Tracing + Auth only.
  `InMemoryRateLimiter` and `RedisRateLimiter` exist; `RATE_LIMIT_BACKEND` defaults to `memory`.
  `Settings.ENVIRONMENT` (`is_production`) exists.
- `RedactionPolicy` handles email, `sk-`, `Bearer` and is applied only to RAG docs. `AIAssistantService`
  logs raw `message` / `buyer_message`. `LangfuseLLMTracker` has `trace_config` and `get_prompt` unused by chat.
- `make eval` runs `CompletionPipeline(EchoCompletionHandler())` over 7 cases; judge cases are dropped from
  the denominator when `JUDGE_CHAT_MODEL` is empty.
- Constraints: Rule 4 (proto in platform-core; we avoid any change), ADR-0011 (capacity-driven routing and
  vLLM belong to `platform-modelserve`, out of scope), repo convention that factories dispatch backends.

## Goals / Non-Goals

**Goals:**

- A single provider failure before the first token is invisible to the buyer; after the first token it is a
  clean error, never a mixed-model reply.
- The router's health view reflects real call outcomes and recovers on its own.
- LLM cost and abuse are bounded per principal; PII does not reach the model or logs.
- The behaviour is proven by a gate that cannot pass vacuously.

**Non-Goals (design-level):** no persistence of breaker state across restarts or replicas (per-process is
acceptable; it only affects how fast each replica learns), no streaming resume, no provider-specific SDK
logic beyond extracting an HTTP status and `usage_metadata`.

## Decisions

### D1 — A `ResilientChatModel` attempt loop owns target selection, inside the streamer

`LLMRouterChatStreamer` stops holding a frozen model. Per request it asks the router for an ordered list of
*eligible* targets (breaker closed or half-open-probe-permitted), builds/reuses a cached model per target
(`model_builder(target)` memoised by target string), and runs the attempt loop from D3. The router stays the
single owner of breaker state; the streamer only reports outcomes.
*Alternatives:* rebuild `LLMInstance` on breaker change (racy, and per-request state is still needed);
LangChain `.with_fallbacks()` (fallback after partial output is possible, cannot express first-chunk rule,
hides outcomes from our breaker). Rejected.

### D2 — Router becomes chain-aware with per-target breakers and a clocked half-open state

`fallback_models(role)` returns `[CHAT_MODEL, *CHAT_FALLBACK_MODELS]` in order. One breaker per
`(role, target)`. `CircuitBreaker` gains an injectable clock, `cooldown_seconds`, and states
closed/open/half-open: after cooldown exactly one caller is admitted as the probe (others still skipped);
probe success closes, failure re-opens with a fresh cooldown. Failure classification moves to a pure function
`classify_failure(exc) -> FailureKind` (`rate_limited`, `server_error`, `timeout`, `connection`, `client_error`,
`unknown`); the first four count. `status_code=None` with a timeout/connection exception now counts; other
4xx stay uncounted. `current_target` is kept for compatibility and returns the first eligible target.
*Alternative:* keep `failure_status_range=range(400,500)` and only add 5xx. Rejected: it keeps counting
request-caused 400/404 and still misses transport failures.

### D3 — Fallback is decided by a `first_chunk_emitted` flag

```
for target in eligible_targets:
    for attempt in retry_policy attempts (pre-chunk only):
        try:  first = await wait_for(anext(stream), first_token_timeout ∧ remaining_deadline)
        except retryable: record_error; continue/next target
        emit first; set first_chunk_emitted; stream rest
        on later error: record_error; raise StreamInterrupted   # no fallback
        record_success; return
raise ChainExhausted
```

The servicer maps `ChainExhausted` → `UNAVAILABLE` (or `RESOURCE_EXHAUSTED` if the last failure was 429),
deadline → `DEADLINE_EXCEEDED`, `StreamInterrupted` → `UNAVAILABLE` after the partial deltas.
*Alternative:* buffer the whole reply and fall back at any time. Rejected: kills token streaming, the point
of the `ai-assistant` streaming requirement.

### D4 — Timeouts: first-token timeout plus deadline cap, reuse existing policies

New settings `CHAT_FIRST_TOKEN_TIMEOUT_SECONDS` (default 8) and `CHAT_MAX_ATTEMPTS` (default 2 per target).
Effective timeout = `min(first_token_timeout, context.time_remaining())`; the servicer passes the remaining
deadline to the streamer as `deadline_seconds`. `RetryPolicy` supplies the backoff and is constructed with
`max_attempts=CHAT_MAX_ATTEMPTS`. No inter-chunk idle timeout in this change (open question).
*Alternative:* rely on the HTTP client's read timeout in the provider SDK. Rejected: differs per provider and
cannot bound time-to-first-token across a fallback chain.

### D5 — Usage and quota are an optional decorator at the servicer/streamer seam

The streamer returns a `ChatResult`-style side channel (final `usage` populated from the last chunk's
`usage_metadata`) so the gRPC layer can finalize quota; when the provider omits usage, finalize uses an
estimate from character count and flags `estimated=true`. `QuotaService.reserve(resource="llm.chat.tokens",
cost=estimated_input+max_output)` runs before the loop when `QUOTA_ENABLED`; success → `finalize`, no chunk
delivered → `refund`; partial delivery counts as used. Idempotency key = request id.
*Alternative:* meter only after the fact from Langfuse. Rejected: cannot refuse an over-quota call.

### D6 — gRPC rate limit as an interceptor using the existing limiter factory

A `RateLimitInterceptor` keyed by the principal the `AuthInterceptor` already resolves, applied to the
`StreamChat` and `ShoppingAssistant` methods, built from `app/modules/platform/rate_limit/factory.py`. It
aborts with `RESOURCE_EXHAUSTED` and puts `retry-after` in trailing metadata. A settings validator rejects
`RATE_LIMIT_BACKEND=memory` when `ENVIRONMENT.is_production` (fail-fast at startup, mirroring the existing
auth-token validator) and requires `REDIS_ENABLED` for `redis`.
*Alternative:* limit only at the gateway. Rejected as sole control: team-ai is also reachable by other
internal callers, and the gateway limit cannot see token cost; the gateway limit stays as defence in depth.

### D7 — System prompt and session history behind small ports

`PromptProvider` returns the system prompt: Langfuse `get_prompt("team-ai/chat-system")` (cached by the
tracker's `prompt_cache_ttl_seconds`) with a static fallback string in code on any error. `SessionStore` port
with Redis (key `chat:session:{principal}:{session_id}`, `EXPIRE` = `CHAT_SESSION_TTL_SECONDS`, LTRIM to
`CHAT_SESSION_MAX_TURNS`) and in-memory (dev/test) implementations; history is additionally trimmed to
`CHAT_HISTORY_MAX_CHARS` dropping oldest turns first. Turns are appended only after a fully successful reply
(spec: failed reply not stored). History is scoped by principal as well as `session_id` so one user cannot
read another's session by guessing an id. Stored text is the *redacted* text.
*Alternative:* let the client resend history. Rejected: proto has only `session_id`, and client-held history
invites prompt injection through fabricated assistant turns.

### D8 — Redaction: extend the policy, apply at three choke points

Add patterns to `RedactionPolicy.redact_text`: VN mobile (`(?:\+?84|0)(?:3|5|7|8|9)\d{8}` with optional
spaces/dots/dashes) and CCCD/CMND (`\b\d{12}\b|\b\d{9}\b`, applied after the phone pattern so phones win).
Apply at: (1) the streamer before building messages and before storing history, (2) every
`AIAssistantService` log call that includes user text, (3) the Langfuse-bound input through the same function.
Order numbers and prices are not matched because the patterns are length-anchored with word boundaries; false
positives on 9-digit values are accepted (over-redaction is the safe failure).
*Alternative:* an NER/PII service. Rejected: new dependency and latency for a regex-solvable Vietnamese
format set.

### D9 — Langfuse trace config attached per attempt

`LLMInstance.trace_config(LLMTraceContext(session_id, user_id, request_id, tags=("grpc","chat"),
metadata={"target": target, "attempt": n}))` is passed as `config=` to `astream`. Disabled tracker returns no
callbacks, so behaviour is identical (spec).

### D10 — Eval: real streamer + fake chat models, and a non-vacuous gate

Add `evals/chat_resilience.jsonl` whose target builds `LLMRouterChatStreamer` over scripted fake chat models
(a test-support `ScriptedChatModel` that yields chunks or raises 429/5xx/timeout at a scripted position).
`scripts/run_eval.py` changes: an unscored case (evaluator `None`) is counted as a failure and listed; exit
non-zero when any case requiring a judge has no judge; `--allow-unjudged` flag opt-out exists for local dev
only and is not used by `make eval`. Judge cases in the smoke set stay and will now demand
`JUDGE_CHAT_MODEL` in CI, or are moved to a separate `make eval-judge` target if CI cannot supply one (see
Open Questions; the spec only requires that the gate is not vacuous).
*Alternative:* delete the judge cases. Rejected: hides, rather than fixes, missing coverage.

## Risks / Trade-offs

- [Per-process breakers: replicas learn health independently] → acceptable (each replica converges within
  `threshold` failures); shared breaker state is a later change.
- [Retry + fallback multiplies latency and provider load during an incident] → attempts bounded per target,
  deadline caps total, open breakers skip dead targets, backoff from `RetryPolicy`.
- [Half-open probe carries real user traffic] → probe only on pre-chunk phase with fallback behind it, so a
  failed probe costs one attempt, not an error.
- [Status extraction differs by provider SDK (httpx/openai/anthropic exceptions)] → single
  `classify_failure` covering `status_code`/`response.status_code`/`httpx.TimeoutException`/`ConnectError`,
  table-tested; unknown → `unknown` counted as failure for exceptions, not for cancellation.
- [Regex redaction over-masks 9-12 digit values] → documented; output side not redacted (non-goal).
- [Session history in Redis stores user text] → stored redacted, TTL-bounded, principal-scoped.
- [Fail-fast prod validator breaks a deploy that relied on memory limiter] → call out in Migration Plan;
  set `RATE_LIMIT_BACKEND=redis` in gitops values first.
- [No proto change means clients get a generic `UNAVAILABLE` with message text only] → accepted; if a
  machine-readable retry hint is needed it is an additive proto PR, flagged, not assumed.

## Migration Plan

1. Land router/breaker/classification and the attempt loop with `CHAT_BACKEND=mock` default unchanged; all
   new behaviour is behind the `llm_router` backend, so the default path is untouched.
2. Ship settings with safe defaults (`QUOTA_ENABLED=false`, rate limit on but generous, history on with small
   TTL). Switch gitops production values to `RATE_LIMIT_BACKEND=redis` *before* the release containing the
   production validator.
3. Enable quota per environment after a week of usage metrics.
4. Rollback: revert the image; no data migration. Session keys expire by TTL; nothing persistent is added.

## Open Questions

- Whether CI can supply `JUDGE_CHAT_MODEL`; if not, judge cases move to a separate `make eval-judge` target
  while `make eval` stays deterministic (does not change specs, only which target carries them).
- Whether to add an inter-chunk idle timeout after first-token timeout proves insufficient in production.
