## Context

See proposal.md for the motivation. Current agora `team-ai` code, as of 2026-10-08:
- `app/modules/ai/llm/router.py` has `ModelRouter`. It has one breaker per role. It counts only 4xx failures and
  ignores `status_code=None`, so timeouts never count, and there is no half-open state.
- `app/modules/ai/llm/runtime.py` builds the chat model once, through `build_llm_instance`.
- `app/modules/chat/chat_stream.py` has `LLMRouterChatStreamer`. It sends one `HumanMessage` to the frozen model and
  has no error handling.
- `app/transport/grpc/servicers/chat.py` (`StreamChat`) is not wrapped by the error mapper in `grpc/errors.py`.
- Quota: `app/modules/platform/quota/service.py` (`QuotaService.reserve/finalize/refund`) has memory, Mongo and
  Postgres stores. Nothing calls it.
- Rate limiting: the gRPC rate limiter is opt-in (`GRPC_RATE_LIMIT_ENABLED`, defined in `config/transport.py`). Its
  default backend is memory.
- Redaction: `app/core/redaction.py` has `RedactionPolicy` with off/redacted/full modes and masks email, `sk-` keys and
  bearer tokens. Nothing calls it on the chat path.
- Model construction: models come from `init_chat_model("<provider>:<name>")` (`langchain-openai` is installed), so an
  `openai:` target honours `OPENAI_BASE_URL`.
- The old implementation lives in the retired team-ai bundle (`~/Documents/agora-archive/full_team_repo/bundles`). It
  is a reference only, because agora's module layout differs. The relevant commits are: 3923657 family (streamer),
  c973f53 (router), f446d18/dceb6cc/f212b4b/cccf118/f83672b (session, prompt, usage, trace content, redaction),
  8cd024a (eval), and 85d0e59/3f14e85/e32c295 (quota).

## Goals / Non-Goals

**Goals:**
- Port the behaviour by spec onto agora's modules.
- Make every scenario observable through the gateway, using a fake provider that can be told per request how to fail.

**Non-Goals:**
- Changing `CHAT_BACKEND`'s local default (`mock`).
- Making the unary assistant RPCs LLM-driven.
- Recommendation serving.

## Decisions

### D1. Streamer: an attempt loop over the chain
- Each request runs `for target in router.eligible_targets("chat")`. Each attempt runs inside
  `asyncio.timeout(min(first_token_timeout, deadline_left))` until the first chunk arrives, then inside the remaining
  deadline only.
- Models are memoised per target, built with `init_chat_model`.
- Failures are classified into `FailureKind` values: `RATE_LIMITED`, `SERVER`, `TIMEOUT`, `CONNECTION`, `CLIENT`. The
  classifier reads the openai exception's `status_code`, `httpx` errors and `asyncio.TimeoutError`.
- `record_error(target, kind)` and `record_success(target)` are called for every attempt.
- After the first chunk is yielded, any exception goes to `record_error` and re-raises as a terminal stream error.
- `LLM_MAX_ATTEMPTS` counts all pre-chunk attempts across the chain. A retry of the same target is used only when the
  chain has a single target.
- Alternative considered: `langchain`'s `with_fallbacks`. Rejected, because it can switch models mid-stream, cannot
  report outcomes to our breaker, and hides the first-chunk boundary.

### D2. Breaker per target
- The state machine is closed → open → half-open.
- Only `RATE_LIMITED`, `SERVER`, `TIMEOUT` and `CONNECTION` increment the counter.
- In half-open, a single in-flight probe is guarded by an `asyncio.Lock` try-acquire. Concurrent requests skip the
  target while a probe is running.
- The clock is injectable for unit tests.

### D3. Error mapping
- `StreamChat` errors are mapped through `grpc/errors.py` with fixed messages:
  - "model unavailable"
  - "model rate limited"
  - "deadline exceeded"
- The original exception is logged with the request id and never sent to the client.

### D4. Quota unit is one reply
- Resource `chat.reply`, with cost 1 per reply. The policy limit comes from `QUOTA_CHAT_REPLIES_PER_WINDOW` and
  `QUOTA_CHAT_WINDOW_SECONDS`.
- The reservation id (`idempotency_key`) is minted server-side as `uuid4`, never taken from the request.
- On completion, `finalize` is called and the token usage is logged. On a failure before any chunk, `refund` is
  called. On a failure after a chunk, the reply is finalized, because the user received output.
- Token-denominated quota was considered. It is deferred, because the cost of a reply is unknown at reserve time, which
  makes "exhausted" non-deterministic.

### D5. Session history
- Interface: `ChatSessionStore`, with `get(session_id)` and `append(session_id, user, assistant)`.
- Backends: Redis (`chat:session:<id>`, a list with a TTL) when `REDIS_ENABLED`, otherwise in-process LRU with a TTL.
- Turns are appended only after a completed reply.
- Trimming happens in two steps: first by `CHAT_HISTORY_MAX_TURNS`, then by a token estimate of characters/4 against
  `CHAT_HISTORY_MAX_TOKENS`.

### D6. Redaction
- `RedactionPolicy.for_llm_input(text)` always applies the "redacted" masks plus two Vietnamese patterns:
  - mobile numbers: `(?:\+84|0)(?:[\s.]?\d){9}`;
  - ID numbers: `(?i)\b(?:cmnd|cccd|căn cước)\b[^\d]{0,12}(\d{9}|\d{12})`, which masks only the number.
- Log and trace text uses `RedactionPolicy.from_trace_content(LLM_TRACE_CONTENT)`.
- Config validation refuses `full` in strict ENV.
- `AIAssistantService` log calls are routed through the log policy.

### D7. Tracing
- When Langfuse is on, the existing `LLMInstance.trace_config(context)` is attached to each attempt's `astream`, with
  metadata `session_id`, `user_id` (the principal) and `request_id`, plus a `target` tag. When Langfuse is off, nothing
  is attached.

### D8. Fake provider for e2e (`platform-e2e/fakes/llm_fake`)
- A stdlib-only Python (`asyncio` + `http.server`) OpenAI-compatible server. Its routes:
  - `POST /v1/chat/completions` (stream and non-stream);
  - `GET /_requests?contains=<s>`, which returns the recorded bodies;
  - `POST /_reset`;
  - `POST /api/public/ingestion` and `POST /api/public/otel/v1/traces`, which record trace bodies;
  - `GET /_ingested?contains=<s>`.
- Behaviour is scripted per request by a directive inside the last user message:
  - `[[fake primary=429 fb1=ok fb2=ok]]`
  - Modes per target are `ok`, `429`, `500`, `400`, `hang` and `break2`. `break2` streams two chunks and then drops
    the connection.
  - The target is the request's `model` field.
  - Without a directive, every target answers `ok`.
- Global per-target modes, set with `POST /_mode`, are used only by the breaker scenarios. Those run in the destructive
  lane, because the breaker state lives in the team-ai process.
- Replies are `"[<model>] "` followed by words, so e2e can tell which target answered. Usage is reported as 11
  prompt and 7 completion tokens.
- The directive passes through redaction unchanged: it has no digits or emails.
- The overlay `platform-e2e/compose/llm-fake.override.yaml` adds the `llm-fake` service and switches team-ai to these
  settings:

  | Setting | Value |
  |---|---|
  | `CHAT_BACKEND` | `llm_router` |
  | `CHAT_MODEL` | `openai:primary` |
  | `CHAT_FALLBACK_MODELS` | `openai:fb1,openai:fb2` |
  | `OPENAI_BASE_URL` | `http://llm-fake:8099/v1` |
  | `OPENAI_API_KEY` | `fake` |
  | `LLM_FIRST_TOKEN_TIMEOUT_SECONDS` | 2 |
  | `LLM_MAX_ATTEMPTS` | 3 |
  | `LLM_BREAKER_THRESHOLD` | 2 |
  | `LLM_BREAKER_COOLDOWN_SECONDS` | 5 |
  | `QUOTA_ENABLED` | `true`, memory backend |
  | `QUOTA_CHAT_REPLIES_PER_WINDOW` | 30 |
  | `GRPC_RATE_LIMIT_ENABLED` | `true` |
  | `RATE_LIMIT_PRINCIPAL_PER_MINUTE` | 20 |
  | `CHAT_HISTORY_MAX_TURNS` | 4 |

- `dc-agora-ov.sh` gains the overlay. The quota and rate-limit scenarios use fresh users, so parallel tests do not
  interfere.
- The trace scenario runs team-ai with `LANGFUSE_ENABLED=true` and `LANGFUSE_BASE_URL=http://llm-fake:8099`, and dummy
  keys in the overlay. If the Langfuse SDK exports asynchronously, the scenario polls `/_ingested`.

### D9. Eval
- `scripts/run_eval.py` gains a `chat_resilience` suite that drives `LLMRouterChatStreamer` with scripted fake chat
  models (`langchain_core` `GenericFakeChatModel` subclasses that raise on demand).
- Unscored cases count as failures, and a judge case with no judge exits 2 and names the case.
- The e2e runs `make -C team-ai eval` as a black box. The judge-case scenario passes an extra eval file through an env
  var (`EVAL_EXTRA_CASES`).

## Risks / Trade-offs

- **The overlay changes team-ai for the whole e2e stack, so the existing assistant scenarios now go through
  `llm_router`.**
  - Mitigation: the fake answers `ok` by default and mirrors the mock's tokenised streaming.
  - Mitigation: the assistant e2e asserts that a reply exists, not its text.
- **A per-principal limit of 20/min could throttle a long single-user UI test.**
  - Mitigation: UI tests register fresh users, and chat-heavy flows send fewer than 20 messages.
- **Langfuse SDK export format may change.**
  - Mitigation: the scenario asserts only that the request id string appears in some ingested body.
- **Breaker scenarios depend on wall-clock cooldown.**
  - Mitigation: the cooldown is 5 s, the steps poll with deadlines, and the scenarios run serially in the destructive
    lane.

## Migration Plan

- Production stays on its current `CHAT_BACKEND`, so the change is additive.
- Turning on `llm_router` in a deployed env requires two things:
  - provider keys from Vault;
  - when the gRPC rate limit is on, Redis plus `RATE_LIMIT_BACKEND=redis`. The boot guard enforces this.
- Rollback is a revert of the team-ai image.
