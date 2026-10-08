## ADDED Requirements

### Requirement: Streams get the same edge policy as unary calls

For every Connect streaming procedure the gateway routes (today `ChatService/StreamChat`), `team-gateway` SHALL:
- validate or mint the request id, and return it in the `X-Request-Id` response header;
- refuse a presented but invalid bearer token with `unauthenticated` (HTTP 401) before any upstream call, never
  downgrading it to anonymous;
- apply the edge admin procedure policy;
- charge the caller's per-identity rate-limit bucket, the same bucket unary calls use, and refuse with
  `resource_exhausted` when it is empty;
- write one `edge.request` log line per stream with the procedure, principal, final code, latency and request id.

A stream request larger than `STREAM_MAX_REQUEST_BYTES` (default 16384) SHALL be refused with `resource_exhausted`
without reaching the upstream.

#### Scenario: An invalid token on a stream is refused

- **WHEN** a client calls `StreamChat` through the gateway with a bearer token whose signature is invalid
- **THEN** the gateway answers HTTP 401 with code `unauthenticated` and no chat message is streamed

#### Scenario: A stream echoes the request id

- **WHEN** a logged-in buyer calls `StreamChat` through the gateway with `X-Request-Id` "e2e-stream-<random>"
- **THEN** the stream completes and the response carries `X-Request-Id` "e2e-stream-<random>"

#### Scenario: Streams are rate limited per caller

- **WHEN** one freshly registered buyer opens more `StreamChat` calls at once than the gateway's rate-limit burst
- **THEN** at least one of them fails with `resource_exhausted`, and a different buyer's `StreamChat` call made right
  after succeeds

#### Scenario: An oversized stream request is refused

- **WHEN** a logged-in buyer calls `StreamChat` with a 20 000-byte message
- **THEN** the call fails with `resource_exhausted`

### Requirement: The tracking collector is rate limited

`POST /api/track` SHALL charge a per-visitor bucket (`TRACK_RATE_LIMIT_RPS`, `TRACK_RATE_LIMIT_BURST`; defaults 5 and 20)
keyed by the verified user, or by the client IP for anonymous visitors. It SHALL answer HTTP 429 when the bucket is
empty, without producing any event of that request to `analytics.events`. It SHALL echo a validated `X-Request-Id`.
`/api/admin/metrics` SHALL charge the caller's ordinary per-identity bucket. Both routes SHALL write one `edge.request`
log line per request.

#### Scenario: A beacon flood is throttled

- **WHEN** one anonymous visitor sends 40 `POST /api/track` batches back to back, each with one event carrying a unique
  marker
- **THEN** some requests answer HTTP 429, and none of the markers of the 429 requests reach `analytics.events`

#### Scenario: The collector echoes the request id

- **WHEN** a visitor sends one valid `POST /api/track` batch with `X-Request-Id` "e2e-track-<random>"
- **THEN** the response is accepted and carries `X-Request-Id` "e2e-track-<random>"

### Requirement: AI generation calls are sent once with their own deadline

`team-gateway` SHALL forward `AIService/MagicListing`, `AIService/ChatCopilot`, `AIService/SummarizeReviews` and
`AIService/ShoppingAssistant` exactly once, never retrying on `unavailable`, with the deadline
`AI_CALL_TIMEOUT_SECONDS` (default 30) instead of the read deadline. The `edge.request` log line SHALL carry the number
of upstream attempts.

#### Scenario: A generation call is not retried when team-ai is down

- **WHEN** team-ai is stopped and a logged-in seller calls `MagicListing` through the gateway with `X-Request-Id`
  "e2e-ai-<random>"
- **THEN** the call fails with `unavailable`, and the gateway's `edge.request` log line for "e2e-ai-<random>" reports
  one upstream attempt

### Requirement: Edge reflection is off unless enabled, and never in staging or production

`team-gateway` SHALL serve gRPC reflection only when `EDGE_REFLECTION_ENABLED=true` (default `false`). When `ENV` is
staging or production it SHALL refuse to start with `EDGE_REFLECTION_ENABLED=true`, exiting non-zero with a message
naming the setting.

#### Scenario: Reflection is not served by default

- **WHEN** a client posts a reflection request to `/grpc.reflection.v1.ServerReflection/ServerReflectionInfo` on the
  local gateway
- **THEN** the gateway answers HTTP 404

#### Scenario: The gateway refuses reflection in production

- **WHEN** the team-gateway image is started with `ENV=production` and `EDGE_REFLECTION_ENABLED=true`
- **THEN** the process exits non-zero and its log names `EDGE_REFLECTION_ENABLED`

### Requirement: The edge recovers quickly after an upstream restart

After an upstream service is recreated with a new address and reports ready, `team-gateway` SHALL route calls to it
again within 5 seconds, without being restarted itself. The reconnect delay SHALL be bounded by `DIAL_TIMEOUT_SECONDS`.

#### Scenario: A recreated upstream is reachable again within seconds

- **WHEN** the team-payment container is recreated and its gRPC port answers
- **THEN** within 5 seconds a seller's `GetWalletBalance` through the gateway succeeds, and it keeps succeeding on 3
  consecutive calls
