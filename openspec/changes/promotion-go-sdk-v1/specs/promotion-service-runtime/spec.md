## ADDED Requirements

### Requirement: Settings load through the SDK and refuse unsafe configuration

team-promotion SHALL load its settings with `config.Load` from go-sdk v1.0.2. `ENV`, `GRPC_PORT`, `DATABASE_URL`, `UPSTREAM_DOMAIN_ADDR` and `UPSTREAM_AUDIT_ADDR` SHALL be required with no default in every `ENV`; an upstream the service calls has no mode where its address may be empty. The service SHALL read no environment variable that switches a dependency on or off; the only allowed flags are `OTEL_ENABLED` and `GRPC_REFLECTION_ENABLED`. `GRPC_REFLECTION_ENABLED` SHALL default to false and SHALL be refused unless `ENV` is `local` or `test`. IF `ENV` is staging or prod, THEN `KAFKA_BROKERS` SHALL be required. `MAX_AD_BID` and `MAX_AD_BUDGET` SHALL be greater than zero. `UPSTREAM_CALL_TIMEOUT_SECONDS` SHALL NOT exist: upstream calls use the SDK 2 s bound. Durations (`SHUTDOWN_GRACE`, `SHUTDOWN_DRAIN_DELAY`, `FEATURE_FLAGS_EVAL_TIMEOUT`, `OUTBOX_*`) SHALL be Go durations. The subcommand `migrate up` SHALL need only `ENV` and `DATABASE_URL`. `.env.example` SHALL list every declared key.

#### Scenario: Missing ENV is refused
- **WHEN** settings load with `GRPC_PORT`, `DATABASE_URL` and `UPSTREAM_DOMAIN_ADDR` set and `ENV` unset
- **THEN** `Load` returns an error that names `ENV` and no settings are returned
- **VERIFIED BY**: team-promotion `internal/config` `TestLoadRequiresEnv`

#### Scenario: Missing database URL or upstream address is refused
- **WHEN** settings load under `ENV=local`, once without `DATABASE_URL`, once without `UPSTREAM_DOMAIN_ADDR` and once without `UPSTREAM_AUDIT_ADDR`
- **THEN** each load returns an error naming the missing key
- **VERIFIED BY**: team-promotion `internal/config` `TestLoadRequiresDatabaseURLAndUpstreamAddrs`

#### Scenario: Retired enable flags are not read
- **WHEN** `DATABASE_ENABLED=false`, `KAFKA_ENABLED=false`, `FEATURE_FLAGS_ENABLED=false` and `OUTBOX_ENABLED=false` are set and `DATABASE_URL` is unset
- **THEN** `Load` still returns the missing `DATABASE_URL` error, and no `Settings` field carries an `env` tag ending in `_ENABLED` other than `OTEL_ENABLED` and `GRPC_REFLECTION_ENABLED`
- **VERIFIED BY**: team-promotion `internal/config` `TestNoDependencyEnableFlags`

#### Scenario: Reflection is off by default and refused outside local
- **WHEN** settings load under `ENV=local` with no reflection variable, and again under `ENV=prod` with `GRPC_REFLECTION_ENABLED=true`
- **THEN** the first load has `ReflectionEnabled` false and the second returns an error naming `GRPC_REFLECTION_ENABLED`
- **VERIFIED BY**: team-promotion `internal/config` `TestReflectionDefaultOffAndRefusedInStrictEnv`

#### Scenario: Strict ENV requires brokers
- **WHEN** settings load under `ENV=staging` without `KAFKA_BROKERS`
- **THEN** the load returns an error naming `KAFKA_BROKERS`
- **VERIFIED BY**: team-promotion `internal/config` `TestStrictEnvRequiresKafkaBrokers`

#### Scenario: Ad limits must be positive
- **WHEN** `MAX_AD_BID` or `MAX_AD_BUDGET` is `0`, `-5` or `abc`, and then `1`
- **THEN** the first three values are rejected and `1` is accepted for each key
- **VERIFIED BY**: team-promotion `internal/config` `TestAdCapsDefaultsAndValidation`

#### Scenario: The retired call timeout key is not read
- **WHEN** `UPSTREAM_CALL_TIMEOUT_SECONDS=0.01` is set and `DATABASE_URL` is unset
- **THEN** `Load` returns the missing `DATABASE_URL` error and no `Settings` field carries the key
- **VERIFIED BY**: team-promotion `internal/config` `TestRetiredCallTimeoutKeyNotRead`

#### Scenario: Migrate settings need only ENV and the database URL
- **WHEN** the migrate settings load with only `ENV=local` and `DATABASE_URL` set, and again with `DATABASE_URL` unset
- **THEN** the first load succeeds and the second returns an error naming `DATABASE_URL`
- **VERIFIED BY**: team-promotion `internal/config` `TestLoadMigrateNeedsOnlyEnvAndDatabaseURL`

#### Scenario: .env.example matches the settings
- **WHEN** the drift check runs over `Settings` and `.env.example`
- **THEN** every declared key is present once in the file and every key in the file is declared
- **VERIFIED BY**: team-promotion `internal/config` `TestEnvExampleInSync`

### Requirement: Boot fails fast without a database and migrations run only through migrate up

`app.Run` SHALL open Postgres with `postgres.Open` and SHALL return an error, opening no listener, when the database is unreachable. `app.Run` SHALL NOT apply migrations. The service binary SHALL provide the subcommand `server migrate up`, which SHALL apply the embedded migrations `0001` to `0005` with the SDK `migrate.Up` and exit; it SHALL be idempotent, and a migration error, including a dirty version, SHALL make it return an error and exit non-zero. Any other subcommand SHALL be refused and start nothing. The production binary SHALL contain no in-memory repository: the fakes in `internal/store/storefake` SHALL be imported only by test files.

#### Scenario: Unreachable database fails boot
- **WHEN** `app.Run` starts with a `DATABASE_URL` that points to a closed port
- **THEN** it returns an error within 8 s and the gRPC port is not listening
- **VERIFIED BY**: team-promotion `internal/app` `TestRunFailsOnUnreachableDatabase`

#### Scenario: Boot does not migrate
- **WHEN** `app.Run` starts against a reachable empty database
- **THEN** no table `vouchers` and no table `schema_migrations` exists after boot
- **VERIFIED BY**: team-promotion `internal/app` `TestRunDoesNotMigrate`

#### Scenario: migrate up applies migrations to an empty database
- **WHEN** `migrate up` runs against an empty database
- **THEN** the tables `vouchers`, `voucher_reservations`, `flash_sale_campaigns`, `flash_sale_holds` and `promotion_outbox` exist and `schema_migrations` is at version 5 and not dirty
- **VERIFIED BY**: team-promotion `internal/app` `TestMigrateUpMigratesEmptyDatabase`

#### Scenario: migrate up continues from version 4
- **WHEN** `migrate up` runs against a database that `0001` to `0004` already migrated under `schema_migrations` version 4
- **THEN** only `0005` is applied, existing voucher and campaign rows are unchanged, and the version is 5
- **VERIFIED BY**: team-promotion `internal/store` `TestMigrateFromVersion4AppliesOutboxOnly`

#### Scenario: migrate up twice is a no-op
- **WHEN** `migrate up` runs twice in a row against an empty database
- **THEN** both runs return no error and the version stays 5
- **VERIFIED BY**: team-promotion `internal/app` `TestMigrateUpTwiceIsNoop`

#### Scenario: A dirty migration state fails migrate up
- **WHEN** `migrate up` runs against a database whose `schema_migrations` row is dirty
- **THEN** it returns an error and the version row is unchanged
- **VERIFIED BY**: team-promotion `internal/app` `TestMigrateUpFailsOnDirtyState`

#### Scenario: An unknown subcommand is refused
- **WHEN** the binary entry runs with the arguments `migrate down` and with `serve`
- **THEN** each returns an error naming the subcommand, no listener opens and no migration runs
- **VERIFIED BY**: team-promotion `internal/app` `TestExecuteRejectsUnknownSubcommand`

#### Scenario: The outbox migration equals the SDK schema
- **WHEN** the file `0005_outbox.up.sql` is compared with `outbox.SchemaSQL("promotion_outbox")`
- **THEN** the two texts are identical
- **VERIFIED BY**: team-promotion `internal/store` `TestOutboxMigrationMatchesSDKSchema`

#### Scenario: Production code has no in-memory backend
- **WHEN** `go list -deps ./cmd/server` runs
- **THEN** the output does not contain `internal/store/storefake`
- **VERIFIED BY**: team-promotion `internal/store` `TestFakesNotImportedByProductionCode`

### Requirement: Readiness follows the database and liveness does not

team-promotion SHALL serve gRPC health with the overall status `""` driven by a `postgres` checker and the service `liveness` reporting process liveness only. Overall health SHALL turn NOT_SERVING within two check intervals after the database stops answering; `liveness` SHALL stay SERVING in that case. WHEN shutdown starts, THEN both statuses SHALL turn NOT_SERVING before the server stops accepting RPCs, and an in-flight RPC SHALL complete. Readiness SHALL consist of the `postgres` check only; Kafka and Flipt SHALL NOT be readiness checkers, and no upstream check is registered (a team-domain or team-audit outage fails only the calls that need it, closed).

#### Scenario: Ready and live after boot
- **WHEN** `app.Run` has started against a reachable database
- **THEN** health `""` and health `liveness` both return SERVING within one check interval
- **VERIFIED BY**: team-promotion `internal/app` `TestHealthServingAfterBoot`

#### Scenario: A failing database check flips readiness only
- **WHEN** the database pool is closed while the service runs, so the `postgres` check fails
- **THEN** health `""` returns NOT_SERVING within two intervals and health `liveness` still returns SERVING
- **VERIFIED BY**: team-promotion `internal/app` `TestReadinessFlipsLivenessStays`

#### Scenario: Shutdown drains health before the stop
- **WHEN** the run context is canceled while a health Watch stream is open and a `ReserveFlashSaleStock` call is in flight
- **THEN** the stream shows NOT_SERVING for `""` and `liveness`, and the in-flight call returns its normal response
- **VERIFIED BY**: team-promotion `internal/app` `TestShutdownDrainsHealthFirst`

#### Scenario: A Kafka outage keeps readiness SERVING
- **WHEN** the brokers are unreachable and a health check runs
- **THEN** health `""` returns SERVING
- **VERIFIED BY**: team-promotion `internal/app` `TestKafkaOutageKeepsReadiness`

### Requirement: Voucher and campaign changes are published through a transactional outbox

`CreateVoucher` and `CreateCampaign` SHALL insert the entity and one `promotion_outbox` row in one database transaction. The row SHALL have `aggregate_type` `voucher` or `flash_sale`, `aggregate_id` equal to the entity id, `event_type` `platform.promotion.v1.VoucherChanged` or `platform.promotion.v1.FlashSaleChanged`, the topic `PROMOTION_EVENTS_TOPIC`, and a proto-marshalled `EventEnvelope` payload carrying the event id, the verified principal, the request id and the traceparent. A relayer component SHALL publish rows to Kafka keyed by the entity id, in order per aggregate. IF the transaction rolls back, THEN no outbox row SHALL exist. IF Kafka is down, THEN the RPC SHALL still succeed and the row SHALL be published after recovery. Published rows SHALL be purged 7 days after publication by the SDK `Store.Purge`, run on a schedule inside the service (every hour and at start). Redemption RPCs (`ValidateAndReserve`, `CommitReservation`, `ReleaseReservation`, flash hold RPCs) SHALL write no outbox row.

#### Scenario: Creating a voucher stores one outbox row
- **WHEN** an admin creates a platform voucher
- **THEN** `promotion_outbox` holds exactly one row with `aggregate_type` `voucher`, `aggregate_id` equal to the voucher id and `event_type` `platform.promotion.v1.VoucherChanged`
- **VERIFIED BY**: team-promotion `internal/store` `TestCreateVoucherEnqueuesOutboxRow`

#### Scenario: Creating a campaign stores one outbox row
- **WHEN** a seller creates a flash-sale campaign on their own listing
- **THEN** `promotion_outbox` holds exactly one row with `aggregate_type` `flash_sale` and `event_type` `platform.promotion.v1.FlashSaleChanged`
- **VERIFIED BY**: team-promotion `internal/store` `TestCreateCampaignEnqueuesFlashSaleChanged`

#### Scenario: The envelope carries identity and ids
- **WHEN** an admin with principal id `u-admin` and request id `req-1` creates a voucher
- **THEN** the unmarshalled envelope has `principal.id` `u-admin`, `request_id` `req-1`, a non-empty `event_id` equal to the row's event id, and a payload that unmarshals to `VoucherChanged`
- **VERIFIED BY**: team-promotion `internal/events` `TestEnvelopeCarriesPrincipalAndRequestID`

#### Scenario: An envelope without a principal omits it
- **WHEN** a voucher is created from a context that holds no principal, and from one whose principal id is empty
- **THEN** both outbox payloads unmarshal to an `EventEnvelope` whose `principal` is nil and whose `payload` is the `VoucherChanged`
- **VERIFIED BY**: team-promotion `internal/events` `TestEnvelopeWithoutPrincipalOmitsIt`

#### Scenario: A call without a request id still gets one
- **WHEN** an admin calls `CreateVoucher` through the running server with no `x-request-id` metadata
- **THEN** the outbox envelope's `request_id` is a non-empty value equal to the id the server logged for the call, and a call that sends `x-request-id` `req-9` stores `req-9`
- **VERIFIED BY**: team-promotion `internal/rpc` `TestCallWithoutRequestIDGetsOneInEnvelope`

#### Scenario: A failed insert leaves no outbox row
- **WHEN** a voucher with an existing code is created
- **THEN** the call returns `AlreadyExists` and the outbox row count is unchanged
- **VERIFIED BY**: team-promotion `internal/store` `TestVoucherInsertFailureLeavesNoOutboxRow`

#### Scenario: A Kafka outage does not fail the write
- **WHEN** the publisher fails every record and an admin creates a voucher, then the publisher recovers
- **THEN** the create call returns OK, the row is unpublished during the outage, and the relayer publishes the record after recovery
- **VERIFIED BY**: team-promotion `internal/app` `TestKafkaOutageKeepsWriteAndRelaysLater`

#### Scenario: The relayed record is keyed by the entity id
- **WHEN** the relayer publishes the row of voucher `V1`
- **THEN** the record is on topic `promotion.events`, its key is the id of `V1`, and its headers include `event-id` and `event-type`
- **VERIFIED BY**: team-promotion `internal/events` `TestRelayedRecordKeyAndHeaders`

#### Scenario: A refused caller writes no outbox row
- **WHEN** a seller without the `admin` scope calls `CreateVoucher` with scope PLATFORM
- **THEN** the call returns `PermissionDenied` and the outbox row count is unchanged
- **VERIFIED BY**: team-promotion `internal/rpc` `TestRefusedCallerWritesNoOutboxRow`

#### Scenario: Redemption RPCs write no event
- **WHEN** the order service reserves, commits and releases a voucher and a flash-sale hold
- **THEN** the outbox row count is unchanged
- **VERIFIED BY**: team-promotion `internal/store` `TestRedemptionWritesNoOutboxRow`

#### Scenario: Old published rows are purged
- **WHEN** the purge component runs with a row published 8 days ago, a row published 6 days ago and an unpublished row
- **THEN** only the 8-day-old published row is deleted
- **VERIFIED BY**: team-promotion `internal/app` `TestOutboxPurgeKeepsRecentAndUnpublished`

### Requirement: The flash-sale kill-switch is evaluated through the SDK flags and fails open

team-promotion SHALL evaluate the flag `flash-sale-enabled` with the SDK `flags` package and SHALL pass `true` as the default. IF Flipt is unreachable at boot or at evaluation, THEN boot SHALL continue and the evaluation SHALL return `true`. There SHALL be no `FEATURE_FLAGS_ENABLED` setting. WHEN the provider reports the flag false, THEN `ReserveFlashSaleStock` SHALL refuse with reason `not_active` and `GetActiveFlashSale` and `ListActiveCampaigns` SHALL return no sale, as today.

#### Scenario: Unreachable Flipt still boots and fails open
- **WHEN** `app.Run` starts with `FLIPT_ADDR` pointing to a closed port and the order service reserves one unit on an active campaign
- **THEN** boot succeeds and the reserve returns `reserved=true`
- **VERIFIED BY**: team-promotion `internal/app` `TestUnreachableFliptBootsAndFailsOpen`

#### Scenario: A provider error returns the default
- **WHEN** the flag provider returns an error for `flash-sale-enabled`
- **THEN** the evaluation returns `true`
- **VERIFIED BY**: team-promotion `internal/flashsale` `TestFlagErrorFailsOpen`

#### Scenario: The kill-switch off refuses a reserve
- **WHEN** the provider serves `flash-sale-enabled=false` and the order service reserves one unit
- **THEN** the call returns `reserved=false` with reason `not_active` and no hold exists
- **VERIFIED BY**: team-promotion `internal/flashsale` `TestKillSwitchOffViaProvider`

### Requirement: Errors reach callers through one error table

`internal/rpc` SHALL convert domain errors with `grpcx.Status` and a service error table; domain packages SHALL NOT import `google.golang.org/grpc/codes`. Unknown errors SHALL become `Internal` with the message `internal error`. SQLSTATE 23505 SHALL become `AlreadyExists` with no constraint or value in the message. A canceled request SHALL return `Canceled`; an expired deadline SHALL return `DeadlineExceeded`. Ids and voucher codes SHALL NOT appear in a client-visible message.

#### Scenario: An unexpected error is a generic internal error
- **WHEN** a repository returns an error whose text contains `pq: connection refused to 10.0.0.5`
- **THEN** the call returns `Internal` with the message `internal error`
- **VERIFIED BY**: team-promotion `internal/rpc` `TestInternalErrorIsGeneric`

#### Scenario: A duplicate voucher code is AlreadyExists
- **WHEN** an admin creates a voucher with code `DUP-1` twice
- **THEN** the second call returns `AlreadyExists` and its message does not contain `DUP-1` or `vouchers_code`
- **VERIFIED BY**: team-promotion `internal/rpc` `TestDuplicateVoucherCodeIsAlreadyExists`

#### Scenario: Cancellation and deadline keep their codes
- **WHEN** a request context is canceled, and another expires its deadline, while a repository call runs
- **THEN** the first returns `Canceled` and the second returns `DeadlineExceeded`
- **VERIFIED BY**: team-promotion `internal/rpc` `TestCanceledAndDeadlineMapToCodes`

#### Scenario: Unknown voucher is NotFound without echoing the code
- **WHEN** `GetVoucher` is called with the code `NOPE-9` that does not exist
- **THEN** the call returns `NotFound` with the message `voucher not found`
- **VERIFIED BY**: team-promotion `internal/rpc` `TestGetVoucherNotFoundDoesNotEchoCode`

#### Scenario: Domain packages do not import grpc codes
- **WHEN** the import lists of `internal/voucher`, `internal/flashsale`, `internal/subscription`, `internal/sponsored` and `internal/listing` non-test files are read
- **THEN** none of them imports `google.golang.org/grpc/codes`
- **VERIFIED BY**: team-promotion `internal/rpc` `TestDomainPackagesDoNotImportGRPCCodes`

### Requirement: Authorization uses SDK auth with the existing rules

Handlers SHALL authorize with `auth.RequirePrincipal`, `auth.RequireScopes` and `auth.RequireService`, and the per-RPC rule SHALL stay exactly as before this change; the migration SHALL NOT widen any RPC. `CreateVoucher` with scope PLATFORM or unspecified SHALL need the exact scope `admin`. `CreateVoucher` with scope SHOP and `CreateCampaign` SHALL need `admin` or `listing.write`. `Subscribe` and `CreateAdCampaign` SHALL need exactly the scope `listing.write`; a principal holding `admin` without `listing.write` SHALL get `PermissionDenied`. `GetEntitlements` SHALL return the caller's own seller by default; a principal whose type is not `service` SHALL get `PermissionDenied` when `seller_id` names another seller, and a `service` principal MAY read any seller. `ValidateAndReserve` SHALL serve a `service` principal holding `promotion.reserve` with any reservation id; any other authenticated principal SHALL be served only when `reservation_id` starts with `preview:<its own id>:`, with `buyer_id` forced to its own id, and otherwise SHALL get `PermissionDenied`. `CommitReservation`, `ReleaseReservation` and the flash hold RPCs SHALL need a principal of type `service` holding the exact scope `promotion.reserve`. A caller with no principal or the anonymous principal SHALL get `Unauthenticated`; a caller without the required scope or type SHALL get `PermissionDenied`.

#### Scenario: Anonymous caller is unauthenticated
- **WHEN** an anonymous principal calls `CreateVoucher` and `ReserveFlashSaleStock`
- **THEN** both calls return `Unauthenticated`
- **VERIFIED BY**: team-promotion `internal/rpc` `TestAnonymousIsUnauthenticated`

#### Scenario: A user holding the service scope is denied
- **WHEN** a principal of type `user` with scope `promotion.reserve` calls `CommitReservation`
- **THEN** the call returns `PermissionDenied`
- **VERIFIED BY**: team-promotion `internal/rpc` `TestUserWithServiceScopeIsDenied`

#### Scenario: A seller cannot create a platform voucher
- **WHEN** a seller with scope `listing.write` creates a voucher with scope PLATFORM
- **THEN** the call returns `PermissionDenied` and no audit event is written
- **VERIFIED BY**: team-promotion `internal/rpc` `TestSellerCannotCreatePlatformVoucher`

#### Scenario: The order service principal is served
- **WHEN** a principal of type `service` with scope `promotion.reserve` calls `ReserveFlashSaleStock`
- **THEN** the call is served and returns the reserve outcome
- **VERIFIED BY**: team-promotion `internal/rpc` `TestOrderServicePrincipalIsServed`

#### Scenario: Public reads need no principal
- **WHEN** a call with no principal reads `GetVoucher`, `ListVouchers`, `GetActiveFlashSale`, `ListActiveCampaigns`, `GetFlashSaleStock`, `ListPlans` and `ListSponsoredSlots`
- **THEN** none of the calls returns `Unauthenticated`
- **VERIFIED BY**: team-promotion `internal/rpc` `TestPublicReadsNeedNoPrincipal`

#### Scenario: A user reserving inside its own preview namespace is served
- **WHEN** a user principal `buyer-1` calls `ValidateAndReserve` with `reservation_id` `preview:buyer-1:abc` and `buyer_id` `someone-else`
- **THEN** the call is served, the response carries the voucher scope, and the stored hold has `buyer_id` `buyer-1`
- **VERIFIED BY**: team-promotion `internal/rpc` `TestValidateAndReservePreviewEchoesScope`

#### Scenario: A user reserving outside its preview namespace is PermissionDenied
- **WHEN** user principal `buyer-1` calls `ValidateAndReserve` once with `reservation_id` `preview:buyer-2:abc` (another user's namespace) and once with `order-1` (no namespace)
- **THEN** both calls return `PermissionDenied` and no hold exists for either id
- **VERIFIED BY**: team-promotion `internal/rpc` `TestValidateAndReservePreviewNamespaceDenied`

#### Scenario: A service principal is not bound to the preview namespace
- **WHEN** a service principal with scope `promotion.reserve` calls `ValidateAndReserve` with `reservation_id` `o1` and `buyer_id` `b1`
- **THEN** the call is served with `buyer_id` `b1` honoured and a hold stored under `o1`
- **VERIFIED BY**: team-promotion `internal/rpc` `TestValidateAndReserveHandlerEchoesScope`

#### Scenario: A service principal without the reserve scope is denied
- **WHEN** a service principal holding only `listing.read` calls `ValidateAndReserve` with `reservation_id` `o1`
- **THEN** the call returns `PermissionDenied` and no hold exists
- **VERIFIED BY**: team-promotion `internal/rpc` `TestValidateAndReserveServiceWithoutScopeDenied`

#### Scenario: Subscribe needs exactly listing.write
- **WHEN** a principal holding `listing.write`, and another holding only `admin`, each call `Subscribe` with a valid plan id
- **THEN** the first call is served and the second returns `PermissionDenied`
- **VERIFIED BY**: team-promotion `internal/rpc` `TestSubscribeAdminWithoutListingWriteDenied`

#### Scenario: CreateAdCampaign needs exactly listing.write
- **WHEN** a principal holding only `admin` calls `CreateAdCampaign`
- **THEN** the call returns `PermissionDenied` and no audit event and no campaign exists
- **VERIFIED BY**: team-promotion `internal/rpc` `TestCreateAdCampaignAdminWithoutListingWriteDenied`

#### Scenario: Entitlements of another seller need a service principal
- **WHEN** a user `seller-1` reads `GetEntitlements` for `seller-2`, and a service principal reads the same
- **THEN** the user call returns `PermissionDenied`, the service call returns OK, and the user's call with no `seller_id` returns OK for `seller-1`
- **VERIFIED BY**: team-promotion `internal/rpc` `TestSubscribeAndEntitlementsAuthz`

### Requirement: Upstream clients run as the right principal and are bounded

The team-domain listing client SHALL forward the caller's principal and request id, SHALL add the scope `listing.read` when absent, SHALL wait for a ready connection, and SHALL be bounded by the SDK 2 s call bound (no service setting changes it). `UPSTREAM_DOMAIN_ADDR` SHALL be required in every `ENV`, so no code path runs without a listing client. The team-audit client SHALL send the service principal `service-team-promotion` of type `service` with scope `audit.write` and never the admin's principal, and its write SHALL be bounded by the 2 s `auditx` bound. `UPSTREAM_AUDIT_ADDR` SHALL be required in every `ENV`, so an audit writer always exists.

#### Scenario: Listing lookup runs as the caller with listing.read
- **WHEN** a seller principal with scope `listing.write` triggers a listing ownership lookup
- **THEN** the outgoing metadata carries the seller's id and the scopes `listing.write,listing.read`
- **VERIFIED BY**: team-promotion `internal/upstream` `TestListingLookupForwardsCallerWithListingRead`

#### Scenario: A slow listing lookup is unavailable
- **WHEN** team-domain does not answer within the 2 s SDK bound
- **THEN** the ownership check returns `Unavailable` with the message `listing lookup failed` within 2.5 s
- **VERIFIED BY**: team-promotion `internal/upstream` `TestListingLookupTimeoutIsUnavailable`

#### Scenario: Audit is written as the service principal
- **WHEN** the audit writer sends `WriteAuditEvent` while the context holds an admin principal
- **THEN** the outgoing metadata principal is `service-team-promotion`, type `service`, scopes `audit.write`, and the admin id is absent
- **VERIFIED BY**: team-promotion `internal/upstream` `TestAuditClientUsesServicePrincipal`

#### Scenario: A slow audit write is bounded
- **WHEN** team-audit does not answer a write
- **THEN** the gate returns `Unavailable` "audit trail unavailable" within 2.5 s
- **VERIFIED BY**: team-promotion `internal/upstream` `TestAuditClientTimeoutBounded`

### Requirement: Page cursors keep their format

`ListVouchers` and `ListActiveCampaigns` SHALL keep the cursor as the decimal row offset, SHALL treat a malformed or negative cursor as offset 0, and SHALL replace a page size that is not positive or is above 200 with the default page size 50.

#### Scenario: Page cursor is the plain offset
- **WHEN** a seller owns 120 vouchers and lists with `page_size=50`, then with the returned cursor
- **THEN** the first page has 50 items and cursor `"50"`, the second has 50 items and cursor `"100"`, and a third has 20 items and an empty cursor
- **VERIFIED BY**: team-promotion `internal/voucher` `TestListVouchersCursorIsPlainOffset`

#### Scenario: Oversized page size falls back to the default
- **WHEN** a list call passes `page_size=201`
- **THEN** the page holds at most 50 items
- **VERIFIED BY**: team-promotion `internal/voucher` `TestListVouchersOversizedPageUsesDefault`

#### Scenario: A malformed cursor reads the first page
- **WHEN** a list call passes the cursor `not-a-number`
- **THEN** the page starts at offset 0
- **VERIFIED BY**: team-promotion `internal/voucher` `TestListVouchersMalformedCursorIsOffsetZero`

### Requirement: Spec citations stay traceable after the package move

The packages `internal/service`, `internal/handler`, `internal/repository`, `internal/producer`, `internal/interceptor`, `internal/grpcserver`, `internal/bootstrap` and `internal/featureflags` SHALL NOT exist after this change. Every test name cited for team-promotion in `openspec/specs/**` and in non-archived `openspec/changes/*` SHALL exist as a `func Test` in `team-promotion`, except the names this change lists as deliberately replaced (`TestDeclaredEnvKeysNoDrift`, `TestAuditAddrDeclaredEnvKey`, `TestNewAuditClientEmptyAddrNil`, `TestCreateVoucherNilAuditUnavailable`, `TestCreateVoucherShopAdminNilAuditUnavailable`, `TestCreateVoucherPlatformSellerDeniedNilAudit` and the `TestWithTimeout*` tests of the removed hand-rolled timeout interceptor). Every test name cited by this change SHALL exist in the package it cites.

#### Scenario: Every cited test name exists
- **WHEN** the citation check scans `openspec/specs/**` and non-archived `openspec/changes/*` for `team-promotion` `internal/<pkg>` `Test...` citations
- **THEN** each name is found as a test function in `team-promotion`, and each name cited by this change is found in the cited package, and the replaced names are the only exceptions; a cited name that is not found makes the check fail and names it
- **VERIFIED BY**: team-promotion `internal/docs` `TestSpecCitedTestsExist`

#### Scenario: Removed packages are gone
- **WHEN** the directory list of `team-promotion/internal` is read
- **THEN** none of the removed package names is present
- **VERIFIED BY**: team-promotion `internal/docs` `TestRemovedPackagesAreGone`
