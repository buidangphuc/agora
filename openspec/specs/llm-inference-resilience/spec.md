# llm-inference-resilience Specification

## Purpose
Defines how `team-ai` selects, protects, meters, prompts, redacts and observes calls to the external LLM on the
chat path (`StreamChat` with `CHAT_BACKEND=llm_router`), and the eval gate that proves it.

## Requirements

### Requirement: The chat path routes each request and falls back only before the first chunk

With `CHAT_BACKEND=llm_router`, team-ai SHALL choose the model target for each `StreamChat` request through the router
(not once at startup), walking the ordered chain `CHAT_MODEL` then every entry of `CHAT_FALLBACK_MODELS` once: a target
is called at most once per request and is never retried before the next one. The only repeats a provider sees are the
SDK's own retries, bounded by `LLM_MAX_RETRIES`. team-ai SHALL report the outcome of every call to that target's breaker.
On a transient failure before any text has been streamed to the caller it SHALL try the next target. A deterministic
failure (a 4xx caused by the request) SHALL fail the call at once without trying another target. After the first chunk
of text a failure SHALL end the stream with an error and SHALL NOT switch models, so one reply never mixes two models.
When every target fails before the first chunk the call SHALL fail with `UNAVAILABLE`, or `RESOURCE_EXHAUSTED` when the
last failure was a 429, with a fixed message and no partial reply.

#### Scenario: A healthy primary serves the reply

- **WHEN** a buyer streams a chat message through the gateway and every fake target is healthy
- **THEN** the full reply comes from the primary target

#### Scenario: A primary 429 before the first token falls back

- **WHEN** a buyer streams a chat message for which the primary target answers 429 and the first fallback is healthy
- **THEN** the buyer receives one complete reply from the first fallback and no error

#### Scenario: The whole chain is tried in order

- **WHEN** a buyer streams a chat message for which the primary answers 500 and the first fallback answers 429
- **THEN** the buyer receives one complete reply from the second fallback

#### Scenario: A failure after the first chunk does not switch models

- **WHEN** a buyer streams a chat message for which the primary streams two chunks and then breaks the connection
- **THEN** the buyer receives those two chunks, then an error status, and no chunk from any other target

#### Scenario: An exhausted chain fails cleanly

- **WHEN** a buyer streams a chat message for which every target answers 500
- **THEN** the call fails with `unavailable`, the error message contains no exception or provider text, and no reply
  chunk was streamed

### Requirement: Each target has a breaker that counts transient failures and recovers

Each target in the chain SHALL have its own circuit breaker. A breaker SHALL count HTTP 429, HTTP 5xx, timeouts and
connection errors as failures, and SHALL NOT count other 4xx caused by the request. A call counts once, however many
times the SDK retried inside it. After `LLM_BREAKER_FAILURE_THRESHOLD` consecutive failures it SHALL open, and requests
SHALL skip that target. After `LLM_BREAKER_COOLDOWN_SECONDS` it SHALL let
exactly one probe request through. The probe's success SHALL close the breaker; its failure SHALL re-open it for a fresh
cooldown.

#### Scenario: Repeated 5xx opens the primary's breaker

- **WHEN** the primary target answers 500 for `LLM_BREAKER_FAILURE_THRESHOLD` consecutive chat requests and then becomes healthy
- **THEN** the next chat request made within the cooldown is served by the first fallback without the primary being
  called

#### Scenario: A request-caused 400 does not open the breaker

- **WHEN** the primary answers 400 for `LLM_BREAKER_FAILURE_THRESHOLD` consecutive chat requests and then becomes healthy
- **THEN** the next chat request is served by the primary

#### Scenario: A successful probe closes the breaker

- **WHEN** the primary's breaker is open, the cooldown elapses and the primary is healthy again
- **THEN** the next chat request is served by the primary, and so is the one after it

### Requirement: LLM calls are bounded by a first-token timeout, the SDK retry budget and deadlines

Each target's call SHALL be cancelled when no text arrives within `CHAT_FIRST_TOKEN_TIMEOUT_SECONDS` and recorded as a
timeout failure. Calls before the first chunk SHALL stop after one call per chain target, each with at most
`LLM_MAX_RETRIES` SDK retries. The whole walk SHALL stay within `LLM_CHAIN_DEADLINE_SECONDS`, and no call SHALL run past
the calling request's gRPC deadline; on expiry the call SHALL fail with `DEADLINE_EXCEEDED`, the upstream request SHALL
be cancelled, and the cut-off SHALL NOT count against the target's breaker.

#### Scenario: A hung primary times out and falls back

- **WHEN** a buyer streams a chat message for which the primary never sends a first token and the first fallback is
  healthy
- **THEN** the reply comes from the first fallback within `CHAT_FIRST_TOKEN_TIMEOUT_SECONDS` plus 3 seconds

#### Scenario: Attempts are bounded

- **WHEN** a buyer streams a chat message for which every target answers 500
- **THEN** the fake provider received at most one request per target and SDK retry (`1 + LLM_MAX_RETRIES` per target) for
  that message

### Requirement: Token usage is captured and metered against quota

team-ai SHALL record the provider-reported input and output token counts of each completed reply, with the target, in
its structured log. It SHALL always meter quota, counted in Redis under `QUOTA_REDIS_PREFIX`, and reserve the principal's quota under a server-minted
reservation id before calling the model (one reply unit per call), finalize it on success, and refund it when no chunk
was delivered. A principal whose quota is exhausted SHALL get `RESOURCE_EXHAUSTED` and no model request SHALL be made.

#### Scenario: Usage is recorded for a completed reply

- **WHEN** a buyer's chat reply completes and the fake provider reported 11 input and 7 output tokens
- **THEN** team-ai's log has one usage line for that request id with 11 input tokens, 7 output tokens and the target

#### Scenario: An exhausted quota refuses the call

- **WHEN** a buyer has used up the per-principal quota configured on the e2e stack and streams another chat message
- **THEN** the call fails with `resource_exhausted` and the fake provider received no request for that message

#### Scenario: A failed call does not consume quota

- **WHEN** a buyer with quota for exactly one more reply streams a message for which every target answers 500, then
  streams a message with healthy targets
- **THEN** the first call fails with `unavailable` and the second reply completes

### Requirement: Chat and assistant RPCs are rate limited per principal

`StreamChat` and `ShoppingAssistant` SHALL always be limited per forwarded principal, in Redis, and
SHALL answer `RESOURCE_EXHAUSTED` when the limit is exceeded, without calling the model. Anonymous callers SHALL be keyed
by the client IP the gateway forwards (`x-client-ip`), so one visitor cannot exhaust the bucket of every other anonymous
visitor; a forwarded principal with an empty id SHALL be treated as anonymous. The limit and the quota can no longer be
switched off or moved to process memory, so team-ai has no boot guard for them (flag-trim).

#### Scenario: One buyer over the limit does not affect another

- **WHEN** one buyer streams more chat messages within a minute than the per-principal limit on the e2e stack
- **THEN** the calls past the limit fail with `resource_exhausted`, and a different buyer's chat message right after
  succeeds

### Requirement: The model input carries a system prompt and bounded session history

Every model request SHALL start with a system prompt: the Langfuse prompt when Langfuse is enabled and the fetch
succeeds, otherwise `CHAT_SYSTEM_PROMPT` or the built-in static prompt. It SHALL include the earlier completed turns of
the same `session_id`:
- in order;
- at most `CHAT_HISTORY_MAX_TURNS` turns and `CHAT_HISTORY_MAX_TOKENS` tokens, dropping the oldest first;
- expiring after `CHAT_HISTORY_TTL_SECONDS`.

A request without a `session_id` SHALL be stateless. A reply that failed SHALL leave neither its partial text nor its
user turn in the history.

#### Scenario: The system prompt is always first

- **WHEN** a buyer streams a chat message on the e2e stack, where Langfuse is off
- **THEN** the request the fake provider received starts with a system message equal to the configured system prompt

#### Scenario: A follow-up carries the session history

- **WHEN** a buyer streams "first question" and then "second question" with the same `session_id`
- **THEN** the second request the fake provider received contains, after the system message, the first user turn, the
  first assistant reply and the second user turn, in that order

#### Scenario: History is bounded

- **WHEN** a buyer streams more messages in one session than `CHAT_HISTORY_MAX_TURNS`
- **THEN** the last request the fake provider received contains only the most recent `CHAT_HISTORY_MAX_TURNS` earlier
  turns

#### Scenario: A failed reply is not kept as history

- **WHEN** a buyer streams a message for which every target answers 500, then streams a follow-up with the same
  `session_id`
- **THEN** the follow-up request the fake provider received contains no turn from the failed message

### Requirement: Personal data is redacted before the model and in logs

Text sent to the model SHALL always be redacted:
- email addresses, `sk-` keys and bearer tokens;
- Vietnamese mobile numbers (`0` or `+84` followed by 9 digits, spaces or dots allowed);
- 9-digit CMND or 12-digit CCCD numbers that follow an ID keyword (`CMND`, `CCCD`, `căn cước`).

Bare digit strings without an ID keyword (prices, order numbers) SHALL NOT be masked. User text in team-ai's own log lines
(`ShoppingAssistant`, `ChatCopilot`) SHALL carry the same masks. There is no setting that logs raw user text. Langfuse
traces are not redacted by team-ai.

#### Scenario: Phone and citizen ID are masked before the model

- **WHEN** a buyer streams "gọi 0912 345 678, CCCD 079123456789"
- **THEN** the request the fake provider received contains redaction markers and neither "0912 345 678" nor
  "079123456789"

#### Scenario: Prices and order numbers are not masked

- **WHEN** a buyer streams "giá 850000000, order id 123456789"
- **THEN** the request the fake provider received contains "850000000" and "123456789"

#### Scenario: Assistant logs carry redacted text

- **WHEN** a buyer asks the Shopping Assistant through the gateway a question containing "a.b@example.com"
- **THEN** no team-ai log line contains "a.b@example.com"

### Requirement: Chat attempts are traced when Langfuse is configured

When both `LANGFUSE_PUBLIC_KEY` and `LANGFUSE_SECRET_KEY` are set, each chat model attempt SHALL be exported with the session id, principal id and request
id, the target and the outcome. When either key is empty, no trace callback SHALL be attached and replies SHALL be
unaffected.

#### Scenario: A chat trace carries the request id

- **WHEN** team-ai runs with Langfuse enabled against the fake provider's ingestion endpoint and a buyer streams a chat
  message with `X-Request-Id` "e2e-trace-<random>"
- **THEN** the fake ingestion endpoint receives a trace containing "e2e-trace-<random>" and the buyer's id

### Requirement: The eval gate runs the deterministic cases

`make eval` in team-ai SHALL grade the completions surface against the built-in echo handler with the deterministic
evaluators (`exact`, `contains`), exit zero at a pass rate of at least the gate threshold, and print a JSON report. A case
that needs a judge SHALL be skipped, and reported as skipped, when `JUDGE_CHAT_MODEL` is empty.

Known gap: the rewritten gate no longer includes chat resilience cases (primary 429 fallback, all targets failing, a
failure after the first chunk; these are covered by team-ai's unit tests only) and no longer fails on an unscored judge
case.

#### Scenario: The eval gate scores the deterministic cases

- **WHEN** `make eval` runs in team-ai with no judge configured
- **THEN** it exits zero with a pass rate of 100% and the judge cases skipped
