## Purpose

Defines how `team-ai` calls the external LLM on the chat streaming path (`platform.chat.v1.ChatService/StreamChat`
with `CHAT_BACKEND=llm_router`) so that one provider failure, hang or rate limit does not break buyers, cost is
metered, user data is redacted before it leaves the process, and the behaviour is covered by an eval gate.

## ADDED Requirements

### Requirement: Every LLM attempt selects a target per request and records its outcome

The chat streamer SHALL choose the model target for each request through the router (not once at startup) and
SHALL report the outcome of every attempt (success, or error with its status when known) back to the router.

#### Scenario: Healthy primary serves the request

- **WHEN** a chat request is made and the primary model streams a full reply
- **THEN** the reply comes from the primary target and the router records one success for it

#### Scenario: Breaker state changes the next request's target

- **WHEN** the primary's breaker has opened from recorded failures and a new chat request arrives
- **THEN** that request is served by the next healthy target in the chain without a restart

### Requirement: Fallback happens only before the first streamed chunk

On a failure before any chunk has been emitted to the caller, the streamer SHALL try the next target in the
ordered chain (`CHAT_MODEL`, then every entry of `CHAT_FALLBACK_MODELS` in order). After the first chunk has
been emitted, a failure SHALL terminate the stream with an error and SHALL NOT switch models, so one reply never
mixes output from two models.

#### Scenario: Primary fails before first token

- **WHEN** the primary raises a 429 or 5xx before yielding any chunk and the secondary is healthy
- **THEN** the caller receives one complete reply from the secondary and never sees the failure

#### Scenario: Whole chain is tried in order

- **WHEN** the primary and the first fallback both fail before the first chunk and a second fallback is configured
- **THEN** the second fallback serves the reply

#### Scenario: Failure after first chunk does not switch models

- **WHEN** the primary yields two chunks and then fails
- **THEN** the stream ends with an error status, no chunk from any other model is emitted, and the failure is
  recorded against the primary

#### Scenario: Chain exhausted

- **WHEN** every target in the chain fails before the first chunk
- **THEN** the call fails with gRPC `UNAVAILABLE` (or `RESOURCE_EXHAUSTED` when the last failure was a 429)
  and no partial reply is emitted

### Requirement: The breaker counts transient provider failures and recovers

A target's circuit breaker SHALL count HTTP 429, HTTP 5xx, timeouts and connection errors as failures, SHALL
NOT count other 4xx client errors caused by the request, and SHALL, after a cooldown, allow a single half-open
probe whose success closes the breaker and whose failure re-opens it for another cooldown. Each target in the
chain SHALL have its own breaker.

#### Scenario: Timeouts and 5xx open the breaker

- **WHEN** a target produces consecutive timeouts, connection errors or 5xx responses up to the threshold
- **THEN** its breaker opens and requests skip it

#### Scenario: Request-caused 4xx does not open the breaker

- **WHEN** a target returns HTTP 400 or 404 for a malformed request
- **THEN** the breaker failure count does not increase

#### Scenario: Half-open probe recovers a target

- **WHEN** an open breaker's cooldown elapses and one probe request to that target succeeds
- **THEN** the breaker closes and the target is used again

#### Scenario: Failed probe re-opens

- **WHEN** the half-open probe fails
- **THEN** the breaker re-opens for a fresh cooldown and concurrent requests are not allowed through as probes

### Requirement: LLM calls are bounded by timeouts, bounded retry and the caller's deadline

The streamer SHALL enforce a first-token timeout per attempt, SHALL retry at most a bounded number of times
before the first chunk using the shared retry policy, and SHALL never run past the gRPC deadline of the
calling request.

#### Scenario: Hung provider hits the first-token timeout

- **WHEN** a target yields no chunk within the first-token timeout
- **THEN** the attempt is cancelled, recorded as a timeout failure, and the next retry or target is tried

#### Scenario: Retry is bounded and pre-chunk only

- **WHEN** a target fails repeatedly before the first chunk
- **THEN** attempts stop at the configured maximum and no retry is made once a chunk has been emitted

#### Scenario: gRPC deadline is honoured

- **WHEN** the caller's remaining deadline is shorter than the configured timeouts
- **THEN** the call fails with `DEADLINE_EXCEEDED` no later than that deadline and upstream work is cancelled

### Requirement: Token usage is captured and metered against quota

When the provider reports usage, the system SHALL capture input/output token counts per call and expose them
to tracing and metrics. When `QUOTA_ENABLED` is true it SHALL reserve quota before the call, finalize it with
the actual usage on success, and refund it when no chunk was delivered.

#### Scenario: Usage recorded on success

- **WHEN** a reply completes and the provider reported token usage
- **THEN** the input and output token counts are recorded for that request and target

#### Scenario: Quota exhausted refuses the call

- **WHEN** `QUOTA_ENABLED` is true and the principal's quota is exhausted
- **THEN** the call fails with `RESOURCE_EXHAUSTED` and no LLM request is made

#### Scenario: Failed call is refunded

- **WHEN** a quota was reserved and the chain fails before the first chunk
- **THEN** the reservation is refunded

#### Scenario: Quota disabled changes nothing

- **WHEN** `QUOTA_ENABLED` is false
- **THEN** no quota reserve/finalize/refund is performed

### Requirement: gRPC chat and assistant RPCs are rate limited per principal

`StreamChat` and `ShoppingAssistant` SHALL be rate limited per forwarded principal, answering
`RESOURCE_EXHAUSTED` with a retry hint when exceeded. A production configuration SHALL use the shared (Redis)
rate-limit backend, and startup SHALL fail rather than silently use a per-process limiter in production.

#### Scenario: Principal over the limit

- **WHEN** one principal exceeds the configured request rate on `StreamChat`
- **THEN** further calls in the window fail with `RESOURCE_EXHAUSTED` while other principals are unaffected

#### Scenario: Shared limit across replicas

- **WHEN** the Redis backend is configured and two replicas serve the same principal
- **THEN** their requests count against one shared limit

#### Scenario: Production without Redis backend fails fast

- **WHEN** the service starts in a production environment with `RATE_LIMIT_BACKEND=memory`
- **THEN** startup fails with a configuration error

### Requirement: The chat prompt carries a system prompt and bounded session history

Each LLM request SHALL begin with a system prompt (from the Langfuse prompt when available, otherwise a
built-in static prompt) and SHALL include prior turns of the same `session_id`, bounded by a maximum turn count
and token budget and expiring after a TTL. Requests without a `session_id` SHALL be stateless.

#### Scenario: System prompt always present

- **WHEN** Langfuse is disabled or its prompt fetch fails
- **THEN** the request still starts with the static system prompt

#### Scenario: Follow-up uses session history

- **WHEN** a second message arrives with the same `session_id` within the TTL
- **THEN** the model input contains the earlier user and assistant turns in order

#### Scenario: History is bounded and expires

- **WHEN** a session exceeds the turn or token bound, or its TTL elapses
- **THEN** the oldest turns are dropped, or the history is cleared, respectively

#### Scenario: Failed reply is not stored

- **WHEN** a reply fails before completing
- **THEN** neither the partial reply nor an unanswered user turn is kept as history

### Requirement: Personal data is redacted before reaching the LLM and the logs

The system SHALL apply the redaction policy, including Vietnamese phone numbers and keyword-anchored
national ID numbers (9-digit CMND or 12-digit CCCD that follow an ID keyword such as `CMND` or `CCCD`) in
addition to email, `sk-` keys and bearer tokens, to text sent to the LLM and to every log line that contains
user text, including those written by `AIAssistantService`. Bare digit strings without an ID keyword (prices,
order numbers) SHALL NOT be masked, and national-ID masking SHALL NOT be applied to RAG document indexing.

#### Scenario: Phone and ID are masked in LLM input

- **WHEN** a user message contains a Vietnamese mobile number and a citizen ID written as `CCCD 079123456789`
- **THEN** the text sent to the model contains the redaction markers instead of the values

#### Scenario: Prices and order numbers are not masked

- **WHEN** a user message or document contains `850000000` (a price) or `order id 123456789`
- **THEN** those numbers are left unchanged

#### Scenario: User text is masked in logs

- **WHEN** ShoppingAssistant or ChatCopilot logs the incoming message
- **THEN** the log line contains the redacted text, not the raw text

#### Scenario: Trace mode is respected

- **WHEN** the trace content mode is `full`
- **THEN** redaction follows that configured policy and is applied consistently to input and logs

### Requirement: Chat LLM calls are traced

Each chat LLM attempt SHALL carry the Langfuse trace configuration with session id, principal id and request id,
and SHALL record the target model and outcome; when Langfuse is disabled, chat SHALL behave identically.

#### Scenario: Trace attached when enabled

- **WHEN** Langfuse is enabled and a chat request is served
- **THEN** the LLM call is made with trace config carrying the session, user and request ids

#### Scenario: Langfuse off is a no-op

- **WHEN** Langfuse is disabled
- **THEN** no callback is attached and the reply is unaffected

### Requirement: The eval gate exercises the real chat streamer and cannot pass vacuously

`make eval` SHALL include a chat eval set that runs against the real streamer with fake chat models
simulating 429, 5xx, timeout and mid-stream failure, and SHALL assert fallback, no-mixed-model and error
behaviour. The gate SHALL fail (non-zero exit) when a case that requires a judge is present and no judge model
is configured, and the reported pass rate SHALL NOT exclude such cases from its denominator.

#### Scenario: Chat resilience cases run in the gate

- **WHEN** `make eval` runs
- **THEN** cases for primary 429 fallback, all-targets-fail, and failure after first chunk are executed
  against the real streamer and must pass

#### Scenario: Missing judge fails the gate

- **WHEN** the eval set contains a judge case and `JUDGE_CHAT_MODEL` is empty
- **THEN** the run exits non-zero and names the unscored cases instead of skipping them

#### Scenario: Skipped cases cannot inflate the pass rate

- **WHEN** a case is not scored for any reason
- **THEN** it counts as a failure in the pass rate
