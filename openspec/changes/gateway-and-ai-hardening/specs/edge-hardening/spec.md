## Purpose

Defines how `team-gateway` protects its own HTTP endpoints, applies request-id, logging and rate limiting to every
call shape (unary, server-streaming, plain HTTP), bounds long-lived streams, applies a suitable call policy to
AI generation RPCs, serves browser CORS for `Idempotency-Key`, keeps its environment documentation enforced, and
recovers quickly when an upstream service restarts.

## ADDED Requirements

### Requirement: The cockpit metrics endpoint requires an admin principal

`GET /api/admin/metrics` SHALL be served only to a caller whose bearer token verifies and whose resolved scopes
include `admin`. A request with no token or an invalid token SHALL be rejected with HTTP 401, a request from a
verified principal without `admin` SHALL be rejected with HTTP 403, and neither response SHALL contain metric
data. The handler SHALL NOT set `Access-Control-Allow-Origin` itself: cross-origin access is decided only by the
gateway's shared CORS policy (`CORS_ORIGINS`), so a wildcard origin is never emitted by this endpoint.

#### Scenario: Anonymous caller is rejected

- **WHEN** `GET /api/admin/metrics` is called with no `Authorization` header
- **THEN** the response status is 401 and the body contains no service, RPS or trace data

#### Scenario: Non-admin principal is rejected

- **WHEN** `GET /api/admin/metrics` is called with a valid buyer or seller token
- **THEN** the response status is 403 and the body contains no service, RPS or trace data

#### Scenario: Admin principal receives the metrics

- **WHEN** `GET /api/admin/metrics` is called with a valid admin token
- **THEN** the response status is 200 with the cockpit JSON

#### Scenario: No wildcard CORS header

- **WHEN** an admin calls `GET /api/admin/metrics` with an `Origin` that is not in `CORS_ORIGINS`
- **THEN** the response carries no `Access-Control-Allow-Origin` header, and with an allowed origin it echoes that
  origin, never `*`

### Requirement: No unauthenticated live-event endpoint is exposed

The gateway SHALL NOT serve `GET /api/events/live` (or any Server-Sent-Events route) unless the route
authenticates the caller, authorises the requested room against the caller's principal, and has at least one real
event publisher. Until then the route SHALL return HTTP 404.

#### Scenario: The live endpoint is gone

- **WHEN** a client requests `GET /api/events/live?room=ops:orders`, with or without a token
- **THEN** the response status is 404 and no `text/event-stream` response is opened

### Requirement: Every RPC shape and HTTP route carries a request id, is logged and is rate limited

The gateway SHALL apply request-id assignment, structured request logging and per-key rate limiting to unary RPCs,
server-streaming RPCs (including `ChatService/StreamChat`) and the plain HTTP routes `POST /api/track` and
`GET /api/admin/metrics`; `GET /healthz` and gRPC reflection are exempt. The request id SHALL be the client's
`X-Request-Id` when it matches a safe format (1 to 64 characters of letters, digits, `.`, `_`, `-`) and a freshly
generated id otherwise; it SHALL be returned in the `X-Request-Id` response header, written to the log line, and
forwarded to the upstream service as `x-request-id`. Rate-limit keys SHALL be the resolved principal id for
authenticated callers and the client address for anonymous callers. Buckets are held per gateway instance, so the
effective limit grows with the replica count; this SHALL be documented.

#### Scenario: A stream gets a request id and a log line

- **WHEN** a client starts `StreamChat` without `X-Request-Id`
- **THEN** the response carries an `X-Request-Id` header, the upstream call carries the same value as
  `x-request-id`, and one `edge.request` log line with that id is written when the stream ends

#### Scenario: An unsafe client request id is replaced

- **WHEN** a client sends `X-Request-Id` containing spaces, control characters or more than 64 characters
- **THEN** the gateway ignores it, generates a new id, and uses the new id in the response header, the log and the
  upstream metadata

#### Scenario: StreamChat is rate limited per principal

- **WHEN** one authenticated principal starts more `StreamChat` streams within a second than the configured burst
- **THEN** the excess streams fail with `RESOURCE_EXHAUSTED` without reaching the upstream, and a second
  principal's streams in the same period are unaffected

#### Scenario: The tracking beacon is rate limited

- **WHEN** one client address posts to `/api/track` faster than the configured rate and burst
- **THEN** the excess requests receive HTTP 429 and are not produced to Kafka

#### Scenario: Health probes are never throttled

- **WHEN** `GET /healthz` is called repeatedly beyond the rate limit
- **THEN** every call returns 200

### Requirement: Long-lived streams are bounded

The gateway SHALL limit how long a server-streaming call may run (`STREAM_MAX_SECONDS`) and how many streams one
principal may hold open at once (`STREAM_MAX_CONCURRENT_PER_PRINCIPAL`). A stream that exceeds the duration SHALL
end with `DEADLINE_EXCEEDED` and its upstream call SHALL be cancelled. A new stream beyond the concurrency cap
SHALL fail with `RESOURCE_EXHAUSTED`, and the slot SHALL be released when a stream ends for any reason.

#### Scenario: A stalled upstream cannot hold a stream forever

- **WHEN** an upstream `StreamChat` produces no terminal message within `STREAM_MAX_SECONDS`
- **THEN** the client receives `DEADLINE_EXCEEDED` and the upstream stream is cancelled

#### Scenario: Concurrent stream cap

- **WHEN** a principal holding the maximum number of open streams opens one more
- **THEN** the new stream fails with `RESOURCE_EXHAUSTED`, and after one existing stream ends a new stream is
  accepted

### Requirement: AI generation RPCs use a dedicated call policy

`ShoppingAssistant`, `MagicListing` and `ChatCopilot` SHALL be forwarded with a single attempt (no retry on any
status) and a deadline of `AI_CALL_TIMEOUT_SECONDS`, independent of `CALL_TIMEOUT_SECONDS` and `RETRY_MAX`.
`SummarizeReviews`, which is rendered on the public product page, SHALL keep the standard read policy.

#### Scenario: A generation call is not retried

- **WHEN** the upstream answers `ChatCopilot` with `UNAVAILABLE`
- **THEN** the upstream receives exactly one attempt and the client gets `UNAVAILABLE`

#### Scenario: A slow generation call is allowed past the standard deadline

- **WHEN** the upstream takes longer than `CALL_TIMEOUT_SECONDS` but less than `AI_CALL_TIMEOUT_SECONDS` to answer
  `MagicListing`
- **THEN** the client receives the successful response

#### Scenario: The product page summary keeps the short read policy

- **WHEN** the upstream does not answer `SummarizeReviews` within `CALL_TIMEOUT_SECONDS`
- **THEN** the call fails with `DEADLINE_EXCEEDED` at that deadline, not at `AI_CALL_TIMEOUT_SECONDS`

### Requirement: CORS permits the Idempotency-Key header

The gateway's CORS policy SHALL list `Idempotency-Key` among the allowed request headers, so a browser preflight
that announces it succeeds for an allowed origin.

#### Scenario: Preflight with Idempotency-Key succeeds

- **WHEN** a browser sends `OPTIONS /platform.order.v1.OrderService/CreateOrder` with an allowed `Origin` and
  `Access-Control-Request-Headers: idempotency-key,content-type,authorization`
- **THEN** the response allows those headers (`Access-Control-Allow-Headers` includes `Idempotency-Key`)

#### Scenario: Disallowed origin is still refused

- **WHEN** the same preflight comes from an origin that is not in `CORS_ORIGINS`
- **THEN** no `Access-Control-Allow-Origin` header is returned

### Requirement: The env-example gate is real and every declared setting is used

`make check-env` SHALL fail when `.env.example` and the settings struct disagree in either direction, by running a
test that exists. Every declared environment variable SHALL be read by the running gateway; `DIAL_TIMEOUT_SECONDS`
SHALL bound each upstream connection attempt.

#### Scenario: Drift fails the gate

- **WHEN** a setting is added to the settings struct without a matching `.env.example` line
- **THEN** `make check-env` exits non-zero and names the missing key

#### Scenario: The gate cannot pass vacuously

- **WHEN** `make check-env` runs
- **THEN** at least one test executes (a missing test is a failure, not a pass)

#### Scenario: Dial timeout bounds a connection attempt

- **WHEN** an upstream address blackholes connection attempts and `DIAL_TIMEOUT_SECONDS=1`
- **THEN** a call to that upstream fails with `UNAVAILABLE` or `DEADLINE_EXCEEDED` within its call deadline and
  each connection attempt is abandoned after about one second

### Requirement: The gateway recovers quickly after an upstream restarts

After an upstream service restarts on the same address, the gateway SHALL serve idempotent reads successfully no
later than 2 seconds after the upstream starts listening, without the caller retrying. Reads that arrive while the
upstream is unreachable SHALL wait for reconnection for at most `RECONNECT_WAIT_SECONDS` and within their call
deadline, then fail with `UNAVAILABLE`. Non-idempotent calls SHALL NOT wait and SHALL fail fast, then succeed on the
next attempt once the connection is back (same 2 second bound). Upstream connection backoff SHALL be configurable
(`UPSTREAM_BACKOFF_BASE_MS`, `UPSTREAM_BACKOFF_MAX_MS`).

#### Scenario: A read issued during a restart succeeds

- **WHEN** an upstream stops, a read is issued, and the upstream starts listening again on the same address within
  `RECONNECT_WAIT_SECONDS`
- **THEN** the read returns the upstream's response within 2 seconds of the upstream listening, not a 503

#### Scenario: A write after a restart recovers within the bound

- **WHEN** an upstream restarts and a write is issued before the connection is re-established
- **THEN** that write fails fast with `UNAVAILABLE`, and a write issued 2 seconds after the upstream started
  listening succeeds

#### Scenario: A long outage still fails promptly

- **WHEN** an upstream stays down longer than `RECONNECT_WAIT_SECONDS`
- **THEN** a read fails with `UNAVAILABLE` after at most `RECONNECT_WAIT_SECONDS` plus the standard retry backoff, and does
  not hold a goroutine until the full call deadline

### Requirement: Internal stock RPCs are not exposed at the edge

The gateway SHALL NOT route `platform.listing.v1.ListingService/ReserveStock` or
`platform.listing.v1.ListingService/ReleaseStock` (internal service-to-service RPCs used by `team-order`). A request
to either path, from an anonymous or an authenticated caller of any role, SHALL fail with HTTP 501 / Connect
`unimplemented` and SHALL NOT reach `team-domain`. The other `ListingService` RPCs are unaffected.

#### Scenario: Anonymous reserve is not routed

- **WHEN** an anonymous client posts to `/platform.listing.v1.ListingService/ReserveStock`
- **THEN** the response is HTTP 501 with Connect code `unimplemented` and `team-domain` receives no call

#### Scenario: Authenticated release is not routed

- **WHEN** a buyer, seller or admin posts to `/platform.listing.v1.ListingService/ReleaseStock`
- **THEN** the response is HTTP 501 with Connect code `unimplemented` and `team-domain` receives no call

#### Scenario: Public listing reads still work

- **WHEN** an anonymous client calls `ListingService/GetListing`
- **THEN** the call is forwarded and succeeds
