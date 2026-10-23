## ADDED Requirements

### Requirement: team-gateway builds on go-sdk v1.0.2 with the ADR-0016 layout

team-gateway SHALL require `github.com/buidangphuc/agora/platform-core/packages/go-sdk` at the tag `v1.0.2` with no `replace` directive and no `third_party` copy. `cmd/gateway/main.go` SHALL only load `internal/config` settings and call `internal/app.Run(ctx, settings)`. The Makefile SHALL include `../platform-core/make/go-service.mk` and the SDK `.golangci.yml`. The binary SHALL NOT import a test fake, a no-op publisher or an in-memory stand-in for a dependency. The image SHALL build from `cmd/gateway` without a `third_party` directory.

#### Scenario: go.mod uses the tag without replace
- **WHEN** `go.mod` is parsed
- **THEN** it requires go-sdk `v1.0.2` and has no `replace` directive and no `third_party` directory exists
- **VERIFIED BY**: team-gateway `internal/app` `TestGoModRequiresSDKTagWithoutReplace`

#### Scenario: The runtime binary does not import a fake
- **WHEN** the dependency graph of `./cmd/gateway` is listed
- **THEN** it contains no package whose path ends in `test`, `fake` or `noop`
- **VERIFIED BY**: team-gateway `internal/app` `TestRuntimeDoesNotImportFakes`

#### Scenario: The Makefile uses the shared targets
- **WHEN** `make help` is run in `team-gateway`
- **THEN** the output lists `check`, `lint`, `test`, `check-env` and `docker` from `go-service.mk`
- **VERIFIED BY**: team-gateway `internal/app` `TestMakefileIncludesSharedTargets`

### Requirement: Settings take the SDK listener keys and keep the edge keys

`internal/config` SHALL load `Settings` with `config.Load`, composed of the SDK groups `Runtime`, `GRPC`, `Kafka`, `OTel` and `MockData` and the service groups for upstream addresses, edge auth, edge hardening and event topics. `ENV` and `GRPC_PORT` SHALL be required. Settings SHALL declare no dependency enable flag: the only boolean switches SHALL be `OTEL_ENABLED` and `GRPC_REFLECTION_ENABLED`. Every edge key that is not an SDK group key (`RATE_LIMIT_RPS`, `RATE_LIMIT_BURST`, `CALL_TIMEOUT_SECONDS`, `RETRY_MAX`, `AI_CALL_TIMEOUT_SECONDS`, `TRACK_RATE_LIMIT_RPS`, `TRACK_RATE_LIMIT_BURST`, `STREAM_MAX_REQUEST_BYTES`, `STREAM_REVOCATION_CHECK_SECONDS`, `DIAL_TIMEOUT_SECONDS`, `JWKS_URL`, `JWKS_CACHE_TTL`, `PUBLIC_SCOPES`, `CORS_ORIGINS`, `TRUSTED_PROXIES`, `PROMETHEUS_URL`, `JAEGER_QUERY_URL`, `JAEGER_UI_URL`, `KAFKA_ANALYTICS_TOPIC`, `IDENTITY_EVENTS_TOPIC` and the sixteen `UPSTREAM_*_ADDR`) SHALL keep its name, unit and default. The one new key SHALL be `TRACK_PUBLISH_TIMEOUT` (default `2s`). Every `UPSTREAM_*_ADDR` SHALL be required in every `ENV`: an empty value is refused and no upstream has a disabled or fallback mode. `config.Load` SHALL refuse, naming the key, a `JWKS_URL` that is empty, a `JWKS_CACHE_TTL` of 0 or less, a `RETRY_MAX` below 0, a `TRACK_PUBLISH_TIMEOUT` of 0 or less and an empty upstream address. `.env.example` SHALL list exactly the declared keys.

#### Scenario: Settings declare no dependency enable flags
- **WHEN** the declared env keys of Settings are listed
- **THEN** no key other than `OTEL_ENABLED` and `GRPC_REFLECTION_ENABLED` ends in `_ENABLED`, and `KAFKA_ENABLED`, `EDGE_REFLECTION_ENABLED`, `HTTP_PORT` and `SHUTDOWN_GRACE_SECONDS` are not declared
- **VERIFIED BY**: team-gateway `internal/config` `TestNoDependencyEnableFlags`

#### Scenario: ENV is required
- **WHEN** config loads with `ENV` unset
- **THEN** `Load` returns an error naming `ENV`
- **VERIFIED BY**: team-gateway `internal/config` `TestLoadRequiresEnv`

#### Scenario: The listen port is required
- **WHEN** config loads with `ENV=local`, `JWKS_URL` set and `GRPC_PORT` unset
- **THEN** `Load` returns an error naming `GRPC_PORT`
- **VERIFIED BY**: team-gateway `internal/config` `TestLoadRequiresListenPort`

#### Scenario: Defaults and overrides
- **WHEN** config loads with only `ENV=local`, `GRPC_PORT=9090` and `JWKS_URL` set, then again with `RATE_LIMIT_RPS=1000` and `SHUTDOWN_GRACE=3s`
- **THEN** the first has `RATE_LIMIT_RPS` 20, `RATE_LIMIT_BURST` 40, `CALL_TIMEOUT_SECONDS` 5, `RETRY_MAX` 2, `JWKS_CACHE_TTL` 300, `TRACK_PUBLISH_TIMEOUT` 2s, `SHUTDOWN_GRACE` 10s, `KAFKA_BROKERS` localhost:9092 and reflection off, and the second has rate 1000 and grace 3s
- **VERIFIED BY**: team-gateway `internal/config` `TestLoadDefaultsAndOverrides`

#### Scenario: A missing JWKS URL is rejected in every environment
- **WHEN** config loads with `ENV=local`, `GRPC_PORT=8080` and `JWKS_URL` empty
- **THEN** `Load` returns an error naming `JWKS_URL`
- **VERIFIED BY**: team-gateway `internal/config` `TestLoadRequiresJWKSURL`

#### Scenario: An empty upstream address is rejected
- **WHEN** config loads with `UPSTREAM_ORDER_ADDR` set to the empty string
- **THEN** `Load` returns an error naming `UPSTREAM_ORDER_ADDR` and does not substitute the default
- **VERIFIED BY**: team-gateway `internal/config` `TestLoadRefusesEmptyUpstreamAddr`

#### Scenario: An out-of-range bound is rejected
- **WHEN** config loads once each with `JWKS_CACHE_TTL=0`, `RETRY_MAX=-1`, `TRACK_PUBLISH_TIMEOUT=0s` and `TRACK_PUBLISH_TIMEOUT=-1s`
- **THEN** each `Load` returns an error naming the offending key
- **VERIFIED BY**: team-gateway `internal/config` `TestLoadRefusesInvalidBounds`

#### Scenario: Retired keys are ignored
- **WHEN** config loads with `HTTP_PORT=7000`, `KAFKA_ENABLED=false`, `SHUTDOWN_GRACE_SECONDS=99` and `GRPC_PORT=8080` set
- **THEN** the port is 8080, the shutdown grace is 10s and no field takes the retired values
- **VERIFIED BY**: team-gateway `internal/config` `TestRetiredKeysAreIgnored`

#### Scenario: An explicit empty URL disables a cockpit source
- **WHEN** config loads with `PROMETHEUS_URL` and `JAEGER_QUERY_URL` set to the empty string
- **THEN** both settings are empty and the cockpit handler returns zeroed values and no traces
- **VERIFIED BY**: team-gateway `internal/config` `TestEmptyCockpitURLsStayEmpty`

#### Scenario: Env example matches the settings
- **WHEN** `.env.example` is compared with the Settings keys
- **THEN** every declared key is listed and no other key appears
- **VERIFIED BY**: team-gateway `internal/config` `TestEnvExampleInSync`

### Requirement: Strict environments refuse missing or unsafe configuration

WHEN `ENV` is staging, stage, prod or production, `config.Load` SHALL refuse a missing `KAFKA_BROKERS` and a non-empty `MOCK_DATA`, and SHALL name the offending key. `MOCK_DATA` SHALL be refused before any other validation error is reported. In every `ENV`, a `MOCK_DATA` adapter name the gateway does not declare SHALL fail `app.Run` before a port is opened. The refusal of `GRPC_REFLECTION_ENABLED=true` applies to every `ENV` other than `local` and `test`, which is a wider set than the four strict names above; an `ENV` outside both sets (for example `dev`) SHALL therefore still refuse reflection but SHALL accept a missing `KAFKA_BROKERS` (default `localhost:9092`) and a `MOCK_DATA` adapter the gateway declares.

#### Scenario: Kafka brokers are required in a strict environment
- **WHEN** config loads with `ENV=prod` and `KAFKA_BROKERS` unset
- **THEN** `Load` returns an error naming `KAFKA_BROKERS`
- **VERIFIED BY**: team-gateway `internal/config` `TestStrictEnvRequiresKafkaBrokers`

#### Scenario: Mock data is refused in a strict environment before other errors
- **WHEN** config loads with `ENV=staging`, `MOCK_DATA=cockpit` and `GRPC_PORT` unset
- **THEN** the error names `MOCK_DATA` and is reported ahead of the `GRPC_PORT` error
- **VERIFIED BY**: team-gateway `internal/config` `TestStrictEnvRefusesMockDataFirst`

#### Scenario: An environment outside both sets refuses only reflection
- **WHEN** config loads with `ENV=dev`, `GRPC_PORT=8080`, `JWKS_URL` set and `KAFKA_BROKERS` unset, then again with `GRPC_REFLECTION_ENABLED=true`
- **THEN** the first `Load` succeeds with `KAFKA_BROKERS` `localhost:9092` and the second returns an error naming `GRPC_REFLECTION_ENABLED`
- **VERIFIED BY**: team-gateway `internal/config` `TestEnvOutsideBothSetsRefusesOnlyReflection`

#### Scenario: An undeclared mock adapter fails boot locally
- **WHEN** `Run` starts with `ENV=local` and `MOCK_DATA=payments`
- **THEN** `Run` returns an error naming `payments` within 2 s and the listen port accepts no connection
- **VERIFIED BY**: team-gateway `internal/app` `TestUnknownMockAdapterFailsBoot`

### Requirement: The analytics producer is always the Kafka producer

`app.Run` SHALL build the analytics producer with `kafkax.NewProducer` from `KAFKA_BROKERS` in every `ENV`; there SHALL be no no-op producer. `POST /api/track` SHALL wrap each accepted event in an `EventEnvelope`, produce it to `KAFKA_ANALYTICS_TOPIC` keyed by the session id (else the anonymous id, else the listing id), and the record SHALL carry the `traceparent` and `x-request-id` headers. A produce failure SHALL be logged at warn level and SHALL NOT change the 202 answer. Each publish SHALL be bounded by `TRACK_PUBLISH_TIMEOUT`.

#### Scenario: A beacon becomes a keyed record with trace headers
- **WHEN** a valid view beacon with session id "s-1" and `X-Request-Id` "req-1" is posted to `/api/track` against a real broker
- **THEN** one record appears on `analytics.events` with key "s-1", an `EventEnvelope` of type `platform.analytics.v1.TrackingEvent`, and the headers `traceparent` and `x-request-id` equal to "req-1"
- **VERIFIED BY**: team-gateway `internal/events` `TestKafkaPublisherProducesKeyedRecordWithHeaders`

#### Scenario: A produce failure still answers 202
- **WHEN** the publisher returns an error for a valid beacon
- **THEN** `/api/track` answers 202, one warn line `publish tracking event` is logged and nothing else changes
- **VERIFIED BY**: team-gateway `internal/edge` `TestTrackAcceptsBeaconWhenPublishFails`

#### Scenario: A stuck broker cannot hold the beacon
- **WHEN** the publisher blocks until its context ends and `TRACK_PUBLISH_TIMEOUT` is 200ms
- **THEN** `/api/track` answers 202 within 1 s and the publisher's context error is logged
- **VERIFIED BY**: team-gateway `internal/edge` `TestTrackPublishIsBoundedByTimeout`

#### Scenario: A malformed beacon is rejected without a publish
- **WHEN** a beacon with no recognizable event type is posted
- **THEN** `/api/track` answers 400 and the publisher is not called
- **VERIFIED BY**: team-gateway `internal/edge` `TestHandleTrackUnknownTypeStillRejected`

#### Scenario: The producer has no disabled mode
- **WHEN** `Run` starts with the broker address of a closed port and `KAFKA_ENABLED=false` in the environment
- **THEN** `Run` still builds the producer (the variable is not read) and serves, `Check("liveness")` is SERVING, and `Check("")` stays NOT_SERVING because the revocation consumer has not caught up
- **VERIFIED BY**: team-gateway `internal/app` `TestProducerIsBuiltWithoutEnableFlag`

### Requirement: The session revocation consumer always runs and has no consumer group

`app.Run` SHALL start the revocation consumer in every `ENV`. It SHALL read `IDENTITY_EVENTS_TOPIC` through `kafkax.NewConsumerClient` with an empty group, so that every replica reads every partition from the earliest offset and no consumer group exists on the broker. A `SessionRevoked` event SHALL put its session id on the denylist until the event's expiry (24 hours when absent), and the edge SHALL answer `unauthenticated` for a token whose `sid` is on it. While the consumer cannot read the topic it SHALL keep retrying every 5 seconds and `gateway_revocation_consumer_up` SHALL be 0; the consumer state is reported through that gauge and its lag and size gauges. Kafka is not a readiness check: there SHALL be no producer or broker ping check. The `revocation` check SHALL fail until the consumer has read up to the end offset of every partition captured when it started (an empty topic is caught up at once, once the offsets are known), and SHALL return nil from then on, even if the broker later becomes unreachable, so a replica never serves while its denylist is behind the revocations that existed at its start, and a later broker outage does not take the pod out of the Service. A record that cannot be decoded SHALL be skipped and logged with its partition and offset only.

#### Scenario: A revoked session is rejected after the event is read
- **WHEN** a `SessionRevoked` event for session "sid-1" is produced to `identity.events` and a token with `sid` "sid-1" is then presented
- **THEN** within 5 s the edge answers `unauthenticated` for that token and still accepts a token with `sid` "sid-2"
- **VERIFIED BY**: team-gateway `internal/revocation` `TestConsumerFeedsDenylistFromTopic`

#### Scenario: Revocations survive a restart
- **WHEN** a revocation was produced, the consumer is stopped, and a new consumer is started on the same topic
- **THEN** the new denylist contains the session id within 5 s
- **VERIFIED BY**: team-gateway `internal/revocation` `TestConsumerReplaysFromEarliestOnRestart`

#### Scenario: The consumer creates no consumer group
- **WHEN** the consumer has read the topic against a real broker
- **THEN** listing the consumer groups of the broker returns none for the gateway
- **VERIFIED BY**: team-gateway `internal/revocation` `TestConsumerCreatesNoConsumerGroup`

#### Scenario: An undecodable record is skipped without logging a key
- **WHEN** a record with a non-protobuf value and key "user-42" is produced
- **THEN** the consumer logs one warn line with the partition and the offset, the line does not contain "user-42", and the next record is still applied
- **VERIFIED BY**: team-gateway `internal/revocation` `TestUndecodableRecordIsSkippedWithoutKey`

#### Scenario: Readiness waits for the denylist to catch up
- **WHEN** 50 `SessionRevoked` events exist on `identity.events` before the gateway starts and the revocation check runs while the consumer has applied fewer than 50, then after it has applied all 50
- **THEN** the check returns an error naming the consumer, then returns nil, and a token whose `sid` is among the 50 is refused as soon as the check is nil
- **VERIFIED BY**: team-gateway `internal/app` `TestRevocationCheckWaitsForCatchUp`

#### Scenario: An empty topic is caught up at once
- **WHEN** the gateway starts against a reachable broker whose `identity.events` topic exists and is empty
- **THEN** the revocation check returns nil within 10 s
- **VERIFIED BY**: team-gateway `internal/revocation` `TestEmptyTopicIsCaughtUpImmediately`

#### Scenario: A broker outage after catch-up does not unset the check
- **WHEN** the consumer has caught up and the broker is then stopped
- **THEN** `gateway_revocation_consumer_up` reads 0 within 15 s while the revocation check keeps returning nil
- **VERIFIED BY**: team-gateway `internal/revocation` `TestCaughtUpStaysAfterBrokerOutage`

#### Scenario: Other event types are ignored
- **WHEN** an `EventEnvelope` of another type or a `SessionRevoked` with an empty session id is applied
- **THEN** the denylist stays empty
- **VERIFIED BY**: team-gateway `internal/revocation` `TestApplyIgnoresOtherEventTypesAndEmptySession`

#### Scenario: An unreachable broker keeps the consumer retrying
- **WHEN** the consumer runs against a closed port for 12 s
- **THEN** `Stats().Up` is false, the denylist is empty, the consumer is still running and no panic occurs
- **VERIFIED BY**: team-gateway `internal/revocation` `TestRunWithUnreachableKafkaFailsOpen`

#### Scenario: Expired entries are dropped
- **WHEN** the expiry of a denylist entry has passed and the prune runs
- **THEN** the entry is no longer held and `Revoked` returns false for it
- **VERIFIED BY**: team-gateway `internal/revocation` `TestExpiredEntriesAreRejectedAndPruned`

#### Scenario: A revoked token is refused on every route
- **WHEN** a token whose `sid` is on the denylist is presented to a public route and to a protected route
- **THEN** both answer `unauthenticated` without an upstream call
- **VERIFIED BY**: team-gateway `internal/edge` `TestRevokedSessionIsRejected`

### Requirement: Health reports readiness as the overall status and liveness independently

The gateway SHALL serve `grpc.health.v1.Health/Check` through its Connect mux, backed by `healthx`. The service `""` (readiness) SHALL be SERVING only once the revocation consumer has caught up (see the revocation requirement) and the JWKS has been fetched at least once; the gateway SHALL register no Kafka check and no Postgres check (it holds no database), so a broker outage after start does not change readiness. The service `liveness` SHALL be SERVING from start, SHALL stay SERVING while any dependency is down (before and after catch-up), and both SHALL become NOT_SERVING when shutdown starts. A check of an unknown service name SHALL answer `not_found`. `GET /healthz` SHALL answer 200 `ok` without consulting any dependency. The health handler SHALL be mounted outside the bearer resolution, the per-IP rate limit, the procedure policy tables and the request-id interceptor, and SHALL carry no data beyond SERVING or NOT_SERVING.

#### Scenario: Readiness is SERVING when the denylist has caught up and the JWKS is available
- **WHEN** the gateway runs against a reachable broker and a JWKS server and `Check("")` is called over the gRPC protocol on its listener
- **THEN** within 10 s the status is SERVING
- **VERIFIED BY**: team-gateway `internal/app` `TestHealthServingWhenDependenciesUp`

#### Scenario: Readiness waits for the first JWKS fetch
- **WHEN** the gateway starts while the JWKS server answers 503
- **THEN** `Check("")` is NOT_SERVING and `Check("liveness")` is SERVING, and after the JWKS server recovers `Check("")` is SERVING within 10 s
- **VERIFIED BY**: team-gateway `internal/app` `TestReadinessWaitsForJWKS`

#### Scenario: A broker outage after start does not change readiness
- **WHEN** the gateway is ready and the broker is then stopped
- **THEN** for 12 s `Check("")` stays SERVING, `Check("liveness")` stays SERVING and `GET /healthz` answers 200
- **VERIFIED BY**: team-gateway `internal/app` `TestReadinessIgnoresKafkaAfterCatchUp`

#### Scenario: A gateway that starts without a broker is not ready
- **WHEN** the gateway starts with the broker stopped
- **THEN** `Check("")` is NOT_SERVING, `Check("liveness")` is SERVING and `GET /healthz` answers 200, and after the broker starts `Check("")` is SERVING within 15 s
- **VERIFIED BY**: team-gateway `internal/app` `TestStartWithoutBrokerIsNotReadyUntilCaughtUp`

#### Scenario: An identity outage after the first fetch does not flip readiness
- **WHEN** the JWKS server is stopped after the first successful fetch
- **THEN** `Check("")` stays SERVING for 12 s and a token signed by a cached key still verifies
- **VERIFIED BY**: team-gateway `internal/app` `TestReadyAfterJWKSOutageWithCachedKeys`

#### Scenario: A health check needs no bearer token and is not rate limited
- **WHEN** `Check("")` is called 200 times in 1 s without an `Authorization` header while `RATE_LIMIT_RPS` is 1 and `RATE_LIMIT_BURST` is 1
- **THEN** every call returns SERVING or NOT_SERVING (never `unauthenticated`, `permission_denied` or `resource_exhausted`) and the response carries no `X-Request-Id` header
- **VERIFIED BY**: team-gateway `internal/app` `TestHealthCheckBypassesAuthAndRateLimit`

#### Scenario: An unknown health service is not found
- **WHEN** `Check("billing")` is called
- **THEN** the call fails with `not_found`
- **VERIFIED BY**: team-gateway `internal/app` `TestHealthUnknownServiceIsNotFound`

#### Scenario: The health check speaks the gRPC protocol on the edge port
- **WHEN** a plain `grpc.NewClient` with insecure credentials calls `Check("liveness")` on the gateway listener
- **THEN** the call succeeds with SERVING
- **VERIFIED BY**: team-gateway `internal/app` `TestHealthCheckOverGRPCProtocol`

#### Scenario: The process health route is unchanged
- **WHEN** `GET /healthz` is called while the broker and the JWKS are unavailable
- **THEN** the answer is 200 with body `ok`
- **VERIFIED BY**: team-gateway `internal/app` `TestHealthzIsProcessLiveness`

### Requirement: Tokens are verified against the cached JWKS and a bad token never becomes anonymous

The edge SHALL verify bearer tokens with RS256 against the keys of `JWKS_URL`, selected by `kid`, decoded with the SDK `jwtx` wire types, cached for `JWKS_CACHE_TTL` seconds and refreshed once per 10 s on an unknown `kid`. A token SHALL need `exp` and a non-empty `sub`. A request without a bearer credential SHALL resolve to the anonymous principal with `PUBLIC_SCOPES`; a request with a bearer credential that does not verify SHALL be answered `unauthenticated` with `WWW-Authenticate: Bearer error="invalid_token"`. `app.Run` SHALL start when the JWKS URL is unreachable: it SHALL call `Verifier.Prime` at boot under a 5 s bound, log a failure at warn level without failing boot, and until the first successful fetch SHALL retry `Prime` every 5 s in a background component that stops at the first success or at shutdown. `Verifier.Check` SHALL NOT fetch and SHALL NOT depend on a request arriving.

#### Scenario: A current key verifies
- **WHEN** a token signed by a key published in the JWKS is verified
- **THEN** the claims carry `sub`, `typ`, `scopes` and `sid` from the token
- **VERIFIED BY**: team-gateway `internal/token` `TestVerifierAcceptsCurrentKey`

#### Scenario: A rotated-in key is accepted after one forced refresh
- **WHEN** identity publishes a new `kid` and a token signed by it arrives
- **THEN** the verifier accepts it after one refresh
- **VERIFIED BY**: team-gateway `internal/token` `TestVerifierAcceptsRotatedInKey`

#### Scenario: An unknown key is rejected
- **WHEN** a token signed by a key that is not in the JWKS arrives
- **THEN** the verifier returns an error and the edge answers `unauthenticated`
- **VERIFIED BY**: team-gateway `internal/token` `TestVerifierRejectsUnknownKey`

#### Scenario: A token without expiry or subject is rejected
- **WHEN** a token without `exp`, and then a token without `sub`, is verified
- **THEN** both return an error
- **VERIFIED BY**: team-gateway `internal/token` `TestVerifierRequiresExpAndSub`

#### Scenario: JWKS keys decode through the SDK types
- **WHEN** the JWKS server publishes one RSA key, one key with `kty` "EC" and one RSA key with an empty `kid`
- **THEN** only the RSA key with a `kid` is usable
- **VERIFIED BY**: team-gateway `internal/token` `TestJWKSDocumentDecodesWithSDKTypes`

#### Scenario: An invalid bearer is never downgraded to anonymous
- **WHEN** a request to a public route carries `Authorization: Bearer garbage`
- **THEN** the edge answers `unauthenticated` with the `WWW-Authenticate` header and makes no upstream call
- **VERIFIED BY**: team-gateway `internal/edge` `TestBearerResolution`

#### Scenario: An anonymous caller gets the public scopes only
- **WHEN** a request without credentials calls a procedure gated by the `admin` scope
- **THEN** the edge answers `permission_denied` without an upstream call
- **VERIFIED BY**: team-gateway `internal/edge` `TestRequireProcedureScope`

#### Scenario: The JWKS fetch is retried in the background until it succeeds
- **WHEN** `Run` starts while the JWKS server answers 503 and no request is made, and the server then recovers
- **THEN** within 10 s of the recovery the verifier has fetched the keys, `Check("")` is SERVING, and no further fetch is made by the background component
- **VERIFIED BY**: team-gateway `internal/app` `TestJWKSPrimeRetriesInBackgroundWithoutTraffic`

#### Scenario: Boot does not fail when identity is down
- **WHEN** `Run` starts with `JWKS_URL` pointing at a closed port
- **THEN** the listener accepts connections within 5 s and anonymous public reads are served
- **VERIFIED BY**: team-gateway `internal/app` `TestRunStartsWhenJWKSUnreachable`

### Requirement: Upstream reads retry on Unavailable, writes and AI calls do not

`callRead` SHALL run each read under `resilience.Retry` with `RETRY_MAX + 1` attempts, a 50 ms base and 200 ms maximum backoff, retrying only `Unavailable`. There SHALL be no circuit breaker: the SDK retry is the only resilience policy on the read path and no `UPSTREAM_BREAKER_*` key exists. An error caused by the caller's context (the request was cancelled, or its deadline, for example from `Connect-Timeout-Ms`, expired before the attempt's own timeout) SHALL NOT be retried, and `callRead` SHALL stop retrying as soon as the caller's context is done. A `DeadlineExceeded` from the attempt's own `CALL_TIMEOUT_SECONDS` and any business error (`NotFound`, `PermissionDenied`, `InvalidArgument`, `FailedPrecondition`, `ResourceExhausted`) SHALL NOT be retried. `callWrite` and `callAI` SHALL make exactly one attempt. Each upstream attempt SHALL be counted in the `attempts` field of the `edge.request` line.

#### Scenario: A read retries Unavailable up to RETRY_MAX
- **WHEN** an upstream read returns `Unavailable` twice and then succeeds with `RETRY_MAX` 2
- **THEN** the call succeeds and the `edge.request` line reports `attempts` 3
- **VERIFIED BY**: team-gateway `internal/edge` `TestCallReadRetriesUnavailableUpToRetryMax`

#### Scenario: A persistent outage stops after RETRY_MAX plus one attempts
- **WHEN** an upstream read returns `Unavailable` on every attempt with `RETRY_MAX` 2
- **THEN** the call fails with `unavailable` after exactly 3 upstream calls
- **VERIFIED BY**: team-gateway `internal/edge` `TestCallReadStopsAfterRetryMaxPlusOne`

#### Scenario: A business error is not retried
- **WHEN** an upstream read returns `NotFound`, then `DeadlineExceeded`, then `ResourceExhausted` in three separate calls
- **THEN** each call makes exactly one upstream attempt
- **VERIFIED BY**: team-gateway `internal/edge` `TestCallReadRetriesOnlyUnavailable`

#### Scenario: A caller deadline or cancellation is not retried
- **WHEN** a read is made with a caller context cancelled mid-call, and another with a caller deadline of 1 ms against an upstream that blocks
- **THEN** each makes exactly one upstream attempt and returns within 1 s of the cancellation or deadline
- **VERIFIED BY**: team-gateway `internal/edge` `TestCallerDeadlineAndCancelAreNotRetried`

#### Scenario: A failing upstream is called on every read
- **WHEN** 20 consecutive reads against one service each get `Unavailable` on every attempt with `RETRY_MAX` 0
- **THEN** each of the 20 reaches the upstream once (20 upstream calls in total), so no breaker short-circuits a read
- **VERIFIED BY**: team-gateway `internal/edge` `TestNoBreakerEveryReadReachesUpstream`

#### Scenario: A write and an AI call make one attempt
- **WHEN** an order write and a `MagicListing` call each get `Unavailable` from the upstream
- **THEN** each makes exactly one upstream call and reports `attempts` 1
- **VERIFIED BY**: team-gateway `internal/edge` `TestWriteAndAICallsAreSingleAttempt`

### Requirement: The edge forwards principal, request id and idempotency key unchanged

For every upstream call the edge SHALL set `x-principal-id`, `x-principal-type`, `x-principal-scopes` and `x-request-id` from the resolved principal and the validated request id, set `x-client-ip` and `x-client-user-agent` from edge-observed values, and overwrite any inbound value of those keys. The anonymous principal SHALL be forwarded with id `anonymous`, type `anonymous` and the public scopes. A client `Idempotency-Key` SHALL be forwarded as `idempotency-key` only when, after surrounding whitespace is trimmed, it is 1 to 255 printable ASCII characters and the header appears once; the check applies to the procedures that forward a key (today `CreateOrder`); otherwise (including a repeated header) the call SHALL be refused with `invalid_argument` and the message `invalid Idempotency-Key header: must be a single printable-ASCII value of at most 255 characters`. A client `X-Request-Id` SHALL be kept only when it matches `^[A-Za-z0-9._-]{1,64}$`.

#### Scenario: Spoofed principal headers are overwritten
- **WHEN** a client sends `x-principal-id: admin-1` and `x-principal-scopes: admin` with a valid user token
- **THEN** the upstream receives the token's subject and scopes and never the client's values
- **VERIFIED BY**: team-gateway `internal/edge` `TestRefreshSessionForwardedPublicDropsSpoofedPrincipalHeaders`

#### Scenario: The anonymous principal is forwarded with the public scopes
- **WHEN** a request without credentials reaches the search upstream
- **THEN** the upstream metadata holds `x-principal-id` "anonymous", `x-principal-type` "anonymous" and the `PUBLIC_SCOPES` list
- **VERIFIED BY**: team-gateway `internal/edge` `TestAnonymousPrincipalIsForwardedWithPublicScopes`

#### Scenario: A valid idempotency key is forwarded
- **WHEN** a client sends `Idempotency-Key: ord-123`
- **THEN** the upstream metadata holds `idempotency-key` "ord-123"
- **VERIFIED BY**: team-gateway `internal/edge` `TestIdempotencyKeyForwardedAndValidated`

#### Scenario: An invalid idempotency key is refused
- **WHEN** a client sends a key of 256 characters, then a key with a control character, then an empty key
- **THEN** each call is refused with `invalid_argument` and the fixed message and no upstream call is made
- **VERIFIED BY**: team-gateway `internal/edge` `TestValidIdempotencyKey`

#### Scenario: A repeated idempotency key is refused
- **WHEN** a client sends two `Idempotency-Key` headers to `CreateOrder`, and separately sends one key padded with spaces (`  ord-123  `)
- **THEN** the repeated header is refused with `invalid_argument`, the fixed message and no upstream call, and the padded key is forwarded as `ord-123`
- **VERIFIED BY**: team-gateway `internal/edge` `TestRepeatedIdempotencyKeyIsRefusedAndWhitespaceTrimmed`

#### Scenario: An unsafe request id is replaced
- **WHEN** a client sends `X-Request-Id` of 65 characters, then one containing a space
- **THEN** a fresh id is forwarded and echoed in `X-Request-Id`
- **VERIFIED BY**: team-gateway `internal/edge` `TestRequestIDIsValidatedBeforeForwarding`

#### Scenario: Client context cannot be spoofed
- **WHEN** a client sends `X-Forwarded-For` from a peer that is not in `TRUSTED_PROXIES`
- **THEN** `x-client-ip` is the socket peer address
- **VERIFIED BY**: team-gateway `internal/edge` `TestClientContextDirectIgnoresSpoofedHeaders`

### Requirement: Shutdown drains health first and stops within the grace period

On SIGINT or SIGTERM the gateway SHALL turn the health services NOT_SERVING, wait `SHUTDOWN_DRAIN_DELAY`, stop accepting connections, let in-flight requests finish for at most `SHUTDOWN_GRACE` and close the Kafka producer, the revocation consumer, the upstream connections and the telemetry providers. `Run` SHALL return within `SHUTDOWN_DRAIN_DELAY + SHUTDOWN_GRACE` plus 1 s. A listener that cannot bind SHALL end `Run` with that error.

#### Scenario: Health drains before the server stops
- **WHEN** the context of `Run` is cancelled with `SHUTDOWN_DRAIN_DELAY` of 300ms
- **THEN** `Check("")` and `Check("liveness")` answer NOT_SERVING during the delay while the listener still accepts connections
- **VERIFIED BY**: team-gateway `internal/app` `TestHealthDrainsBeforeServerStops`

#### Scenario: Run returns within the grace period
- **WHEN** the context is cancelled while a request that never finishes is in flight and `SHUTDOWN_GRACE` is 1s
- **THEN** `Run` returns within 3 s and the listener is closed
- **VERIFIED BY**: team-gateway `internal/app` `TestRunShutsDownWithinGrace`

#### Scenario: A busy port fails boot
- **WHEN** `Run` starts with `GRPC_PORT` of a port that is already bound
- **THEN** `Run` returns a non-nil error within 5 s
- **VERIFIED BY**: team-gateway `internal/app` `TestRunFailsWhenPortIsBusy`

### Requirement: Logs and telemetry use the SDK and keep the edge.request fields

The gateway SHALL build its logger with `logx` and its tracer and meter with `otelx`. `OTEL_ENABLED=false` SHALL install no exporter. The `edge.request` line SHALL keep the fields `method`, `principal`, `code`, `latency_ms`, `request_id` and `attempts`, and SHALL gain `principal_id` and `trace_id` from the context. No log line SHALL contain a bearer token, a request body or a Kafka record key. The revocation gauges `gateway_revocation_consumer_up`, `gateway_revocation_consumer_lag` and `gateway_revocation_denylist_size` SHALL be registered on the global meter.

#### Scenario: The edge.request line keeps its fields
- **WHEN** one unary call is served with `X-Request-Id` "req-9"
- **THEN** the log line `edge.request` has `method`, `principal`, `code`, `latency_ms`, `request_id` "req-9" and `attempts`, with `request_id` appearing once
- **VERIFIED BY**: team-gateway `internal/edge` `TestEdgeRequestLineKeepsFields`

#### Scenario: A token never reaches the logs
- **WHEN** a call with an invalid `Authorization: Bearer <token>` is served and refused as `unauthenticated`
- **THEN** no captured log line contains the token string
- **VERIFIED BY**: team-gateway `internal/edge` `TestBearerTokenIsNeverLogged`

#### Scenario: Telemetry is disabled by default
- **WHEN** `Run` starts with `OTEL_ENABLED` unset
- **THEN** no OTLP connection is attempted and startup is unaffected
- **VERIFIED BY**: team-gateway `internal/app` `TestOTelDisabledByDefault`

#### Scenario: The revocation gauges report the consumer state
- **WHEN** the consumer is down, then up with two denylist entries
- **THEN** the gauges read up 0 and size 0, then up 1 and size 2
- **VERIFIED BY**: team-gateway `internal/revocation` `TestRevocationGaugesReportConsumerState`
