## ADDED Requirements

### Requirement: team-notification depends on the tagged SDK and follows the Go service layout

`team-notification` SHALL require `github.com/buidangphuc/agora/platform-core/packages/go-sdk` at version `v1.0.2` in `go.mod`, SHALL NOT contain a `replace` directive for it or a `third_party/` directory, and SHALL include `../platform-core/make/go-service.mk` from its `Makefile` with `LINT_CONFIG` pointing at the SDK `.golangci.yml`, without copies of either file. `cmd/server/main.go` SHALL only create the signal context, call `config.Load` and `app.Run`, and exit non-zero on error. The packages `internal/bootstrap`, `internal/grpcserver`, `internal/interceptor`, `internal/handler`, `internal/repository`, `internal/service`, `internal/consumer` and `internal/upstream` SHALL NOT exist.

#### Scenario: The module requires the SDK tag without a replace
- **WHEN** `go.mod` and the service tree are inspected
- **THEN** `go.mod` has a `require` for the SDK module at `v1.0.2`, no `replace` line, no `third_party` directory, and none of the removed `internal/` packages exists
- **VERIFIED BY**: team-notification `internal/app` `TestModuleRequiresSDKTagWithoutReplace`

#### Scenario: A go.mod with a replace directive is rejected
- **WHEN** the module check runs on a `go.mod` that contains `replace github.com/buidangphuc/agora/platform-core/packages/go-sdk => ../x` or requires another version
- **THEN** the check returns an error naming the offending line
- **VERIFIED BY**: team-notification `internal/app` `TestModuleCheckRejectsReplaceAndWrongVersion`

#### Scenario: The Makefile reuses the shared targets
- **WHEN** the `Makefile` is read
- **THEN** it sets `SERVICE := team-notification` and `GRPC_PORT := 50058`, includes `../platform-core/make/go-service.mk`, sets `LINT_CONFIG` to `../platform-core/packages/go-sdk/.golangci.yml`, and no `.golangci.yml` or `go-service.mk` copy exists in the service
- **VERIFIED BY**: team-notification `internal/app` `TestMakefileIncludesSharedGoServiceMk`

### Requirement: Settings load from the environment with no dependency enable flags

`config.Load` SHALL build `Settings` from the SDK groups `Runtime`, `GRPC`, `Postgres`, `OTel`, `Kafka` and `LogKeyFP` plus the service fields `LOW_STOCK_THRESHOLD` (default 5), the topic, DLQ topic and consumer-group names of the listing, chat and order consumers (unchanged names and defaults), `UPSTREAM_DOMAIN_ADDR` and `UPSTREAM_IDENTITY_ADDR`. It SHALL fail before any resource is opened when `ENV`, `GRPC_PORT` or `DATABASE_URL` is unset, when `KAFKA_BROKERS` or `LOG_KEY_FP_SECRET` is unset in a strict ENV (staging, prod), when `UPSTREAM_DOMAIN_ADDR` or `UPSTREAM_IDENTITY_ADDR` is unset in any ENV (required, no default and no fallback mode), or when `LOW_STOCK_THRESHOLD` is not an integer in [0, 2147483647]. The service SHALL NOT read `DATABASE_ENABLED`, `KAFKA_ENABLED` or any other dependency enable flag; the only `*_ENABLED` keys are `OTEL_ENABLED` and `GRPC_REFLECTION_ENABLED` (default false, refused when `ENV` is strict). `.env.example` SHALL list exactly the keys `Settings` reads.

#### Scenario: A missing DATABASE_URL fails load
- **WHEN** `Load` runs with `ENV=local`, `GRPC_PORT=50058` and no `DATABASE_URL`
- **THEN** it returns an error that names `DATABASE_URL`
- **VERIFIED BY**: team-notification `internal/config` `TestLoadRejectsMissingDatabaseURL`

#### Scenario: Kafka brokers and the key fingerprint secret are required in a strict ENV
- **WHEN** `Load` runs with `ENV=staging` and `KAFKA_BROKERS` unset, then again with `KAFKA_BROKERS` set and `LOG_KEY_FP_SECRET` unset
- **THEN** each call returns an error naming the missing key, and with `ENV=local` both load with the SDK local defaults
- **VERIFIED BY**: team-notification `internal/config` `TestLoadRequiresKafkaBrokersAndKeyFPSecretInStrictEnv`

#### Scenario: Upstream addresses are required in every ENV
- **WHEN** `Load` runs with `ENV=prod` and `UPSTREAM_DOMAIN_ADDR` unset, then with `UPSTREAM_IDENTITY_ADDR` unset, and with `ENV=local` and each unset in turn
- **THEN** every call returns an error naming the missing key; no `localhost` default is applied
- **VERIFIED BY**: team-notification `internal/config` `TestLoadRequiresUpstreamAddrsInEveryEnv`

#### Scenario: No dependency enable flag exists
- **WHEN** the `Settings` struct tags, `.env.example`, `README.md` and the Go sources under `cmd/` and `internal/` (all inside `team-notification/`) are scanned for `_ENABLED`, `DATABASE_ENABLED`, `KAFKA_ENABLED` and `SHUTDOWN_GRACE_SECONDS`
- **THEN** the only `_ENABLED` matches are `OTEL_ENABLED` and `GRPC_REFLECTION_ENABLED`, and the other two names match nowhere; compose and gitops are outside the service directory and are covered by the Deploy track
- **VERIFIED BY**: team-notification `internal/config` `TestNoDependencyEnableFlags`

#### Scenario: Reflection is refused in a strict ENV and off by default
- **WHEN** `Load` runs with `ENV=prod` and `GRPC_REFLECTION_ENABLED=true`, and again with `ENV=local` and the variable unset
- **THEN** the first call returns an error before any group validation, and the second returns `ReflectionEnabled=false`
- **VERIFIED BY**: team-notification `internal/config` `TestLoadRefusesReflectionInStrictEnv`

#### Scenario: The low-stock threshold boundaries
- **WHEN** `LOW_STOCK_THRESHOLD` is `0`, `2147483647`, `-1`, `2147483648` or `abc`
- **THEN** the first two load with that value, and the last three return an error that contains `invalid LOW_STOCK_THRESHOLD`; unset loads 5
- **VERIFIED BY**: team-notification `internal/config` `TestLoadLowStockThreshold`

#### Scenario: .env.example matches Settings
- **WHEN** `config.CheckEnvExample` compares `.env.example` with `Settings`
- **THEN** it reports no missing and no extra key, and `.env.example` lists neither `DATABASE_ENABLED`, `KAFKA_ENABLED` nor `SHUTDOWN_GRACE_SECONDS`
- **VERIFIED BY**: team-notification `internal/config` `TestEnvExampleInSync`

### Requirement: Boot uses the real database and broker or fails

The service binary SHALL provide a `migrate up` subcommand that applies the embedded migrations with the SDK `pkg/migrate` and exits non-zero on failure. `app.Run` SHALL open Postgres with `postgres.Open`, register the `postgres` health checker and start the server and consumers; it SHALL NOT run migrations at boot. An unreachable database SHALL make `app.Run` return an error in every `ENV`, with already opened resources closed. The service SHALL NOT start with a nil pool, SHALL NOT use an in-memory repository, deduper or state store outside tests, and SHALL return no `Unimplemented` for the alert or preferences RPCs. A consumer that stops with an error SHALL stop the process with that error.

#### Scenario: An unreachable database fails boot
- **WHEN** `app.Run` starts with `ENV=local` and a `DATABASE_URL` pointing at a closed port
- **THEN** it returns an error within 10 s and the gRPC port is never listening
- **VERIFIED BY**: team-notification `internal/app` `TestRunFailsWhenDatabaseUnreachable`

#### Scenario: A failed migration fails the migrate subcommand
- **WHEN** `server migrate up` runs against a database whose `schema_migrations` is marked dirty
- **THEN** it exits non-zero with an error that wraps the migrate error, and a second run on a clean database exits 0
- **VERIFIED BY**: team-notification `internal/app` `TestMigrateUpFailsOnDirtyMigration`

#### Scenario: Boot never migrates
- **WHEN** `app.Run` starts against an empty Postgres (`testkit.Postgres`)
- **THEN** no table is created by `app.Run` (`schema_migrations` is absent) and no `migrate` call is made from `app.Run`
- **VERIFIED BY**: team-notification `internal/app` `TestRunDoesNotMigrate`

#### Scenario: Migrate then boot serves every RPC
- **WHEN** `server migrate up` runs against an empty Postgres (`testkit.Postgres`), then `app.Run` starts with a Redpanda broker from `testkit.Redpanda`
- **THEN** the tables `notifications`, `alert_subscriptions`, `notification_prefs`, `processed_events` and `listing_last_seen` exist and `SubscribeAlert`, `GetNotificationPrefs` and `ListNotifications` with a user principal return `OK`
- **VERIFIED BY**: team-notification `internal/app` `TestMigrateThenRunServesAllRPCs`

#### Scenario: A consumer failure stops the process
- **WHEN** one consumer component returns an error while the server runs
- **THEN** `app.Run` returns that error and the health service reports NOT_SERVING on the overall name
- **VERIFIED BY**: team-notification `internal/app` `TestConsumerErrorStopsRun`

#### Scenario: Postgres open is bounded and fails fast
- **WHEN** `postgres.Open` is given an address that accepts no connection (the SDK ping bound is 5 s)
- **THEN** `app.Run` returns the wrapped `ping database` error within 10 s; the 5 s bound itself is inherited from the SDK and tested there
- **VERIFIED BY**: team-notification `internal/app` `TestRunFailsWhenDatabaseUnreachable`

#### Scenario: Docker-backed tests never skip
- **WHEN** the suite runs with `GO_SDK_REQUIRE_DOCKER=1` and Docker is unavailable
- **THEN** the Docker-backed tests (`testkit.Postgres`, `testkit.Redpanda`) fail instead of skipping, and `make check` exits non-zero
- **VERIFIED BY**: team-notification `internal/app` `TestDockerRequiredNeverSkips`

#### Scenario: Production wiring has no in-memory fallback
- **WHEN** the non-test Go sources of `cmd/` and `internal/` are scanned
- **THEN** none contains `InMemory`, `Noop`, `mock mode`, `lookup disabled` or `RequireDurableStorage`
- **VERIFIED BY**: team-notification `internal/app` `TestNoRuntimeFallbacksInSource`

### Requirement: Readiness follows dependencies and liveness does not

The service SHALL register the health checker `postgres` only; Kafka SHALL NOT be a readiness check. The overall health status SHALL be SERVING only while the `postgres` checker passes. The health service name `liveness` SHALL stay SERVING while a checker fails, and SHALL become NOT_SERVING only when shutdown drains. On shutdown the service SHALL drain health first, wait `SHUTDOWN_DRAIN_DELAY`, then stop within `SHUTDOWN_GRACE`. gRPC reflection SHALL be registered only when `GRPC_REFLECTION_ENABLED` is true.

#### Scenario: A database outage flips readiness but not liveness
- **WHEN** Postgres is stopped while the service runs
- **THEN** within 10 s (2 x `healthx.DefaultInterval` of 5 s) the overall status is NOT_SERVING and `liveness` is still SERVING
- **VERIFIED BY**: team-notification `internal/app` `TestReadinessFollowsPostgresLivenessDoesNot`

#### Scenario: A broker outage does not flip readiness
- **WHEN** the test starts `testkit.Redpanda`, runs the service and stops the broker
- **THEN** the overall status stays SERVING for 15 s, no checker named `kafka` exists, and the gRPC server still answers `GetUnreadCount`
- **VERIFIED BY**: team-notification `internal/app` `TestReadinessIgnoresKafka`

#### Scenario: Liveness answers from memory
- **WHEN** 1,000 health `Check` calls for the `liveness` service run while Postgres is down
- **THEN** every call returns SERVING and the p99 latency is under 5 ms
- **VERIFIED BY**: team-notification `internal/app` `TestLivenessCheckLatencyUnder5ms`

#### Scenario: Shutdown drains health first
- **WHEN** the run context is cancelled while a client holds a health `Watch`
- **THEN** the stream receives NOT_SERVING for the overall name and for `liveness` before the server stops
- **VERIFIED BY**: team-notification `internal/app` `TestShutdownDrainsHealthFirst`

#### Scenario: Reflection is off unless enabled
- **WHEN** the server runs with `GRPC_REFLECTION_ENABLED` unset, and again with it `true` and `ENV=local`
- **THEN** a reflection `ListServices` call returns `Unimplemented` the first time and lists `platform.notification.v1.NotificationService` the second time
- **VERIFIED BY**: team-notification `internal/app` `TestReflectionOffByDefault`

#### Scenario: A panicking handler does not kill the process
- **WHEN** a handler panics during a call
- **THEN** the caller receives `Internal` with message `internal error` and the next `GetUnreadCount` call succeeds
- **VERIFIED BY**: team-notification `internal/rpc` `TestPanicReturnsInternalAndServerKeepsServing`

### Requirement: Notification RPCs serve only the calling user's data

Every notification RPC (`ListNotifications`, `MarkAsRead`, `GetUnreadCount`, `SubscribeAlert`, `UnsubscribeAlert`, `ListAlertSubscriptions`, `GetNotificationPrefs`, `UpdateNotificationPrefs`) SHALL resolve the owner with `auth.RequireUser` and scope every query to that user id. A missing or anonymous principal SHALL give `Unauthenticated`; a service principal SHALL give `PermissionDenied`; a notification id that does not exist or belongs to another user SHALL give `NotFound` from `MarkAsRead`, and an empty `id` SHALL mark all of the caller's notifications read and no other user's. `ListNotifications` SHALL order newest first, default `page_size` to 20 and `page_number` to 1, clamp `page_size` to at most 100, and return `total_unread`. A store error SHALL reach the caller only as `Internal` with the message `internal error`.

#### Scenario: A user sees only their own notifications
- **WHEN** users U1 and U2 each have two notifications and U1 calls `ListNotifications`
- **THEN** the response has U1's two notifications newest first, `total_unread` is 2 for unread rows, and none belongs to U2
- **VERIFIED BY**: team-notification `internal/rpc` `TestUsersDoNotSeeEachOthersData`

#### Scenario: Another user's notification id is NotFound
- **WHEN** U1 calls `MarkAsRead` with the id of U2's notification
- **THEN** the call returns `NotFound` and U2's notification is still unread
- **VERIFIED BY**: team-notification `internal/rpc` `TestMarkAsReadOtherUsersNotificationIsNotFound`

#### Scenario: An empty id marks all of the caller's notifications read
- **WHEN** U1 (three unread) and U2 (two unread) exist and U1 calls `MarkAsRead` with an empty `id`
- **THEN** U1's `GetUnreadCount` returns 0 and U2's still returns 2
- **VERIFIED BY**: team-notification `internal/rpc` `TestMarkAsReadEmptyIDMarksAllOfCallerOnly`

#### Scenario: Listing stays fast at 10,000 rows per user
- **WHEN** one user has 10,000 notifications and `ListNotifications`, `GetUnreadCount` and `MarkAsRead` (one id) each run 200 times on Postgres
- **THEN** the p95 latency of each is under 100 ms
- **VERIFIED BY**: team-notification `internal/store` `TestInboxQueriesP95At10kRows`

#### Scenario: Principal types are enforced on every per-user RPC
- **WHEN** each per-user RPC is called with no principal, with an anonymous principal and with a service principal
- **THEN** the first two return `Unauthenticated` and the third returns `PermissionDenied`, and no row is read or written
- **VERIFIED BY**: team-notification `internal/rpc` `TestPerUserRPCsRequireAUserPrincipal`

#### Scenario: The page size is clamped at the boundaries
- **WHEN** `ListNotifications` is called with `page_size` 0, 100, 101 and `page_number` 0 on a user with 150 notifications
- **THEN** the responses hold 20, 100 and 100 notifications, and `page_number` 0 behaves as page 1
- **VERIFIED BY**: team-notification `internal/rpc` `TestListNotificationsPageSizeClamp`

#### Scenario: A store failure is not leaked
- **WHEN** the store returns an error whose text contains a SQL fragment and a user id
- **THEN** the caller receives `Internal` with the message `internal error` and the text appears in the server log only
- **VERIFIED BY**: team-notification `internal/rpc` `TestMapErrsHideInternalCause`

#### Scenario: A unique-key violation maps to AlreadyExists
- **WHEN** a store call fails with SQLSTATE 23505
- **THEN** the caller receives `AlreadyExists`
- **VERIFIED BY**: team-notification `internal/rpc` `TestUniqueViolationMapsToAlreadyExists`

### Requirement: Alert and preference RPCs validate input and are always served

`GetNotificationPrefs` SHALL return, for a user with no stored preferences, the defaults: all six real notification types enabled and digest frequency OFF. `UpdateNotificationPrefs` SHALL return `InvalidArgument` and store nothing when the request carries no prefs payload (`prefs.ErrNilPrefs`); an empty user id (`alerts.ErrEmptyUser`) SHALL return `InvalidArgument` too. `SubscribeAlert` SHALL require a non-empty `listing_id` and an `AlertType` other than `UNSPECIFIED`, and SHALL be idempotent per `(user, listing, type)`. `UnsubscribeAlert` SHALL require a non-empty `subscription_id`, SHALL succeed when the id is missing or foreign, and SHALL delete only the caller's row. Validation failures SHALL return `InvalidArgument`. These RPCs SHALL be served whenever the service is up; none returns `Unimplemented`.

#### Scenario: Subscribing twice returns the same subscription
- **WHEN** a user calls `SubscribeAlert` twice for the same listing and type
- **THEN** both responses carry the same subscription id and `ListAlertSubscriptions` returns one row
- **VERIFIED BY**: team-notification `internal/rpc` `TestSubscribeAlertIsIdempotent`

#### Scenario: An invalid subscription is rejected
- **WHEN** `SubscribeAlert` is called with an empty `listing_id`, or with `ALERT_TYPE_UNSPECIFIED`
- **THEN** each call returns `InvalidArgument` and no row is created
- **VERIFIED BY**: team-notification `internal/rpc` `TestSubscribeAlertValidationIsInvalidArgument`

#### Scenario: Unsubscribing another user's subscription changes nothing
- **WHEN** U1 calls `UnsubscribeAlert` with U2's subscription id
- **THEN** the call succeeds and U2's subscription still exists
- **VERIFIED BY**: team-notification `internal/rpc` `TestUnsubscribeForeignSubscriptionIsNoOp`

#### Scenario: Preferences default and round-trip per user
- **WHEN** a user with no stored preferences calls `GetNotificationPrefs`, then `UpdateNotificationPrefs` disabling `NOTIFICATION_TYPE_CHAT`
- **THEN** the first response holds the defaults (all six real types enabled, digest OFF), a second `GetNotificationPrefs` shows CHAT disabled, and another user's preferences are unchanged
- **VERIFIED BY**: team-notification `internal/prefs` `TestUpdateRoundtripAndIsolation`

#### Scenario: A missing prefs payload is rejected
- **WHEN** `UpdateNotificationPrefs` is called with no `prefs` field, and again as a direct call with an empty user id
- **THEN** each returns `InvalidArgument`, and a following `GetNotificationPrefs` still returns the defaults
- **VERIFIED BY**: team-notification `internal/prefs` `TestPrefsInputValidation`

### Requirement: Kafka consumers run on the SDK loop with a transactional ledger

`team-notification` SHALL run three consumers, one consumer group each, on the SDK `consume.Consumer`: `listing.events` (group `team-notification.listing`), `chat.events` (`team-notification.chat`) and `order.events` (`team-notification.order`), each with its own DLQ topic. The handler SHALL record `(consumer, event_id)` in `processed_events` with `consume.Dedupe.Seen` inside the same database transaction as the notification and state writes. A redelivered event SHALL create no second notification. A handler error SHALL roll the transaction back, including the ledger row, so the retry creates the notification. A malformed envelope or an empty `event_id` SHALL be sent to the DLQ and its offset committed. The loop SHALL NOT commit past a record that is neither handled nor dead-lettered. Logs SHALL carry the keyed fingerprint `key_fp` and SHALL NOT carry the raw record key.

#### Scenario: A redelivered event notifies once
- **WHEN** the same `ListingChanged`, chat or `OrderShipped` envelope (same `event_id`) is handled twice, including once after a process restart
- **THEN** exactly one notification exists for the recipient
- **VERIFIED BY**: team-notification `internal/events` `TestRedeliveredEventNotifiesOnce_Postgres`

#### Scenario: A failed write rolls the ledger row back
- **WHEN** the notification insert fails once for a chat event and the record is retried
- **THEN** no `processed_events` row exists after the first attempt and exactly one notification exists after the retry
- **VERIFIED BY**: team-notification `internal/events` `TestFailedWriteRollsBackLedgerAndRetryNotifies_Postgres`

#### Scenario: A malformed record goes to the DLQ and the offset advances
- **WHEN** a record whose value is not an `EventEnvelope`, and a record with an empty `event_id`, are consumed
- **THEN** each is produced to the consumer's DLQ topic, its offset is committed, no notification is created, and the next record is handled
- **VERIFIED BY**: team-notification `internal/events` `TestMalformedRecordGoesToDLQAndAdvances`

#### Scenario: A transient failure retries without committing
- **WHEN** the handler fails transiently on its first two attempts for a record
- **THEN** the record is handled on the third attempt and its offset is committed only after that
- **VERIFIED BY**: team-notification `internal/events` `TestTransientFailureRetriesBeforeCommit`

#### Scenario: Logs show the key fingerprint, not the key
- **WHEN** a handler error is logged for a record whose key is a user id `u-1234`
- **THEN** the log line contains a `key_fp` field and does not contain `u-1234`
- **VERIFIED BY**: team-notification `internal/events` `TestConsumerLogsKeyFingerprintNotKey`

#### Scenario: A chat message becomes a notification within 30 seconds
- **WHEN** `app.Run` runs against `testkit.Postgres` and `testkit.Redpanda` and a chat envelope for U1 is produced to `chat.events`
- **THEN** `ListNotifications` for U1 returns one CHAT notification within 30 s
- **VERIFIED BY**: team-notification `internal/app` `TestChatEventBecomesNotificationWithin30s`

#### Scenario: Consumer settings keep their names and defaults
- **WHEN** `Load` runs with none of the topic, DLQ or group variables set
- **THEN** the topics are `listing.events`, `chat.events`, `order.events`, the DLQ topics add `.dlq`, and the groups are `team-notification.listing`, `team-notification.chat`, `team-notification.order`
- **VERIFIED BY**: team-notification `internal/config` `TestConsumerSettingsDefaults`

### Requirement: Notification rules keep their behaviour on the new wiring

The listing, chat and order consumers SHALL keep producing the notifications specified in `buyer-notifications` and `notification-order-work`, with all state in Postgres: the price drop and restock baselines in `listing_last_seen`, the low-stock state, and the ledger. A first-seen listing SHALL NOT fire. A recipient who disabled `NOTIFICATION_TYPE_CHAT` or `NOTIFICATION_TYPE_ORDER` SHALL receive no chat or order notification; listing alerts (price drop, back in stock, low stock) SHALL NOT consult preferences, as today. A chat or order event SHALL notify only its intended recipient, and an event for user A SHALL NOT create a row for user B.

#### Scenario: A price drop after a restart is still detected
- **WHEN** a listing was seen at 2,000,000, the service restarts, and the price drops to 500,000 for a subscribed user
- **THEN** that user receives one price-drop notification
- **VERIFIED BY**: team-notification `internal/events` `TestPriceDropAfterRestartAndRedelivery_Postgres`

#### Scenario: A first-seen listing creates nothing
- **WHEN** the first snapshot of a listing arrives with any price and stock
- **THEN** no notification is created and the baseline is stored
- **VERIFIED BY**: team-notification `internal/events` `TestPriceFirstSeen_CreatesNone`

#### Scenario: A disabled notification type is skipped
- **WHEN** the recipient has disabled `NOTIFICATION_TYPE_CHAT` and a chat event for them is handled
- **THEN** no notification is created and the event is recorded as processed
- **VERIFIED BY**: team-notification `internal/events` `TestChat_DisabledPreferenceSkips`

#### Scenario: A disabled order notification type is skipped
- **WHEN** the buyer has disabled `NOTIFICATION_TYPE_ORDER` and an `OrderShipped` event for them is handled
- **THEN** no notification is created and the event is recorded as processed
- **VERIFIED BY**: team-notification `internal/events` `TestOrder_DisabledPreferenceSkips`

#### Scenario: A listing alert ignores preferences
- **WHEN** a user who disabled every notification type is subscribed to a listing whose price then drops
- **THEN** the user receives one price-drop notification, and the same holds for back in stock and low stock
- **VERIFIED BY**: team-notification `internal/events` `TestListingAlertsIgnorePrefs`

#### Scenario: An event for user A never creates a row for user B
- **WHEN** an `OrderShipped` event with buyer "B1" and seller "S1" and a chat event whose recipient is "U1" are handled
- **THEN** access is denied by construction for everyone else: "S1" and every user other than "B1" and "U1" have no notification row, and no row has a `user_id` that is not the event's intended recipient
- **VERIFIED BY**: team-notification `internal/events` `TestEventsNeverCreateRowsForOtherUsers`

#### Scenario: The low-stock state survives a restart
- **WHEN** a listing crosses the low-stock threshold, the service restarts, and the same low level arrives again
- **THEN** the seller has exactly one low-stock notification
- **VERIFIED BY**: team-notification `internal/events` `TestLowStockStateDurableAcrossRestart`

#### Scenario: Only the buyer of a shipped order is notified
- **WHEN** an `OrderShipped` event with buyer "B1" and seller "S1" is handled
- **THEN** "B1" has one `NOTIFICATION_TYPE_ORDER` notification linking to `/account/orders/<order_id>` and "S1" has none
- **VERIFIED BY**: team-notification `internal/events` `TestOrderShipped_NotifiesBuyer`

#### Scenario: An order event for a different seller notifies nobody else
- **WHEN** an `OrderPlaced` COD event for seller "S1" is handled
- **THEN** seller "S2" and the buyer have no notification, and `notifications` holds no row for any user other than "S1"
- **VERIFIED BY**: team-notification `internal/events` `TestOrderPlacedOtherSellerNotNotified`

### Requirement: Sender names come from the SDK upstream client as a read-only service principal

`UPSTREAM_DOMAIN_ADDR` and `UPSTREAM_IDENTITY_ADDR` SHALL be set in every ENV. The chat consumer SHALL resolve the sender name through `upstream.NewClient` connections to team-domain (`BatchGetStorefronts`) and team-identity (`GetPublicProfiles`), calling as service principal `service-team-notification` with exactly the scopes `listing.read` and `identity.read`. The pair of lookups SHALL be bounded to 2 s. A lookup failure, timeout or empty result SHALL give the neutral label and the notification SHALL still be created. The service SHALL NOT start without the resolver and SHALL NOT log a "lookup disabled" state.

#### Scenario: A seller reply names the shop
- **WHEN** the sender is the thread's seller and team-domain returns the display name "Nhà Sách An Nhiên"
- **THEN** the chat notification title contains "Nhà Sách An Nhiên"
- **VERIFIED BY**: team-notification `internal/names` `TestResolve_SellerGetsShopName`

#### Scenario: Lookup failure still notifies
- **WHEN** both upstream calls return an error
- **THEN** the resolver returns an empty name and the chat notification is created with the label `Người dùng`
- **VERIFIED BY**: team-notification `internal/events` `TestChat_LookupFailureStillNotifies`

#### Scenario: The lookup carries the read-only service principal
- **WHEN** the resolver calls an upstream test server
- **THEN** the incoming metadata has `x-principal-id=service-team-notification`, `x-principal-type=service` and `x-principal-scopes=listing.read,identity.read`, whatever principal the consumer context had
- **VERIFIED BY**: team-notification `internal/names` `TestResolverSendsReadOnlyServicePrincipal`

#### Scenario: The resolver principal cannot use the per-user RPCs
- **WHEN** a client calls `ListNotifications` and `MarkAsRead` with the exact metadata the resolver sends (`x-principal-type=service`, scopes `listing.read,identity.read`)
- **THEN** both are denied with `PermissionDenied` and no row is read or changed
- **VERIFIED BY**: team-notification `internal/rpc` `TestResolverPrincipalDeniedOnPerUserRPCs`

#### Scenario: A slow upstream is cut at 2 seconds
- **WHEN** team-domain does not answer for 10 s
- **THEN** the resolver returns an empty name within 2.5 s
- **VERIFIED BY**: team-notification `internal/names` `TestResolverBoundedToTwoSeconds`
