## ADDED Requirements

### Requirement: team-payment is built on the tagged go-sdk with the ADR-0016 layout

team-payment SHALL require `github.com/buidangphuc/agora/platform-core/packages/go-sdk` at `v1.0.2` in `go.mod` with no `replace` directive and no `third_party` directory. Its `Makefile` SHALL include `../platform-core/make/go-service.mk` and lint with `../platform-core/packages/go-sdk/.golangci.yml`, and the service SHALL hold no copy of either file. `golangci-lint` SHALL be pinned with a `tool` directive in `go.mod`. The binary SHALL be `cmd/server/main.go`, which loads settings and calls `internal/app.Run(ctx, settings)`. The packages `internal/bootstrap`, `internal/interceptor` and `internal/grpcserver` SHALL NOT exist.

#### Scenario: go.mod requires the tag and nothing local
- **WHEN** the module file of team-payment is parsed
- **THEN** it has exactly one `require` of the SDK module at `v1.0.2`, zero `replace` directives and a `tool github.com/golangci/golangci-lint/v2/cmd/golangci-lint` line
- **VERIFIED BY**: team-payment `internal/app` `TestGoModRequiresTaggedSDK`

#### Scenario: Shared make and lint files are referenced, not copied
- **WHEN** the `Makefile` and the service directory are inspected
- **THEN** the `Makefile` includes `../platform-core/make/go-service.mk`, sets `LINT_CONFIG` to `../platform-core/packages/go-sdk/.golangci.yml`, and neither `.golangci.yml`, `make/go-service.mk` nor `third_party` exists in `team-payment`
- **VERIFIED BY**: team-payment `internal/app` `TestSharedMakeAndLintAreReferenced`

#### Scenario: A replace directive or a copied shared file fails the layout checks
- **WHEN** `go.mod` gains a `replace` directive, or a `third_party` directory or a `.golangci.yml` appears in `team-payment`
- **THEN** the layout tests fail and name the offending file or directive
- **VERIFIED BY**: team-payment `internal/app` `TestLayoutChecksRejectCopiesAndReplace`

#### Scenario: Hand-rolled infrastructure packages are gone
- **WHEN** the directories under `team-payment/internal` are listed
- **THEN** `bootstrap`, `interceptor` and `grpcserver` are absent, and `internal/app`, `internal/config`, `internal/rpc`, `internal/payment`, `internal/settlement` and `internal/store` are present
- **VERIFIED BY**: team-payment `internal/app` `TestLayoutHasNoLegacyPackages`

### Requirement: Settings have no dependency flags and refuse unsafe values

`config.Load()` SHALL bind Settings with `sdkconfig.Load`. `ENV` SHALL be required with no default. The declared env keys SHALL NOT include `DATABASE_ENABLED`, `KAFKA_ENABLED`, `PAYMENT_SETTLEMENT_CONSUMER_ENABLED`, `MOCK_PAYMENTS`, `SHUTDOWN_GRACE_SECONDS` or `UPSTREAM_CALL_TIMEOUT_SECONDS`. `DATABASE_URL`, `UPSTREAM_ORDER_ADDR` and `UPSTREAM_AUDIT_ADDR` SHALL be required in every ENV: an upstream the service uses has no fallback mode. `GRPC_REFLECTION_ENABLED` SHALL default to false and SHALL be refused when ENV is not `local` or `test`. When ENV is a strict environment (staging, stage, prod, production) `KAFKA_BROKERS` and `LOG_KEY_FP_SECRET` SHALL be required; in a local ENV they SHALL default to `localhost:9092` and the documented local secret. `.env.example` SHALL document every declared key. The payout hold keys `PAYOUT_HOLD_DAYS` and `PAYOUT_HOLD_WINDOW` SHALL keep their validation and their error naming the key.

#### Scenario: A missing ENV refuses to start
- **WHEN** `config.Load()` runs with `ENV` unset and every other required key set
- **THEN** it returns an error that names `ENV`
- **VERIFIED BY**: team-payment `internal/config` `TestLoadRefusesMissingEnv`

#### Scenario: An empty database URL refuses to start in every environment
- **WHEN** `config.Load()` runs with `ENV=local` and `DATABASE_URL` empty
- **THEN** it returns an error that names `DATABASE_URL`
- **VERIFIED BY**: team-payment `internal/config` `TestLoadRefusesEmptyDatabaseURL`

#### Scenario: Removed keys are not declared
- **WHEN** the env keys declared by `Settings` are listed
- **THEN** none of `DATABASE_ENABLED`, `KAFKA_ENABLED`, `PAYMENT_SETTLEMENT_CONSUMER_ENABLED`, `MOCK_PAYMENTS`, `SHUTDOWN_GRACE_SECONDS`, `UPSTREAM_CALL_TIMEOUT_SECONDS` is in the list, and `OTEL_ENABLED` and `GRPC_REFLECTION_ENABLED` are
- **VERIFIED BY**: team-payment `internal/config` `TestRemovedKeysAreNotDeclared`

#### Scenario: Reflection is off by default
- **WHEN** `config.Load()` runs with `ENV=local` and `GRPC_REFLECTION_ENABLED` unset
- **THEN** `Settings.ReflectionEnabled` is false and the started server answers a reflection request with `Unimplemented`
- **VERIFIED BY**: team-payment `internal/app` `TestReflectionOffByDefault`

#### Scenario: Reflection is refused in a strict environment
- **WHEN** `config.Load()` runs with `ENV=production` and `GRPC_REFLECTION_ENABLED=true`
- **THEN** it returns an error that names `GRPC_REFLECTION_ENABLED`
- **VERIFIED BY**: team-payment `internal/config` `TestReflectionRefusedInStrictEnv`

#### Scenario: A strict environment requires brokers, upstream addresses and key secret
- **WHEN** `config.Load()` runs with `ENV=staging`, `DATABASE_URL` set, and `KAFKA_BROKERS`, `UPSTREAM_ORDER_ADDR`, `UPSTREAM_AUDIT_ADDR` and `LOG_KEY_FP_SECRET` unset
- **THEN** it returns one error naming all four keys
- **VERIFIED BY**: team-payment `internal/config` `TestStrictEnvRequiresBrokersAuditAndKeySecret`

#### Scenario: Upstream addresses are required in every environment
- **WHEN** `config.Load()` runs with `ENV=local`, `DATABASE_URL` set, and `UPSTREAM_ORDER_ADDR` and `UPSTREAM_AUDIT_ADDR` unset
- **THEN** it returns one error naming both `UPSTREAM_ORDER_ADDR` and `UPSTREAM_AUDIT_ADDR`, and with both set the same input loads with brokers `localhost:9092`
- **VERIFIED BY**: team-payment `internal/config` `TestUpstreamAddrsRequiredInEveryEnv`

#### Scenario: An invalid payout hold window still refuses to start and names its key
- **WHEN** `config.Load()` runs with `PAYOUT_HOLD_DAYS=3651`, and again with `PAYOUT_HOLD_WINDOW=abc`
- **THEN** each call returns an error naming `PAYOUT_HOLD_DAYS` and `PAYOUT_HOLD_WINDOW` respectively, and `PAYOUT_HOLD_WINDOW=20s` overrides `PAYOUT_HOLD_DAYS=7` to a 20 s window
- **VERIFIED BY**: team-payment `internal/config` `TestPayoutHoldWindow`

#### Scenario: .env.example documents every key
- **WHEN** the keys declared by `Settings` are compared with the keys in `.env.example`
- **THEN** every declared key appears in `.env.example` and no removed key does
- **VERIFIED BY**: team-payment `internal/config` `TestEnvExampleInSync`

### Requirement: Mock payments are fixture data behind the payment-provider adapter

team-payment SHALL read mock settings only from `MOCK_DATA`, a comma-separated list of adapter names; the only known adapter SHALL be `payment-provider`. A name not in the known list SHALL refuse startup naming `MOCK_DATA`. A non-empty `MOCK_DATA` with a strict ENV SHALL be refused by `config.Load()` before any other group is validated, and the error SHALL name `MOCK_DATA`. When `payment-provider` is active the service SHALL log once at boot that fixture data is served. With the adapter active, `ProcessMockPayment` SHALL run the same ownership checks, order lookup, status compare-and-set and same-transaction `PaymentSettled` outbox write as the real path; only the provider outcome SHALL come from the fixture. `MOCK_PAYMENTS` SHALL NOT be read.

#### Scenario: The fixture provider settles through the real path
- **WHEN** `MOCK_DATA=payment-provider` and the buyer of a PENDING order calls `ProcessMockPayment` with `simulate_success=true`
- **THEN** the transaction in Postgres reads PAID with `provider_reference` `mock-<transaction id>` and exactly 1 `payment_outbox_events` row of type `platform.payment.v1.PaymentSettled` exists for the order
- **VERIFIED BY**: team-payment `internal/rpc` `TestMockDataProviderSettlesThroughRealPath`

#### Scenario: A simulated failure sets FAILED and writes no outbox row
- **WHEN** `MOCK_DATA=payment-provider` and the buyer calls `ProcessMockPayment` with `simulate_success=false`
- **THEN** the transaction reads FAILED and 0 outbox rows exist for the order
- **VERIFIED BY**: team-payment `internal/rpc` `TestMockDataProviderFailureWritesNoOutbox`

#### Scenario: Without the adapter the mock RPC is refused
- **WHEN** `MOCK_DATA` is empty and any caller calls `ProcessMockPayment`
- **THEN** the call fails with `failed_precondition` "mock payments are disabled" and the transaction is unchanged
- **VERIFIED BY**: team-payment `internal/rpc` `TestProcessMockPayment_DisabledByDefault`

#### Scenario: MOCK_PAYMENTS no longer enables anything
- **WHEN** the service starts with `MOCK_PAYMENTS=true` and `MOCK_DATA` empty
- **THEN** `ProcessMockPayment` fails with `failed_precondition`
- **VERIFIED BY**: team-payment `internal/app` `TestMockPaymentsKeyIsIgnored`

#### Scenario: A strict environment refuses mock data before validating anything else
- **WHEN** `config.Load()` runs with `ENV=production`, `MOCK_DATA=payment-provider` and `DATABASE_URL` empty
- **THEN** the returned error names `MOCK_DATA`
- **VERIFIED BY**: team-payment `internal/config` `TestMockDataRefusedBeforeValidation`

#### Scenario: An unknown adapter refuses startup
- **WHEN** the service starts with `ENV=local` and `MOCK_DATA=payment-provider,llm`
- **THEN** startup fails with an error naming `MOCK_DATA` and the known adapter `payment-provider`
- **VERIFIED BY**: team-payment `internal/app` `TestUnknownMockAdapterRefused`

#### Scenario: Active mock data is logged once
- **WHEN** the service starts with `MOCK_DATA=payment-provider`
- **THEN** the boot log holds exactly 1 warning naming adapter `payment-provider`
- **VERIFIED BY**: team-payment `internal/app` `TestMockDataLoggedOnce`

#### Scenario: A user who is neither the buyer nor an admin cannot use the mock path
- **WHEN** `MOCK_DATA=payment-provider` and another buyer calls `ProcessMockPayment` for a transaction they do not own
- **THEN** the call fails with `permission_denied` and the transaction is unchanged
- **VERIFIED BY**: team-payment `internal/rpc` `TestProcessMockPayment_Errors`

### Requirement: Every dependency is always on and has no runtime fallback

`app.Run` SHALL open Postgres with `postgres.Open`, create the Kafka producer, dial the order and audit upstreams, start the outbox relayer, the outbox purge and the settlement consumer in every ENV. Startup SHALL fail with a non-zero exit when Postgres is unreachable. `payment.NewService` SHALL require a transaction writer, a ledger repository and a settlement ledger and SHALL return an error for a nil one. In-memory stores SHALL exist only in `internal/store/fake`, and neither `cmd/server` nor `internal/app` SHALL import that package. A settle SHALL always write the payment status and the outbox row in one transaction.

#### Scenario: An unreachable database fails startup
- **WHEN** `app.Run` starts with `ENV=local` and a `DATABASE_URL` that refuses connections
- **THEN** it returns an error that wraps the database open failure and no gRPC listener is opened
- **VERIFIED BY**: team-payment `internal/app` `TestRunFailsWhenDatabaseUnreachable`

#### Scenario: The service refuses a missing transaction writer
- **WHEN** `payment.NewService` is built with a nil transaction writer
- **THEN** it returns an error naming the transaction writer
- **VERIFIED BY**: team-payment `internal/payment` `TestNewServiceRequiresTxWriter`

#### Scenario: Production wiring imports no in-memory store
- **WHEN** the import graph of `cmd/server` and `internal/app` is listed
- **THEN** it contains no package under `internal/store/fake`
- **VERIFIED BY**: team-payment `internal/app` `TestNoInMemoryStoreInProductionWiring`

#### Scenario: Settle writes the payment and its outbox row in one transaction
- **WHEN** a PENDING payment is settled and the outbox insert fails
- **THEN** the transaction rolls back, the payment still reads PENDING and 0 outbox rows exist; with a working insert the payment reads PAID with 1 outbox row
- **VERIFIED BY**: team-payment `internal/payment` `TestService_Settle_WritesPaymentAndOutboxAtomically`

### Requirement: Health separates readiness from liveness

The gRPC health service SHALL report the overall status `""` from the registered check `postgres` only: Kafka is not a readiness check, and the service SHALL stay ready while the broker is down (the outbox retries and the consumer reconnects). The service SHALL report the service `liveness` as SERVING for as long as the process runs, independent of those checks. On shutdown the overall status SHALL turn NOT_SERVING before the server stops.

#### Scenario: A database outage makes the service not ready but alive
- **WHEN** the Postgres check fails for one probe interval
- **THEN** the overall health status is NOT_SERVING and the `liveness` service is SERVING
- **VERIFIED BY**: team-payment `internal/app` `TestDatabaseOutageIsNotReadyButAlive`

#### Scenario: A broker outage does not make the service not ready
- **WHEN** the Kafka broker is unreachable while Postgres is healthy
- **THEN** the overall health status stays SERVING and the `liveness` service is SERVING
- **VERIFIED BY**: team-payment `internal/app` `TestBrokerOutageKeepsServiceReady`

#### Scenario: Recovery restores readiness
- **WHEN** the Postgres check passes again
- **THEN** the overall health status returns to SERVING within one probe interval
- **VERIFIED BY**: team-payment `internal/app` `TestReadinessRecoversAfterOutage`

#### Scenario: Shutdown drains health first
- **WHEN** the context passed to `app.Run` is cancelled
- **THEN** the overall status is NOT_SERVING before the gRPC listener closes, and `Run` returns nil within `SHUTDOWN_GRACE`
- **VERIFIED BY**: team-payment `internal/app` `TestShutdownDrainsHealthThenStops`

### Requirement: Migrations run only through the migrate subcommand

The `server` binary SHALL accept the subcommand `migrate up`, which applies the embedded migrations of `team-payment/migrations` with the SDK `migrate.Up` against `DATABASE_URL` and exits 0 on success, non-zero on failure. `app.Run` SHALL NOT apply, create or alter any schema. The compose stack SHALL run a `team-payment-migrate` service from the service image with `migrate up`, and the gitops chart SHALL run the same command in an Argo PreSync Job (deploy track).

#### Scenario: The migrate subcommand applies every embedded migration
- **WHEN** `server migrate up` runs against an empty Postgres
- **THEN** it exits 0 and `payment_outbox_events` has the columns `seq`, `topic` and `traceparent`, and a second run exits 0 and changes nothing
- **VERIFIED BY**: team-payment `cmd/server` `TestMigrateUpAppliesEmbeddedMigrations`

#### Scenario: The migrate subcommand fails on an unreachable database
- **WHEN** `server migrate up` runs with a `DATABASE_URL` that refuses connections
- **THEN** it exits non-zero and its log names the database open failure
- **VERIFIED BY**: team-payment `cmd/server` `TestMigrateUpFailsWhenDatabaseUnreachable`

#### Scenario: Booting the service never migrates
- **WHEN** `app.Run` starts against an empty Postgres with no migration applied
- **THEN** no table is created and the overall health status is NOT_SERVING
- **VERIFIED BY**: team-payment `internal/app` `TestRunDoesNotMigrate`

### Requirement: Docker-backed tests run and never skip

The Postgres and Kafka test suites of team-payment SHALL run under `GO_SDK_REQUIRE_DOCKER=1`, and a missing Docker daemon SHALL fail the suite instead of skipping it. A skipped Docker-backed test SHALL count as a gate failure.

#### Scenario: A missing Docker daemon fails the suite
- **WHEN** `go test ./internal/store/...` runs with `GO_SDK_REQUIRE_DOCKER=1` and no Docker daemon
- **THEN** the run exits non-zero and reports 0 skipped tests
- **VERIFIED BY**: team-payment `internal/store` `TestDockerRequiredFailsInsteadOfSkipping`

### Requirement: Payment events are relayed from the SDK outbox over the upgraded table

The `PaymentSettled` event SHALL be enqueued with `outbox.Store.EnqueueTx` into `payment_outbox_events` in the settle transaction, with `event_id` equal to the envelope `event_id`, `aggregate_id` equal to the order id and the marshalled envelope as payload. The SDK `outbox.Relayer` SHALL publish it to topic `payment.events` keyed by the order id. Migrations `0008` to `0017` SHALL upgrade an existing `payment_outbox_events` table to the SDK shape without losing a pending row and SHALL number existing rows by `(created_at, event_id)`. The upgrade SHALL requeue every row parked as `failed` by the old relayer to `pending` with `attempts` 0. A permanent publish error SHALL park the row; a transient error SHALL retry. Published rows SHALL be purged after 7 days by `outbox.Store.Purge` called on a schedule inside the service.

#### Scenario: A settled payment is published once with its envelope
- **WHEN** a payment is settled PAID and the relayer runs
- **THEN** exactly 1 record is produced to `payment.events` with key equal to the order id and a value that unmarshals to an `EventEnvelope` of type `platform.payment.v1.PaymentSettled` whose `event_id` equals the outbox `event_id`
- **VERIFIED BY**: team-payment `internal/store` `TestSettleEventIsRelayedOnce`

#### Scenario: The outbox upgrade keeps pending events in order
- **WHEN** migrations up to `0007` are applied, 3 pending rows for one order are inserted with increasing `created_at`, and migrations `0008` to `0017` are applied
- **THEN** the 3 rows keep status `pending`, have increasing `seq` in `created_at` order, and the relayer publishes them in that order
- **VERIFIED BY**: team-payment `internal/store` `TestOutboxUpgradeKeepsPendingOrder`

#### Scenario: The outbox upgrade requeues rows parked as failed
- **WHEN** migrations up to `0007` are applied, 1 row with status `failed` and 1 with status `published` are inserted, and migrations `0008` to `0017` are applied
- **THEN** the `failed` row reads `pending` with `attempts` 0 and the `published` row is unchanged
- **VERIFIED BY**: team-payment `internal/store` `TestOutboxUpgradeRequeuesFailedRows`

#### Scenario: Published rows older than 7 days are purged
- **WHEN** the scheduled purge runs with one row published 8 days ago, one published 6 days ago and one pending row created 9 days ago
- **THEN** only the row published 8 days ago is deleted
- **VERIFIED BY**: team-payment `internal/store` `TestPurgeDeletesPublishedRowsOlderThanSevenDays`

#### Scenario: The purge runs on a schedule inside the service
- **WHEN** `app.Run` has been up for more than one purge interval with a published row older than 7 days
- **THEN** that row is gone and the pending rows are untouched
- **VERIFIED BY**: team-payment `internal/app` `TestOutboxPurgeRunsOnSchedule`

#### Scenario: Events of one order stay ordered after a retry
- **WHEN** the first of two events of one order fails to publish once and then succeeds
- **THEN** the second event is not published before the first
- **VERIFIED BY**: team-payment `internal/store` `TestClaimPendingKeepsCreationOrder_Postgres`

#### Scenario: A broker outage delays but never loses the event
- **WHEN** the publisher returns a transient error for the first 2 attempts of a pending row
- **THEN** the row stays `pending` with `attempts` 2 and the third attempt publishes it and marks it `published`
- **VERIFIED BY**: team-payment `internal/store` `TestRelayerRetriesTransientPublishErrors`

#### Scenario: A row that cannot be encoded is parked
- **WHEN** the publisher returns an `outbox.Permanent` error for a row
- **THEN** the row has status `failed`, is not retried, and a later row of another order is still published
- **VERIFIED BY**: team-payment `internal/store` `TestRelayerParksPermanentErrors`

### Requirement: The settlement consumer runs on the SDK consume loop

The consumer of `order.events` SHALL use `consume.Consumer` with group `team-payment.settlement`, dead-letter topic `order.events.payment-settlement.dlq` and `kgo.ConsumeResetOffset(AtEnd)` for a new group. It SHALL credit the seller on `OrderPaidEvent`, refund the remainder of the payment on `OrderCancelled` from PAID, void the unpaid transaction on `OrderCancelled` from PENDING, refund an RMA return on `ReturnRefunded`, and ignore every other type. A malformed or unappliable record SHALL be wrapped with `consume.Permanent` and parked on the dead-letter topic without retries and without blocking later records. A transient error SHALL be retried up to the loop's attempt limit and then parked. Logs SHALL carry the key fingerprint (`key_fp`) and never the raw record key.

#### Scenario: A paid-order event credits once and a redelivery changes nothing
- **WHEN** the same `OrderPaidEvent` record is handled twice
- **THEN** the seller has exactly 1 `ORDER_SETTLEMENT` entry for the order and the balance equals the order amount
- **VERIFIED BY**: team-payment `internal/settlement` `TestConsumer_CreditsOnceAndRedeliveryIsNoop`

#### Scenario: A cancelled paid order is refunded once
- **WHEN** an `OrderCancelled` with previous status PAID is handled twice
- **THEN** the payment reads refunded in full once and the seller has exactly 1 `REFUND_DEDUCTION` entry for it
- **VERIFIED BY**: team-payment `internal/settlement` `TestCancel_RedeliveredIsNoop`

#### Scenario: A cancelled pending order voids its unpaid transaction
- **WHEN** an `OrderCancelled` with previous status PENDING is handled for an order that has a PENDING transaction
- **THEN** the transaction reads CANCELLED and no refund is recorded
- **VERIFIED BY**: team-payment `internal/settlement` `TestCancelledPendingVoidsTransaction`

#### Scenario: A return refund is applied once
- **WHEN** a `ReturnRefunded` record is handled twice
- **THEN** the payment's refunded amount increases once by the clamped amount
- **VERIFIED BY**: team-payment `internal/settlement` `TestReturnRefunded_RefundsOnceAndRedeliveryIsNoop`

#### Scenario: A paid-order event naming another seller's items is parked
- **WHEN** an `OrderPaidEvent` has line items of more than one seller, or an item without a seller id
- **THEN** no seller is credited, the handler error wraps `consume.Permanent` and the record goes to the dead-letter topic after 1 attempt
- **VERIFIED BY**: team-payment `internal/settlement` `TestConsumer_MultiSellerOrderIsPermanent`

#### Scenario: A shipped-order event does not touch the ledger
- **WHEN** an `OrderShipped` record is handled
- **THEN** the handler returns nil and the ledger has 0 new entries
- **VERIFIED BY**: team-payment `internal/settlement` `TestConsumer_IgnoresOrderShipped`

#### Scenario: A malformed record is parked after one attempt
- **WHEN** a record whose value is not an `EventEnvelope` is handled
- **THEN** the handler error wraps `consume.Permanent`, the loop makes 1 attempt and the record is produced to `order.events.payment-settlement.dlq`
- **VERIFIED BY**: team-payment `internal/settlement` `TestConsumer_PoisonRecordsGoToDLQAfterOneAttempt`

#### Scenario: A transient database error is retried and then parked
- **WHEN** the ledger returns a transient error for every attempt of one record
- **THEN** the record is attempted 5 times and then produced to the dead-letter topic, and the next record is processed
- **VERIFIED BY**: team-payment `internal/settlement` `TestConsumer_TransientErrorRetriedThenDLQ`

#### Scenario: A dead-letter outage commits nothing
- **WHEN** the dead-letter produce fails for a poison record
- **THEN** the offset of that record is not committed and a later record is not committed before it
- **VERIFIED BY**: team-payment `internal/settlement` `TestConsumer_DLQOutageCommitsNothingLater`

#### Scenario: Logs carry a key fingerprint and never the raw key
- **WHEN** a record with key `order-123` fails and is parked
- **THEN** the log output contains a `key_fp` attribute and does not contain `order-123`
- **VERIFIED BY**: team-payment `internal/settlement` `TestConsumerLogsKeyFingerprintNotKey`

#### Scenario: A new group starts at the latest offset
- **WHEN** the consumer client is built for a group with no committed offset
- **THEN** the client's reset offset is the end of the log
- **VERIFIED BY**: team-payment `internal/app` `TestSettlementConsumerStartsAtEnd`

### Requirement: Admin refunds are audited through the SDK audit gate

`RefundPayment` by a principal that is not the order's seller but holds scope `admin` SHALL write one audit event through `auditx.Gate` before the refund is applied: action `payment.admin_refund`, target type `payment`, target id the payment id, actor id the admin's user id, metadata `order_id`, `refund_id` and `amount`, never the reason. The audit connection SHALL use the service principal `service-team-payment` with scope `audit.write`. When the write fails or times out after 2 s, the call SHALL return `Unavailable` with message `audit trail unavailable` and no refund row, status change or ledger entry SHALL be recorded. A seller refund SHALL NOT call the audit service.

#### Scenario: An admin refund writes one event with the SDK event shape
- **WHEN** an admin refunds 200000 of a PAID payment with refund id `r-1`
- **THEN** exactly 1 `auditx.Event` is written with action `payment.admin_refund`, target `payment`/the payment id, actor the admin id and metadata keys `order_id`, `refund_id`, `amount` only
- **VERIFIED BY**: team-payment `internal/rpc` `TestAdminRefundAuditMetadataKeys`

#### Scenario: The audit write precedes the refund
- **WHEN** an admin refunds a PAID payment
- **THEN** the recorded call order is the audit write, then the refund
- **VERIFIED BY**: team-payment `internal/rpc` `TestAdminRefundAuditBeforeRefund`

#### Scenario: A failed audit write refuses the refund
- **WHEN** the audit writer returns an error during an admin refund
- **THEN** the call returns `Unavailable` with message `audit trail unavailable` and the payment still reads PAID with 0 refunds
- **VERIFIED BY**: team-payment `internal/rpc` `TestAdminRefundAuditErrorFailsClosed`

#### Scenario: A slow audit service is cut off
- **WHEN** the audit service does not answer within 2 s
- **THEN** the call returns `Unavailable` within 3 s and no refund is applied
- **VERIFIED BY**: team-payment `internal/rpc` `TestAdminRefundAuditTimeoutFailsClosed`

#### Scenario: The audit call is made as the service principal
- **WHEN** the audit writer sends an event
- **THEN** the outgoing metadata holds principal id `service-team-payment`, type `service` and scope `audit.write`, and holds no user principal
- **VERIFIED BY**: team-payment `internal/upstream` `TestAuditWriterUsesServicePrincipal`

#### Scenario: A seller refund is not audited and survives an audit outage
- **WHEN** the order's seller refunds a PAID payment while the audit service is down
- **THEN** the refund succeeds and 0 audit events are written
- **VERIFIED BY**: team-payment `internal/rpc` `TestSellerRefundIgnoresAuditOutage`

#### Scenario: A caller who is neither the seller nor an admin is denied without an event
- **WHEN** a buyer who is not the seller and holds no `admin` scope calls `RefundPayment`
- **THEN** the call fails with `permission_denied` and 0 audit events are written
- **VERIFIED BY**: team-payment `internal/rpc` `TestRefundPaymentNonSellerDeniedNoAudit`

#### Scenario: An anonymous caller is rejected before any audit
- **WHEN** an anonymous principal calls `RefundPayment`
- **THEN** the call fails with `unauthenticated` and 0 audit events are written
- **VERIFIED BY**: team-payment `internal/rpc` `TestRefundPaymentAnonymousNoAudit`

### Requirement: Service errors map to gRPC codes through one table

Handlers SHALL return errors through `grpcx.Status` with one `ErrorTable`. A sentinel for a missing row SHALL map to `NotFound`, a duplicate key (a sentinel or SQLSTATE 23505) to `AlreadyExists`, a refused state to `FailedPrecondition`, a refused caller to `PermissionDenied` and an invalid argument to `InvalidArgument`. `context.Canceled` SHALL map to `Canceled` and `context.DeadlineExceeded` to `DeadlineExceeded`. Any other error SHALL return `Internal` with the message `internal error`, with no driver or upstream text in the response.

#### Scenario: A unique-key violation is AlreadyExists
- **WHEN** the store returns a Postgres error with SQLSTATE 23505 to a handler
- **THEN** the call fails with `already_exists`
- **VERIFIED BY**: team-payment `internal/rpc` `TestUniqueViolationIsAlreadyExists`

#### Scenario: A cancelled or timed-out call keeps its code
- **WHEN** the store returns `context.Canceled`, and in another call `context.DeadlineExceeded`
- **THEN** the calls fail with `canceled` and `deadline_exceeded`
- **VERIFIED BY**: team-payment `internal/rpc` `TestContextErrorsKeepTheirCodes`

#### Scenario: An unexpected error is not echoed
- **WHEN** the store returns an error whose text contains `password=secret`
- **THEN** the call fails with `internal` and message `internal error`, and the response does not contain `password=secret`
- **VERIFIED BY**: team-payment `internal/rpc` `TestInternalErrorsAreNotEchoed`

#### Scenario: A refund exceeding the remainder keeps its refusal
- **WHEN** a seller refunds more than the refundable remainder
- **THEN** the call fails with `failed_precondition` and message `refund amount exceeds the refundable remainder`
- **VERIFIED BY**: team-payment `internal/rpc` `TestRefundExceedsRemainderIsFailedPrecondition`

### Requirement: The ledger list keeps its cursor format on the SDK pagination helpers

`ListLedgerEntries` SHALL clamp the page size with `pagination.Clamp(size, 20, 100)` and SHALL encode and decode the offset cursor with `pagination.EncodeOffset` and `pagination.DecodeOffset`. A cursor produced before this change SHALL decode to the same offset. A malformed cursor SHALL fail with `InvalidArgument`.

#### Scenario: A cursor from the previous build still pages
- **WHEN** a cursor string produced by the pre-migration encoder for offset 20 is passed to `ListLedgerEntries`
- **THEN** the page starts at the 21st newest entry
- **VERIFIED BY**: team-payment `internal/rpc` `TestLedgerCursorFormatIsUnchanged`

#### Scenario: The page size is clamped
- **WHEN** `ListLedgerEntries` is called with page size 0 and again with 500
- **THEN** the pages hold at most 20 and at most 100 entries respectively
- **VERIFIED BY**: team-payment `internal/rpc` `TestLedgerPageSizeClamped`

#### Scenario: A malformed cursor is rejected
- **WHEN** `ListLedgerEntries` is called with cursor `not-a-cursor`
- **THEN** the call fails with `invalid_argument`
- **VERIFIED BY**: team-payment `internal/rpc` `TestLedgerMalformedCursorInvalidArgument`

#### Scenario: A seller cannot page another seller's ledger
- **WHEN** a user principal calls `ListLedgerEntries` with another seller's id
- **THEN** the call fails with `permission_denied`
- **VERIFIED BY**: team-payment `internal/rpc` `TestWalletRPCAccess`
