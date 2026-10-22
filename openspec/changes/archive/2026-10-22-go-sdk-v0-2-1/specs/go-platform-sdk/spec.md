## ADDED Requirements

### Requirement: Outbox enqueue is idempotent on event id

`outbox.Store.EnqueueTx` SHALL insert with `ON CONFLICT (event_id) DO NOTHING`, so enqueuing an existing event id inside a transaction neither fails nor duplicates.

#### Scenario: Duplicate enqueue does not abort the transaction
- **WHEN** the same event id is enqueued twice in one transaction, which then commits
- **THEN** the commit succeeds and exactly one outbox row exists
- **VERIFIED BY**: `platform-core/packages/go-sdk/pkg/outbox` `TestEnqueueDuplicateIsNoop`

### Requirement: Upstream calls can use a per-call service principal

`upstream.WithServicePrincipal(ctx, id, scopes...)` SHALL return a context. A call made with that context SHALL send that service principal instead of the forwarded or connection-level principal.

#### Scenario: Per-call service principal overrides forwarding, not other calls
- **WHEN** two calls on one connection are made, one with `WithServicePrincipal(ctx, "team-order", "listing.read")` and one without, while the incoming principal is a buyer
- **THEN** the first sends `team-order`/`service`/`listing.read`, and the second forwards the buyer, not the service principal
- **VERIFIED BY**: `platform-core/packages/go-sdk/pkg/upstream` `TestPerCallServicePrincipal`

### Requirement: Flags can be built over any provider

`flags.NewWithProvider(p openfeature.FeatureProvider, timeout time.Duration)` SHALL return a client evaluating through `p`. Evaluation errors SHALL fall back to the caller's default.

#### Scenario: In-memory provider toggles a flag and an unknown flag does not fail
- **WHEN** a client over an in-memory provider with `checkout-enabled=false` evaluates it with default `true`
- **THEN** it returns `false`, and an unknown flag returns its default rather than failing
- **VERIFIED BY**: `platform-core/packages/go-sdk/pkg/flags` `TestNewWithProviderEvaluates`

### Requirement: Consumer survives fetch errors

`consume.Consumer.Run` SHALL log fetch errors and keep polling. It SHALL NOT return on them.

#### Scenario: Transient fetch error does not stop the loop
- **WHEN** a fetch returns an error and later records arrive
- **THEN** the later records are handled and `Run` has not returned
- **VERIFIED BY**: `platform-core/packages/go-sdk/pkg/consume` `TestFetchErrorDoesNotStopLoop`

### Requirement: Consumer clients accept extra options

`kafkax.NewConsumerClient(c, group, topics, opts...)` SHALL accept extra `kgo.Opt` values, applied after the defaults.

#### Scenario: Auto topic creation can be enabled for a consumer
- **WHEN** a consumer client is built with `kgo.AllowAutoTopicCreation()` for a topic that does not exist yet
- **THEN** it consumes records produced later, without failing on the unknown topic
- **VERIFIED BY**: `platform-core/packages/go-sdk/pkg/kafkax` `TestConsumerClientAcceptsOptions`
