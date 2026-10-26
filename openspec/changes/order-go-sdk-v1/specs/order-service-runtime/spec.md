## ADDED Requirements

### Requirement: team-order is built on the tagged go-sdk with the ADR-0016 layout

team-order SHALL require `github.com/buidangphuc/agora/platform-core/packages/go-sdk` at `v1.0.2` in `go.mod` with no `replace` directive and no `third_party` directory. Its `Makefile` SHALL include `../platform-core/make/go-service.mk` and lint with `../platform-core/packages/go-sdk/.golangci.yml`, and the service SHALL hold no copy of either file. `golangci-lint` SHALL be pinned with a `tool` directive in `go.mod`. The binaries SHALL be `cmd/server/main.go`, which loads settings and calls `internal/app.Run(ctx, settings)`, and `cmd/resync-commits/main.go`. The packages `internal/bootstrap`, `internal/featureflags`, `internal/interceptor`, `internal/grpcserver`, `internal/handler`, `internal/service`, `internal/repository` and `internal/consumer` SHALL NOT exist.

#### Scenario: go.mod requires the tag and nothing local
- **WHEN** the module file of team-order is parsed
- **THEN** it has exactly one `require` of the SDK module at `v1.0.2`, zero `replace` directives and a `tool github.com/golangci/golangci-lint/v2/cmd/golangci-lint` line
- **VERIFIED BY**: team-order `internal/app` `TestGoModRequiresTaggedSDK`

#### Scenario: Shared make and lint files are referenced without a local copy
- **WHEN** the `Makefile` and the service directory are inspected
- **THEN** the `Makefile` includes `../platform-core/make/go-service.mk`, sets `LINT_CONFIG` to `../platform-core/packages/go-sdk/.golangci.yml`, and neither `.golangci.yml`, `make/go-service.mk` nor `third_party` exists in `team-order`
- **VERIFIED BY**: team-order `internal/app` `TestSharedMakeAndLintAreReferenced`

#### Scenario: Hand-rolled infrastructure packages are gone
- **WHEN** the directories under `team-order/internal` are listed
- **THEN** `bootstrap`, `featureflags`, `interceptor`, `grpcserver`, `handler`, `service`, `repository` and `consumer` are absent, and `app`, `config`, `rpc`, `order`, `store`, `events` and `upstream` are present
- **VERIFIED BY**: team-order `internal/app` `TestLayoutHasNoLegacyPackages`

### Requirement: Settings have no dependency flags and refuse unsafe values

`config.Load()` SHALL bind Settings with `sdkconfig.Load`. `ENV` SHALL be required with no default. The declared env keys SHALL NOT include `DATABASE_ENABLED`, `KAFKA_ENABLED`, `OUTBOX_ENABLED`, `FEATURE_FLAGS_ENABLED`, `SHUTDOWN_GRACE_SECONDS`, `UPSTREAM_CALL_TIMEOUT_SECONDS`, `OUTBOX_CLAIM_LOCK_SECONDS`, `OUTBOX_MAX_ATTEMPTS`, `FEATURE_FLAGS_EVAL_TIMEOUT_MS` or `MOCK_DATA`. `DATABASE_URL` SHALL be required in every ENV. `GRPC_REFLECTION_ENABLED` SHALL default to false and SHALL be refused when ENV is not `local` or `test`. `UPSTREAM_AUDIT_ADDR`, `UPSTREAM_DOMAIN_ADDR`, `UPSTREAM_IDENTITY_ADDR` and `UPSTREAM_PROMOTION_ADDR` SHALL be required in every ENV with no default and no fallback mode: an upstream the service uses is always dialled. When ENV is a strict environment (staging, stage, prod, production) `KAFKA_BROKERS` and `LOG_KEY_FP_SECRET` SHALL also be required; in a local ENV they SHALL default to `localhost:9092` and the documented local secret. The reservation and lifecycle duration keys SHALL keep their existing validation and fallback rules. `.env.example` SHALL document every declared key.

#### Scenario: A missing ENV refuses to start
- **WHEN** `config.Load()` runs with `ENV` unset and every other required key set
- **THEN** it returns an error that names `ENV`
- **VERIFIED BY**: team-order `internal/config` `TestLoadRefusesMissingEnv`

#### Scenario: An empty database URL refuses to start in every environment
- **WHEN** `config.Load()` runs with `ENV=local` and `DATABASE_URL` empty
- **THEN** it returns an error that names `DATABASE_URL`
- **VERIFIED BY**: team-order `internal/config` `TestLoadRefusesEmptyDatabaseURL`

#### Scenario: Removed keys are not declared
- **WHEN** the env keys declared by `Settings` are listed
- **THEN** none of `DATABASE_ENABLED`, `KAFKA_ENABLED`, `OUTBOX_ENABLED`, `FEATURE_FLAGS_ENABLED`, `SHUTDOWN_GRACE_SECONDS`, `UPSTREAM_CALL_TIMEOUT_SECONDS`, `OUTBOX_CLAIM_LOCK_SECONDS`, `OUTBOX_MAX_ATTEMPTS`, `FEATURE_FLAGS_EVAL_TIMEOUT_MS`, `MOCK_DATA` is in the list, and `OTEL_ENABLED` and `GRPC_REFLECTION_ENABLED` are
- **VERIFIED BY**: team-order `internal/config` `TestRemovedKeysAreNotDeclared`

#### Scenario: Reflection is off by default
- **WHEN** `config.Load()` runs with `ENV=local` and `GRPC_REFLECTION_ENABLED` unset
- **THEN** `Settings.ReflectionEnabled` is false and the started server answers a reflection request with `Unimplemented`
- **VERIFIED BY**: team-order `internal/app` `TestReflectionOffByDefault`

#### Scenario: Reflection is refused in a strict environment
- **WHEN** `config.Load()` runs with `ENV=production` and `GRPC_REFLECTION_ENABLED=true`
- **THEN** it returns an error that names `GRPC_REFLECTION_ENABLED`
- **VERIFIED BY**: team-order `internal/config` `TestReflectionRefusedInStrictEnv`

#### Scenario: A strict environment requires brokers and key secret
- **WHEN** `config.Load()` runs with `ENV=staging`, `DATABASE_URL` and the four upstream addresses set, and `KAFKA_BROKERS` and `LOG_KEY_FP_SECRET` unset
- **THEN** it returns one error naming both keys
- **VERIFIED BY**: team-order `internal/config` `TestStrictEnvRequiresBrokersAndKeySecret`

#### Scenario: A missing upstream address refuses to start in every environment
- **WHEN** `config.Load()` runs with `ENV=local`, `DATABASE_URL` set, and `UPSTREAM_AUDIT_ADDR`, `UPSTREAM_DOMAIN_ADDR`, `UPSTREAM_IDENTITY_ADDR` and `UPSTREAM_PROMOTION_ADDR` unset
- **THEN** it returns one error naming all four keys
- **VERIFIED BY**: team-order `internal/config` `TestUpstreamAddressesRequiredInEveryEnv`

#### Scenario: A local environment fills the documented defaults
- **WHEN** `config.Load()` runs with `ENV=local`, `GRPC_PORT`, `DATABASE_URL` and the four upstream addresses set and nothing else
- **THEN** it succeeds with brokers `localhost:9092` and `LogKeyFP.Secret` equal to the documented local-only value `local-dev-key-fp-secret` (the same value as `.env.example`; with `ENV=staging` the same unset keys are refused by the strict-environment scenario)
- **VERIFIED BY**: team-order `internal/config` `TestLocalEnvDefaults`

#### Scenario: A malformed receipt duration still refuses to start and names its key
- **WHEN** `config.Load()` runs with `AUTO_COMPLETE_AFTER=banana`, and again with `RETURN_WINDOW=0s`
- **THEN** each call returns an error naming `AUTO_COMPLETE_AFTER` and `RETURN_WINDOW` respectively
- **VERIFIED BY**: team-order `internal/config` `TestLifecycleDurationMalformedRejected`

#### Scenario: An unusable reservation duration still falls back with a warning
- **WHEN** `config.Load()` runs with `RESERVATION_TTL=banana` and `RESERVATION_SWEEP_INTERVAL=0s`
- **THEN** it succeeds, `ReservationTTL()` returns 15m and `ReservationSweepInterval()` returns 1m, and each result carries a warning naming its variable
- **VERIFIED BY**: team-order `internal/config` `TestReservationDurations`

#### Scenario: .env.example documents every key
- **WHEN** the keys declared by `Settings` are compared with the keys in `.env.example`
- **THEN** every declared key appears in `.env.example` and no removed key does
- **VERIFIED BY**: team-order `internal/config` `TestEnvExampleInSync`

### Requirement: Importing a dependency means using it

team-order SHALL NOT contain a runtime in-memory repository, a noop publisher or a nil-dependency mode. `app.Run` SHALL fail to start with a non-zero exit when Postgres is unreachable, in every ENV. `order.NewOrderService`, `order.NewCartService` and `rpc.NewOrderHandler` SHALL return an error when the saga repository, the listing client, the address client, the promotion clients, the audit gate or the flag evaluator is nil, and `events.NewDisputeHandler` SHALL return an error when the audit gate is nil. Non-test code SHALL NOT import `internal/store/fake`, and no non-test file under `cmd` or `internal` SHALL define `RequireDurableStorage`, `NoopPublisher` or call a `NewInMemory*` constructor.

#### Scenario: An unreachable Postgres stops the process
- **WHEN** `app.Run` starts with `ENV=local` and a `DATABASE_URL` that points at a closed port
- **THEN** it returns an error that names the database within 15 seconds and no gRPC port is opened
- **VERIFIED BY**: team-order `internal/app` `TestRunFailsWhenPostgresUnreachable`

#### Scenario: A nil promotion client is refused at construction
- **WHEN** `order.NewOrderService` is called with a nil promotion voucher client, and again with a nil flash client
- **THEN** each call returns a non-nil error naming the missing client and no service is built
- **VERIFIED BY**: team-order `internal/order` `TestNewOrderServiceRefusesNilDependencies`

#### Scenario: A nil flag evaluator is refused at construction
- **WHEN** `rpc.NewOrderHandler` is called with a nil flag evaluator
- **THEN** it returns a non-nil error naming the evaluator
- **VERIFIED BY**: team-order `internal/rpc` `TestNewOrderHandlerRefusesNilDependencies`

#### Scenario: A nil audit gate is refused at construction
- **WHEN** `rpc.NewOrderHandler` is called with a nil audit gate, and `events.NewDisputeHandler` is called with a nil audit gate
- **THEN** each call returns a non-nil error naming the gate
- **VERIFIED BY**: team-order `internal/rpc` `TestNewOrderHandlerRefusesNilDependencies`; team-order `internal/events` `TestNewDisputeHandlerRefusesNilGate`

#### Scenario: No runtime fallback exists in non-test code
- **WHEN** the Go files under `cmd` and `internal` that are not `_test.go` and not under `internal/store/fake` are scanned
- **THEN** none contains `NewInMemory`, `NoopPublisher`, `RequireDurableStorage` or an import of `internal/store/fake`
- **VERIFIED BY**: team-order `internal/app` `TestNoRuntimeInMemoryFallback`

### Requirement: Readiness follows Postgres and liveness does not

team-order SHALL register the health check `postgres` with `healthx` and no Kafka check: Kafka is not a readiness condition. The SDK has no upstream readiness checker, so no upstream check is registered; a required upstream is covered by its own error handling per call. The overall health status (service name empty) SHALL be NOT_SERVING while the `postgres` check fails. The health service `liveness` SHALL be SERVING from construction, SHALL stay SERVING while a check fails and SHALL turn NOT_SERVING when shutdown starts draining.

#### Scenario: A failing Postgres check makes readiness fail and keeps liveness serving
- **WHEN** the `postgres` check fails for one probe interval
- **THEN** the overall status is NOT_SERVING and the `liveness` service is SERVING
- **VERIFIED BY**: team-order `internal/app` `TestLivenessSurvivesDependencyOutage`

#### Scenario: A passing Postgres check makes readiness serve
- **WHEN** the `postgres` check passes for one probe interval
- **THEN** the overall status and the `liveness` service are both SERVING
- **VERIFIED BY**: team-order `internal/app` `TestReadinessServesWhenChecksPass`

#### Scenario: A Kafka outage does not affect readiness
- **WHEN** the Kafka brokers are unreachable while the `postgres` check passes for one probe interval
- **THEN** the overall status is SERVING and the set of registered checks is exactly `postgres`
- **VERIFIED BY**: team-order `internal/app` `TestKafkaOutageDoesNotAffectReadiness`

#### Scenario: Shutdown drains liveness before the server stops
- **WHEN** the process receives a termination signal
- **THEN** the `liveness` service reports NOT_SERVING before the gRPC server stops accepting calls
- **VERIFIED BY**: team-order `internal/app` `TestShutdownDrainsHealthFirst`

### Requirement: The checkout kill-switch is evaluated through the SDK flag client

`CreateOrder` SHALL evaluate the boolean flag `checkout-enabled` with default true through the SDK `flags.Evaluator` before any other work. When the flag is off the call SHALL fail with `FailedPrecondition` and message "checkout is temporarily unavailable" and SHALL start no saga. When the flag provider errors or Flipt is unreachable the default SHALL apply and checkout SHALL proceed. An empty `FLIPT_ADDR` SHALL refuse startup. The flag client SHALL be closed during shutdown.

#### Scenario: Kill-switch off blocks CreateOrder
- **WHEN** `checkout-enabled` evaluates false and a buyer calls `CreateOrder`
- **THEN** the call fails `FailedPrecondition` with the message "checkout is temporarily unavailable" and zero reservations exist
- **VERIFIED BY**: team-order `internal/rpc` `TestOrderHandler_CreateOrder_KillSwitchOff`

#### Scenario: Kill-switch on allows CreateOrder
- **WHEN** `checkout-enabled` evaluates true and a buyer calls `CreateOrder` with a valid cart
- **THEN** the call succeeds and one order exists for the buyer
- **VERIFIED BY**: team-order `internal/rpc` `TestOrderHandler_CreateOrder_KillSwitchOn`

#### Scenario: A flag outage fails open
- **WHEN** the flag provider returns an error for `checkout-enabled` and a buyer calls `CreateOrder` with a valid cart
- **THEN** the call succeeds and one order exists for the buyer
- **VERIFIED BY**: team-order `internal/rpc` `TestOrderHandler_CreateOrder_FlagProviderErrorFailsOpen`

#### Scenario: An empty Flipt address refuses startup
- **WHEN** `app.Run` starts with `FLIPT_ADDR` set to the empty string
- **THEN** it returns an error that names `FLIPT_ADDR`
- **VERIFIED BY**: team-order `internal/app` `TestRunRefusesEmptyFliptAddr`

### Requirement: Errors map through one gRPC error table

Every RPC handler SHALL return errors through `grpcx.Status` with one service `ErrorTable`. A Postgres unique violation (SQLSTATE 23505) SHALL map to `AlreadyExists`, `context.Canceled` to `Canceled` and `context.DeadlineExceeded` to `DeadlineExceeded`. An error with no table entry SHALL return `Internal` with the message "internal error" and SHALL NOT contain driver or upstream error text. A request without a principal or with an anonymous principal SHALL fail `Unauthenticated` on every RPC except `CalculateShippingFee` and `GetShipmentTracking`. The message texts that existing scenarios pin for client-visible failures SHALL be unchanged. The audit write bound is the fixed SDK 2 s; it is no longer configurable (the former `UPSTREAM_CALL_TIMEOUT_SECONDS` is removed).

#### Scenario: A unique violation maps to AlreadyExists
- **WHEN** a store call fails with a Postgres error of SQLSTATE 23505
- **THEN** the handler returns `AlreadyExists`
- **VERIFIED BY**: team-order `internal/rpc` `TestUniqueViolationMapsToAlreadyExists`

#### Scenario: Context errors keep their gRPC codes
- **WHEN** a store call returns `context.Canceled`, and another returns `context.DeadlineExceeded`
- **THEN** the handler returns `Canceled` and `DeadlineExceeded` respectively
- **VERIFIED BY**: team-order `internal/rpc` `TestContextErrorsKeepGRPCCodes`

#### Scenario: An unexpected error does not leak its cause
- **WHEN** a store call fails with the error text "pq: connection to 10.0.0.9 refused"
- **THEN** the handler returns `Internal` with the message "internal error" and the message does not contain "10.0.0.9"
- **VERIFIED BY**: team-order `internal/rpc` `TestHandlerInternalErrorsAreGenericAndDoNotLeakCause`

#### Scenario: An anonymous principal is rejected
- **WHEN** `GetCart` is called with principal id `anonymous`, and `ListBuyerOrders` is called with no principal
- **THEN** both calls fail `Unauthenticated`
- **VERIFIED BY**: team-order `internal/rpc` `TestAnonymousPrincipalUnauthenticated`

#### Scenario: A buyer cannot read another buyer's order
- **WHEN** buyer B calls `GetOrder` for an order between buyer A and seller S
- **THEN** the call fails `PermissionDenied`
- **VERIFIED BY**: team-order `internal/rpc` `TestGetOrderStrangerPermissionDenied`

### Requirement: CreateOrder validates the idempotency key with the SDK rule

`CreateOrder` SHALL read the optional `idempotency-key` metadata, trim surrounding whitespace and validate the result with `idempotency.Validate`: 1 to 255 printable ASCII bytes. An invalid key SHALL fail `InvalidArgument` before any reservation. A valid key SHALL keep today's replay behaviour: a second call with the same buyer and key returns the first call's orders.

#### Scenario: An oversized key is rejected
- **WHEN** a buyer calls `CreateOrder` with an `idempotency-key` of 256 bytes
- **THEN** the call fails `InvalidArgument` and zero reservations exist
- **VERIFIED BY**: team-order `internal/rpc` `TestCreateOrderOversizedIdempotencyKeyInvalidArgument`

#### Scenario: A non-ASCII key is rejected
- **WHEN** a buyer calls `CreateOrder` with an `idempotency-key` that contains the character U+00E9
- **THEN** the call fails `InvalidArgument`
- **VERIFIED BY**: team-order `internal/order` `TestNormalizeIdempotencyKeyRejectsNonASCII`

#### Scenario: A padded key is trimmed and replays the first checkout
- **WHEN** a buyer calls `CreateOrder` with key "k-1" and then again with key "  k-1  " for the same cart
- **THEN** both calls return the same order ids and exactly one order exists
- **VERIFIED BY**: team-order `internal/rpc` `TestCreateOrderPaddedIdempotencyKeyReplays`

#### Scenario: Another buyer's key does not replay their checkout
- **WHEN** buyer A completes a checkout with key "k-1" and buyer B then calls `CreateOrder` with key "k-1" for B's own cart
- **THEN** B receives B's own new orders and none of A's order ids
- **VERIFIED BY**: team-order `internal/order` `TestIdempotencyKeyIsScopedToBuyer`

### Requirement: Upstream calls carry the least-privilege principal chosen per call

team-order SHALL create its domain, identity and promotion connections with the SDK `upstream.NewClient`. It SHALL send `ReserveStock`, `CommitReservation` and `ReleaseStock` of `ListingService` as the service principal `service-team-order` with exactly the scope `listing.write`, and `ValidateAndReserve`, `CommitReservation`, `ReleaseReservation`, `ReserveFlashSaleStock`, `CommitFlashSaleStock` and `ReleaseFlashSaleStock` of team-promotion as that principal with exactly the scope `promotion.reserve`, whether the call runs inside a buyer's request or in background compensation. A `GetListing` call with no end-user principal in its context SHALL be sent as that principal with exactly the scope `listing.read` (the previous interceptor gave every unmarked call without a caller principal that identity; now only `GetListing` runs without an end-user principal: `GetActiveFlashSale`, the identity address lookups and the preview RPCs are only called from a buyer request from a buyer request, the handlers refusing an anonymous principal, so none of them needs a synthesized identity and none receives one). Every other call SHALL forward the caller's principal unchanged and SHALL NOT add scopes. The buyer's `authorization` header SHALL NOT be forwarded on a service-principal call.

#### Scenario: Stock changes are sent as the service principal with the stock scope
- **WHEN** `ReserveStock` is called from a context that carries a buyer principal with scope `listing.read`
- **THEN** the upstream receives principal id `service-team-order`, type `service`, scopes exactly `listing.write`, and no `authorization` header
- **VERIFIED BY**: team-order `internal/upstream` `TestStockCallsCarryServicePrincipal`

#### Scenario: Voucher and flash holds are sent with the promotion scope
- **WHEN** `ValidateAndReserve` and `ReserveFlashSaleStock` are called from a context that carries a buyer principal
- **THEN** the upstream receives principal id `service-team-order`, type `service` and scopes exactly `promotion.reserve` on both calls
- **VERIFIED BY**: team-order `internal/upstream` `TestPromotionCallsCarryPromotionScope`

#### Scenario: A buyer read forwards the buyer principal unchanged
- **WHEN** `GetListing` and `GetActiveFlashSale` are called from a context that carries buyer principal `u-7` with scope `listing.read`
- **THEN** the upstream receives principal id `u-7`, type `user` and scopes exactly `listing.read` on both calls
- **VERIFIED BY**: team-order `internal/upstream` `TestBuyerReadsForwardBuyerPrincipal`

#### Scenario: A service-principal call does not forward the buyer token
- **WHEN** `CommitReservation` is called from a context that carries buyer principal `u-7` and an `authorization` header
- **THEN** the upstream receives no `authorization` header and no principal id `u-7`
- **VERIFIED BY**: team-order `internal/upstream` `TestServiceCallDoesNotForwardBuyerToken`

#### Scenario: A background read gets the read-only service scope
- **WHEN** `GetListing` is called from a context with no principal
- **THEN** the upstream receives principal id `service-team-order`, type `service` and scopes exactly `listing.read`
- **VERIFIED BY**: team-order `internal/upstream` `TestBackgroundReadGetsReadOnlyScope`

#### Scenario: Only GetListing gets a synthesized background identity
- **WHEN** `GetActiveFlashSale` and `ListAddresses` are called from a context with no principal
- **THEN** the upstream receives no principal id on both calls (nothing is synthesized), while `GetListing` from the same context receives `service-team-order`
- **VERIFIED BY**: team-order `internal/upstream` `TestOnlyGetListingGetsBackgroundIdentity`

### Requirement: Admin actions and the dispute refund are audited first through the SDK gate

team-order SHALL write the audit event of `ForceFailSaga`, admin `UpdateOrderStatus`, admin `UpdateReturnStatus`, admin `CreateShipment` and of a dispute-driven return change through `auditx.Gate.Do` before the change. When the audit write fails or times out after 2 seconds, the RPC SHALL return `Unavailable` with the message `audit trail unavailable`, and the order, return or shipment SHALL be unchanged. The action's own error SHALL be returned unchanged when the audit write succeeds. Seller-path calls SHALL NOT write audit events and SHALL work while audit is down. The audit event actions, target types and metadata keys of the existing requirements SHALL be unchanged.

#### Scenario: Admin force-fail with audit down is refused and changes nothing
- **WHEN** team-audit returns `Unavailable` and an admin holding `admin` and `order.admin` calls `ForceFailSaga` on a Pending order
- **THEN** the call fails `Unavailable` with the message "audit trail unavailable" and the order reads Pending
- **VERIFIED BY**: team-order `internal/rpc` `TestForceFailSagaAuditDownUnavailableNoChange`

#### Scenario: An audit write that hangs is bounded
- **WHEN** team-audit does not answer and an admin calls `CreateShipment`
- **THEN** the call fails `Unavailable` within 3 seconds and no shipment exists
- **VERIFIED BY**: team-order `internal/rpc` `TestCreateShipmentAuditTimeoutBounded`

#### Scenario: A refused transition keeps its audit row and its own error
- **WHEN** team-audit answers and an admin calls `UpdateOrderStatus` for a transition the table forbids
- **THEN** the call fails `FailedPrecondition` and exactly one audit event exists for the order
- **VERIFIED BY**: team-order `internal/rpc` `TestUpdateOrderStatusAdminInvalidTransitionAfterAudit`

#### Scenario: A buyer cannot trigger an admin action
- **WHEN** a buyer calls `ForceFailSaga` on the buyer's own order
- **THEN** the call fails `PermissionDenied` (permission denied for a non-admin scope) and zero audit calls are made
- **VERIFIED BY**: team-order `internal/rpc` `TestForceFailSagaDeniedWritesNoAudit`

#### Scenario: A seller is not audited and works while audit is down
- **WHEN** team-audit returns `Unavailable` and the seller of a Paid order calls `UpdateOrderStatus` to Shipped
- **THEN** the call succeeds, the order reads Shipped and zero audit calls were made
- **VERIFIED BY**: team-order `internal/rpc` `TestUpdateOrderStatusSellerNotAudited`

### Requirement: The order.events outbox is written in the transition's transaction and relayed by the SDK relayer

Every first transition to PAID, every won claim to CANCELLED, every placement, payment reminder, completion, return request, return refund and shipment SHALL write its order event with `outbox.Store.EnqueueTx` inside the transition's own transaction, so a failed enqueue rolls the transition back. The `outbox.Relayer` SHALL publish pending rows to `ORDER_EVENTS_TOPIC` keyed by the order id with `seq` order per order, and SHALL run in every ENV. Event ids and payload bytes SHALL be unchanged. Migrations `0012` to `0021` SHALL upgrade `order_outbox_events` to the SDK shape without reordering pending rows and without a write lock beyond step `0014`, and migration `0022` SHALL requeue rows parked as `failed` by the old relayer so the SDK relayer publishes them. Published rows SHALL be purged after 7 days by `Store.Purge` run on a schedule inside the service. The first rollout SHALL scale the old pods to 0 before any new pod starts (a one-time Recreate), so two relayers never run together.

#### Scenario: A paid transition writes its outbox row in the same transaction
- **WHEN** a PENDING order is moved to PAID
- **THEN** the order reads PAID and exactly one `order_outbox_events` row of type `platform.order.v1.OrderPaidEvent` exists for it
- **VERIFIED BY**: team-order `internal/store` `TestPaidTransitionWritesOutboxRow`

#### Scenario: A failing enqueue rolls the transition back
- **WHEN** the outbox insert fails during the move of a PENDING order to PAID
- **THEN** the move returns an error, the order still reads PENDING and zero outbox rows exist for it
- **VERIFIED BY**: team-order `internal/store` `TestPaidTransitionRollsBackWhenOutboxFails`

#### Scenario: The relayer publishes keyed by order id
- **WHEN** one `OrderPaidEvent` row is pending and the relayer runs one pass against a recording publisher
- **THEN** exactly one record is published to `order.events` with key equal to the order id and the row reads `published`
- **VERIFIED BY**: team-order `internal/events` `TestRelayerPublishesKeyedByOrderID`

#### Scenario: A pending row is claimed within one poll interval
- **WHEN** the relayer runs with `OUTBOX_POLL_INTERVAL` 1 s against a recording publisher and a row is enqueued while the relayer is idle
- **THEN** the record is published within 2 seconds of the enqueue
- **VERIFIED BY**: team-order `internal/events` `TestRelayerPublishesWithinPollInterval`

#### Scenario: Events of one order are published in seq order
- **WHEN** an order has pending rows `OrderPlaced`, `OrderPaidEvent`, `OrderShipped` enqueued in that order and the relayer runs
- **THEN** the recording publisher receives them in that order
- **VERIFIED BY**: team-order `internal/events` `TestRelayerKeepsPerOrderSequence`

#### Scenario: The outbox upgrade keeps pending events in order
- **WHEN** 5 pending rows written under the old table shape are upgraded with migrations `0012` to `0022` and the relayer then runs
- **THEN** they are published in `(created_at, event_id)` order and each keeps its `event_id` and payload bytes
- **VERIFIED BY**: team-order `internal/store` `TestOutboxUpgradeKeepsPendingOrder`

#### Scenario: The upgrade requeues rows parked as failed
- **WHEN** 2 rows with status `failed` and 1 row with status `published` written under the old table shape are upgraded with migrations `0012` to `0022` and the relayer then runs
- **THEN** the 2 formerly failed rows are published once each in `seq` order, and the published row is not published again
- **VERIFIED BY**: team-order `internal/store` `TestOutboxUpgradeRequeuesFailedRows`

#### Scenario: Published rows older than 7 days are purged
- **WHEN** the purge component runs while the table holds a published row aged 8 days, a published row aged 6 days and a pending row aged 8 days
- **THEN** only the 8-day-old published row is deleted
- **VERIFIED BY**: team-order `internal/events` `TestOutboxPurgeDeletesOnlyOldPublishedRows`

#### Scenario: The event envelope contract is unchanged
- **WHEN** the paid envelope is built for an order with two line items
- **THEN** it carries the same event type, `event_id`, aggregate id and line items as before the migration
- **VERIFIED BY**: team-order `internal/events` `TestPaidEnvelopeMatchesContract`

### Requirement: The payment consumer runs on the SDK consume loop

The `payment.events` consumer SHALL run on `consume.Consumer` with `kafkax.NewConsumerClient`, group `ORDER_PAYMENT_CONSUMER_GROUP` and DLQ topic `PAYMENT_EVENTS_DLQ_TOPIC`. A `PaymentSettled` event SHALL move the order to PAID once; a redelivered event id SHALL be a no-op through `consume.Dedupe` over `processed_events`, recorded after the effect. A transient error SHALL be retried up to 5 attempts and then parked on the DLQ with the original key. An error wrapping `ErrPermanent` SHALL be parked at once. Log records about a consumed record SHALL carry `key_fp` and SHALL NOT carry the raw key.

#### Scenario: A settled payment moves the order to PAID once
- **WHEN** the same `PaymentSettled` event is delivered twice for a PENDING order
- **THEN** the order reads PAID, exactly one `OrderPaidEvent` outbox row exists and exactly one `processed_events` row exists for consumer `team-order.payment`
- **VERIFIED BY**: team-order `internal/events` `TestPaymentRedeliveryAppliesOnce`

#### Scenario: A transient failure is retried and then parked
- **WHEN** the order store fails with a transport error on every attempt for one record keyed "order-77"
- **THEN** the handler runs 5 times, the DLQ topic `payment.events.dlq` receives one record with key bytes "order-77", and the offset is committed once
- **VERIFIED BY**: team-order `internal/events` `TestPaymentTransientFailureParksAfterFiveAttempts`

#### Scenario: A poison record is parked at once
- **WHEN** a record whose value is not a valid envelope reaches the consumer
- **THEN** the handler runs once and the DLQ receives the record unchanged
- **VERIFIED BY**: team-order `internal/events` `TestPaymentPoisonRecordParkedAtOnce`

#### Scenario: The offset is not committed when the DLQ write fails (payment)
- **WHEN** a payment record fails permanently and the DLQ producer returns an error
- **THEN** the offset of that record is not committed and the same record is delivered again
- **VERIFIED BY**: team-order `internal/events` `TestPaymentNoCommitWhenDLQFails`

#### Scenario: Consumer logs carry a key fingerprint and not the key
- **WHEN** a record keyed "order-77" fails its handler and is parked
- **THEN** every log record about it has attribute `key_fp` of 12 hex characters and no log attribute or message contains "order-77"
- **VERIFIED BY**: team-order `internal/events` `TestPaymentConsumerLogsKeyFingerprintOnly`

#### Scenario: The same event id is independent per consumer
- **WHEN** event id "e-1" is recorded for consumer `team-order.payment` and then checked for consumer `team-order.dispute`
- **THEN** the second check reports not processed
- **VERIFIED BY**: team-order `internal/store` `TestProcessedEventsKeyIsPerConsumer`

#### Scenario: Group ids and DLQ names are unchanged by default
- **WHEN** `config.Load()` runs with `ENV=local` and no consumer keys set
- **THEN** the payment group is `team-order.payment`, the payment DLQ is `payment.events.dlq`, the dispute group is `team-order.dispute` and the dispute DLQ is `engagement.events.order.dlq`
- **VERIFIED BY**: team-order `internal/config` `TestKafkaEngagementConsumerSettings`

### Requirement: The dispute consumer retries transient errors until applied on the SDK consume loop

The `engagement.events` dispute consumer SHALL run on `consume.Consumer` with `MaxAttempts` set to `math.MaxInt32`, `MaxBackoff` 5 seconds and `MaxPollRecords` 1, and SHALL start in every ENV (the audit connection is always configured). A transient error, including an audit outage, SHALL be retried for the same record with a per-attempt delay of at most 5 seconds, SHALL NOT commit the offset meanwhile and SHALL NOT produce to the DLQ. An error wrapping `ErrPermanent` SHALL be parked on `ENGAGEMENT_EVENTS_DLQ_TOPIC` at once. The `DisputeConsumerStartDecision` function SHALL NOT exist. During a team-audit outage the consumer stalls its partition on the record being applied until audit answers (accepted behaviour; `payment.events` has its own client and group and is unaffected).

#### Scenario: An audit outage is retried without dead-lettering
- **WHEN** team-audit fails the first 12 audit writes for a BUYER_FAVOUR `DisputeResolved` event and then answers
- **THEN** zero records are produced to the DLQ, the offset is committed exactly once after the successful attempt, every retry delay is at most 5 seconds, and the return ends REFUNDED
- **VERIFIED BY**: team-order `internal/events` `TestDisputeRunRetriesTransientWithoutDLQ`

#### Scenario: A return that no longer fits the order is parked at once
- **WHEN** a BUYER_FAVOUR event names a REJECTED return whose refund would exceed the order remainder
- **THEN** the handler runs once, the return stays REJECTED, and the DLQ receives the record
- **VERIFIED BY**: team-order `internal/events` `TestDisputeRefundNoRemainderDeadLettered`

#### Scenario: The offset is not committed when the DLQ write fails (dispute)
- **WHEN** a dispute record fails permanently and the DLQ producer returns an error
- **THEN** the offset of that record is not committed and the same record is delivered again
- **VERIFIED BY**: team-order `internal/events` `TestDisputeNoCommitWhenDLQFails`

#### Scenario: A redelivered dispute event refunds once
- **WHEN** the same BUYER_FAVOUR event id is delivered twice for an APPROVED return of a paid order
- **THEN** the return reads REFUNDED, exactly one `ReturnRefunded` outbox row exists and one `processed_events` row exists for consumer `team-order.dispute`
- **VERIFIED BY**: team-order `internal/events` `TestDisputeRedeliveryRefundsOnce`

#### Scenario: The dispute audit event names the resolving admin
- **WHEN** a BUYER_FAVOUR event with resolver id "admin-9" changes a PENDING return
- **THEN** exactly one audit event exists with action `return.dispute_refund`, actor id "admin-9", target type `return_request` and the return id, written before the return changes
- **VERIFIED BY**: team-order `internal/events` `TestDisputeRefundWritesAuditBeforeChange`

#### Scenario: No start-decision function remains
- **WHEN** the exported identifiers of `internal/app` and `internal/events` are listed
- **THEN** `DisputeConsumerStartDecision` is not among them
- **VERIFIED BY**: team-order `internal/app` `TestNoDisputeStartDecision`

### Requirement: Stock release in compensation and cancel is retried only on transient codes

The idempotent `ReleaseStock` call of compensation, buyer cancel and the sweeps SHALL run under `resilience.Do` with `resilience.Retry`, at most 3 attempts by default (unchanged), the base delay `releaseCfg.backoff` (100 ms by default, unchanged) growing by the SDK backoff (was `backoff x attempt`), and retrying only `Unavailable` and `DeadlineExceeded` (was: every error). A non-retryable code (for example `FailedPrecondition`) SHALL be attempted once (was: 3 times). `WithReleaseRetry(timeout, maxAttempts, backoff)`, the per-release timeout (30 s default) and the 2 s inline-release budget of a buyer cancel SHALL survive unchanged, and the retry bounds SHALL come from them. When every attempt fails the reservation SHALL be parked `RELEASE_FAILED` for the TTL sweep, as before. `ReleaseFlashSaleStock` and `ReleaseReservation` (flash and voucher holds) SHALL stay best-effort: a failed release is logged and never fails the cancel or the compensation, and no `RELEASE_FAILED` parking exists for them, as before.

#### Scenario: A release succeeds on the second attempt
- **WHEN** `ReleaseStock` returns `Unavailable` on the first call and succeeds on the second during compensation
- **THEN** the reservation is RELEASED and `ReleaseStock` was called exactly 2 times
- **VERIFIED BY**: team-order `internal/order` `TestCompensationReleaseRetriesUnavailableOnce`

#### Scenario: Three failed attempts park the reservation
- **WHEN** `ReleaseStock` returns `Unavailable` on every call with the default retry bounds
- **THEN** the reservation is RELEASE_FAILED and `ReleaseStock` was called exactly 3 times
- **VERIFIED BY**: team-order `internal/order` `TestCompensationReleaseParksAfterThreeFailures`

#### Scenario: A non-retryable code is attempted once
- **WHEN** `ReleaseStock` returns `FailedPrecondition`
- **THEN** `ReleaseStock` was called exactly 1 time (the previous code called it 3 times) and the reservation is RELEASE_FAILED
- **VERIFIED BY**: team-order `internal/order` `TestCompensationReleaseNonRetryableOnce`

#### Scenario: WithReleaseRetry still bounds the attempts
- **WHEN** the service is built with `WithReleaseRetry(time.Second, 2, time.Millisecond)` and `ReleaseStock` returns `Unavailable` on every call
- **THEN** `ReleaseStock` was called exactly 2 times and the reservation is RELEASE_FAILED
- **VERIFIED BY**: team-order `internal/order` `TestWithReleaseRetryBoundsAttempts`

#### Scenario: A buyer cancel stays within the inline release budget
- **WHEN** a buyer cancels a PENDING order while `ReleaseStock` blocks until its context ends
- **THEN** `CancelOrder` returns success within 3 seconds, the order reads CANCELLED and the reservation is RELEASE_FAILED for the sweep
- **VERIFIED BY**: team-order `internal/order` `TestCancelInlineReleaseBudgetParksSlowRelease`

#### Scenario: A failed flash or voucher release never fails the cancel
- **WHEN** a buyer cancels an order with a flash hold and a voucher hold while `ReleaseFlashSaleStock` and `ReleaseReservation` return `Unavailable`
- **THEN** `CancelOrder` returns success, the order reads CANCELLED and each failure is logged once
- **VERIFIED BY**: team-order `internal/order` `TestCancelFlashAndVoucherReleaseFailureIsLoggedOnly`

### Requirement: Sweeps run as lifecycle components and stop with the server

The reservation sweep and the lifecycle sweep (payment reminders and unpaid-order expiry) SHALL run as `lifecycle.Every` components in every ENV, at `RESERVATION_SWEEP_INTERVAL` and `LIFECYCLE_SWEEP_INTERVAL`, and the outbox purge SHALL run as a third `lifecycle.Every` component every hour (fixed, not configurable). A sweep that fails SHALL be retried on the next tick. On shutdown the gRPC server SHALL stop first, then the relayer, consumers, sweeps and purge, then the Kafka clients and the pool.

#### Scenario: Each tick drives one lifecycle sweep
- **WHEN** the lifecycle component runs with a 1 ms interval against a recording sweeper for 3 ticks
- **THEN** `SweepLifecycle` is called at least 3 times with non-zero instants
- **VERIFIED BY**: team-order `internal/app` `TestRunLifecycleSweeper_TicksCallSweepLifecycle_OrderUnpaidPastDeadlineIsCancelled`

#### Scenario: A failing reservation sweep is retried on the next tick
- **WHEN** `SweepExpiredReservations` fails on the first tick and succeeds on the second
- **THEN** the component is still running after the first tick and the second tick releases the expired reservation
- **VERIFIED BY**: team-order `internal/app` `TestReservationSweepRetriesAfterFailure`

#### Scenario: Shutdown stops the server before the dependents
- **WHEN** the lifecycle receives cancellation while the relayer, consumers and sweeps run
- **THEN** the gRPC server's stop completes before the relayer, the two consumers, the two sweeps and the purge return, and the Kafka clients and the pool are closed last
- **VERIFIED BY**: team-order `internal/app` `TestShutdownStopsServerBeforeDependents`

### Requirement: The processed-events ledger is keyed per consumer

Migration `0011` SHALL replace the primary key of `processed_events` with `(consumer, event_id)` without deleting rows. The migrations `0001` to `0022` SHALL apply in order on an empty database and each `down` migration SHALL reverse its `up`.

#### Scenario: The key swap keeps existing rows
- **WHEN** 3 rows exist in `processed_events` under the old primary key and migration `0011` runs
- **THEN** the table still has 3 rows and the primary key columns are `consumer, event_id`
- **VERIFIED BY**: team-order `internal/store` `TestProcessedEventsPKMigrationKeepsRows`

#### Scenario: All migrations apply and reverse
- **WHEN** migrations `0001` to `0022` are applied to an empty database and then all `down` migrations are applied in reverse
- **THEN** every step succeeds and no table of the service remains
- **VERIFIED BY**: team-order `internal/store` `TestMigrationsApplyAndReverse`

### Requirement: Migrations run only through the migrate subcommand

The `server` binary SHALL accept the subcommand `migrate up`, which applies the migrations embedded in the binary (`migrations/embed.go`) with the SDK `migrate.Up` against `DATABASE_URL` and exits 0 on success and non-zero on any failure. Starting the service without a subcommand SHALL NOT apply any migration: boot opens Postgres and serves, and a missing schema surfaces as an error of the first query, never as an automatic migration. Local compose and gitops run `migrate up` from the same service image (deploy track).

#### Scenario: migrate up applies the embedded migrations and exits
- **WHEN** `server migrate up` runs against an empty Postgres with `ENV=local`
- **THEN** it exits 0, the tables of migrations `0001` to `0022` exist and a second run exits 0 with no change
- **VERIFIED BY**: team-order `cmd/server` `TestMigrateUpAppliesEmbeddedMigrations`

#### Scenario: migrate up against an unreachable Postgres fails
- **WHEN** `server migrate up` runs with a `DATABASE_URL` that points at a closed port
- **THEN** it exits non-zero within 15 seconds and its stderr names the database
- **VERIFIED BY**: team-order `cmd/server` `TestMigrateUpFailsWhenPostgresUnreachable`

#### Scenario: Boot never migrates
- **WHEN** `app.Run` starts against an empty Postgres with no `migrate up` run before
- **THEN** no table of the service exists afterwards (the process may report the missing schema but creates nothing)
- **VERIFIED BY**: team-order `internal/app` `TestRunDoesNotMigrate`

#### Scenario: An unknown subcommand is refused
- **WHEN** `server migrate down` runs
- **THEN** it exits non-zero with a usage message and touches no database
- **VERIFIED BY**: team-order `cmd/server` `TestUnknownSubcommandRefused`

### Requirement: The resync-commits command uses the SDK pool and upstream client

`cmd/resync-commits` SHALL load `config.Load()`, open Postgres with `postgres.Open` and call team-domain through `upstream.NewClient` with the stock service principal. It SHALL exit 1 when Postgres is unreachable, 2 when some rows need attention and 0 when every row is reconciled, as before. It SHALL NOT read `DATABASE_ENABLED`.

#### Scenario: A dry run counts rows and calls nothing
- **WHEN** the command runs with `-dry-run` against a database holding 4 COMMITTED reservations
- **THEN** it reports `scanned` 4 and `committed` 0, makes zero calls to team-domain and exits 0
- **VERIFIED BY**: team-order `internal/order` `TestResyncDryRunCountsAndCallsNothing`

#### Scenario: A row released in team-domain needs attention
- **WHEN** team-domain answers `NotFound` for one of 3 COMMITTED reservations
- **THEN** the report shows `not_found_in_domain` 1 and `NeedsAttention()` is true
- **VERIFIED BY**: team-order `internal/order` `TestResyncReportsNotFoundNeedsAttention`

#### Scenario: An unreachable Postgres exits 1 without calling team-domain
- **WHEN** the command runs and the Postgres connection fails
- **THEN** it exits 1 and makes zero calls to team-domain
- **VERIFIED BY**: team-order `internal/order` `TestResyncFailsWhenPostgresUnreachable`

#### Scenario: The command commits stock with the stock scope only
- **WHEN** the command calls `CommitReservation` for one COMMITTED reservation
- **THEN** team-domain receives principal id `service-team-order`, type `service` and scopes exactly `listing.write`
- **VERIFIED BY**: team-order `internal/order` `TestResyncCommitsAsServicePrincipal`
