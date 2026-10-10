# llm-inference-resilience Specification

## Purpose
Defines how `team-ai` selects, protects, meters, prompts, redacts and observes calls to the external LLM on the
chat path (`StreamChat` with `CHAT_BACKEND=llm_router`), and the eval gate that proves it.

## Requirements

### Requirement: The chat path routes each request and falls back only before the first chunk

With `CHAT_BACKEND=llm_router`, team-ai SHALL choose the model target for each `StreamChat` request through the router
(not once at startup), trying the ordered chain `CHAT_MODEL` then every entry of `CHAT_FALLBACK_MODELS`. It SHALL report
the outcome of every attempt to the router. On a failure before any chunk has been streamed to the caller it SHALL try
the next target. After the first chunk a failure SHALL end the stream with an error and SHALL NOT switch models, so one
reply never mixes two models. When every target fails before the first chunk the call SHALL fail with `UNAVAILABLE`,
or `RESOURCE_EXHAUSTED` when the last failure was a 429, with a fixed message and no partial reply.

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
connection errors as failures, and SHALL NOT count other 4xx caused by the request. After `LLM_BREAKER_THRESHOLD`
consecutive failures it SHALL open, and requests SHALL skip that target. After `LLM_BREAKER_COOLDOWN_SECONDS` it SHALL let
exactly one probe request through. The probe's success SHALL close the breaker; its failure SHALL re-open it for a fresh
cooldown.

#### Scenario: Repeated 5xx opens the primary's breaker

- **WHEN** the primary target answers 500 for `LLM_BREAKER_THRESHOLD` consecutive chat requests and then becomes healthy
- **THEN** the next chat request made within the cooldown is served by the first fallback without the primary being
  called

#### Scenario: A request-caused 400 does not open the breaker

- **WHEN** the primary answers 400 for `LLM_BREAKER_THRESHOLD` consecutive chat requests and then becomes healthy
- **THEN** the next chat request is served by the primary

#### Scenario: A successful probe closes the breaker

- **WHEN** the primary's breaker is open, the cooldown elapses and the primary is healthy again
- **THEN** the next chat request is served by the primary, and so is the one after it

### Requirement: LLM calls are bounded by a first-token timeout, bounded retry and the caller's deadline

Each attempt SHALL be cancelled when no chunk arrives within `LLM_FIRST_TOKEN_TIMEOUT_SECONDS` and recorded as a timeout
failure. Attempts before the first chunk SHALL stop at `LLM_MAX_ATTEMPTS` in total. No attempt SHALL run past the
calling request's gRPC deadline; on expiry the call SHALL fail with `DEADLINE_EXCEEDED` and the upstream request SHALL be
cancelled.

#### Scenario: A hung primary times out and falls back

- **WHEN** a buyer streams a chat message for which the primary never sends a first token and the first fallback is
  healthy
- **THEN** the reply comes from the first fallback within `LLM_FIRST_TOKEN_TIMEOUT_SECONDS` plus 3 seconds

#### Scenario: Attempts are bounded

- **WHEN** a buyer streams a chat message for which every target answers 500
- **THEN** the fake provider received at most `LLM_MAX_ATTEMPTS` requests for that message

### Requirement: Token usage is captured and metered against quota

team-ai SHALL record the provider-reported input and output token counts of each completed reply, with the target, in
its structured log. When `QUOTA_ENABLED` is true it SHALL reserve the principal's quota under a server-minted
reservation id before calling the model, finalize it with the actual usage on success, and refund it when no chunk was
delivered. A principal whose quota is exhausted SHALL get `RESOURCE_EXHAUSTED` and no model request SHALL be made.

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

With `GRPC_RATE_LIMIT_ENABLED`, `StreamChat` and `ShoppingAssistant` SHALL be limited per forwarded principal and
SHALL answer `RESOURCE_EXHAUSTED` when the limit is exceeded, without calling the model. Anonymous callers SHALL be keyed
by the client IP the gateway forwards (`x-client-ip`), so one visitor cannot exhaust the bucket of every other anonymous
visitor; a forwarded principal with an empty id SHALL be treated as anonymous. Because anonymous chat is allowed
(`ai:use` is not enforced), team-ai SHALL refuse to start with `CHAT_BACKEND=llm_router` outside dev, local and test
unless both `GRPC_RATE_LIMIT_ENABLED` and `QUOTA_ENABLED` are true. When team-ai's environment
setting `ENVIRONMENT` is anything other than dev, local or test, team-ai SHALL refuse to start with the gRPC rate limit
enabled and `RATE_LIMIT_BACKEND=memory`.

#### Scenario: One buyer over the limit does not affect another

- **WHEN** one buyer streams more chat messages within a minute than the per-principal limit on the e2e stack
- **THEN** the calls past the limit fail with `resource_exhausted`, and a different buyer's chat message right after
  succeeds

#### Scenario: Production refuses a per-process rate limiter

- **WHEN** the team-ai image is started with `ENVIRONMENT=production`, `GRPC_RATE_LIMIT_ENABLED=true` and
  `RATE_LIMIT_BACKEND=memory`
- **THEN** the process exits non-zero and its log names `RATE_LIMIT_BACKEND`

#### Scenario: Production refuses an unmetered LLM chat path

- **WHEN** the team-ai image is started with `ENVIRONMENT=production`, `CHAT_BACKEND=llm_router`,
  `GRPC_RATE_LIMIT_ENABLED=false` and `QUOTA_ENABLED=false`
- **THEN** the process exits non-zero and its log names `QUOTA_ENABLED`

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

Text sent to the model SHALL always be redacted, whatever `LLM_TRACE_CONTENT` says:
- email addresses, `sk-` keys and bearer tokens;
- Vietnamese mobile numbers (`0` or `+84` followed by 9 digits, spaces or dots allowed);
- 9-digit CMND or 12-digit CCCD numbers that follow an ID keyword (`CMND`, `CCCD`, `căn cước`).

Bare digit strings without an ID keyword (prices, order numbers) SHALL NOT be masked. User text in log lines
(`StreamChat`, `ShoppingAssistant`, `ChatCopilot`) and in traces SHALL follow `LLM_TRACE_CONTENT`:
- `redacted` (default): the masks above;
- `off`: no user text;
- `full`: raw text, refused when `ENVIRONMENT` is production.

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

### Requirement: Chat attempts are traced when Langfuse is enabled

When `LANGFUSE_ENABLED` is true, each chat model attempt SHALL be exported with the session id, principal id and request
id, the target and the outcome. When Langfuse is disabled, no trace callback SHALL be attached and replies SHALL be
unaffected.

#### Scenario: A chat trace carries the request id

- **WHEN** team-ai runs with Langfuse enabled against the fake provider's ingestion endpoint and a buyer streams a chat
  message with `X-Request-Id` "e2e-trace-<random>"
- **THEN** the fake ingestion endpoint receives a trace containing "e2e-trace-<random>" and the buyer's id

### Requirement: The eval gate exercises the real streamer and cannot pass vacuously

`make eval` in team-ai SHALL run a chat resilience eval set against the real streamer with fake models: primary 429
fallback, all targets failing, and a failure after the first chunk. It SHALL exit non-zero when a case requiring a judge
is present and `JUDGE_CHAT_MODEL` is empty, naming the unscored cases. Every unscored case SHALL count as a failure in the
reported pass rate.

#### Scenario: The eval gate runs the chat resilience cases

- **WHEN** `make eval` runs in team-ai with no judge configured and no judge cases
- **THEN** it exits zero and its report lists the primary-429, all-fail and after-first-chunk cases as passed

#### Scenario: A judge case without a judge fails the gate

- **WHEN** `make eval` runs with an eval set containing one judge case and `JUDGE_CHAT_MODEL` empty
- **THEN** it exits non-zero, names that case, and reports a pass rate below 100%
