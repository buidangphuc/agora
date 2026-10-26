## ADDED Requirements

### Requirement: team-search loads its settings through the SDK and declares no dependency enable flag

team-search SHALL load the settings of the query server (`config.Load`) and of the indexer (`config.LoadIndexer`) with the SDK `config.Load`. `ENV` SHALL be required and `GRPC_PORT` SHALL be required for the server. Neither settings struct SHALL declare `DATABASE_ENABLED`, `KAFKA_ENABLED`, `SHUTDOWN_GRACE_SECONDS` or any key ending in `_ENABLED` other than `OTEL_ENABLED` and `GRPC_REFLECTION_ENABLED`. `GRPC_REFLECTION_ENABLED` SHALL default to false and `Load` SHALL refuse true unless `ENV` is `local` or `test`, before any group is validated. The server SHALL require `DATABASE_URL` in every `ENV`; the indexer SHALL NOT require it. `OPENSEARCH_URL` SHALL be required by both settings structs. The indexer SHALL declare `SHUTDOWN_GRACE` (Go duration, default `10s`, checked positive) because it has no gRPC group. The OpenTelemetry service name SHALL default to `team-search` for the server and `team-search-indexer` for the indexer when `OTEL_SERVICE_NAME` is unset. `.env.example` at the service root SHALL list exactly the keys the two settings structs declare.

#### Scenario: An unset ENV refuses to load

- **WHEN** `config.Load` and `config.LoadIndexer` run with every variable set except `ENV`
- **THEN** each returns an error that names `ENV`
- **VERIFIED BY**: `team-search/internal/config` `TestLoadRequiresEnv`

#### Scenario: A missing gRPC port refuses to load the server

- **WHEN** `config.Load` runs with `ENV=local` and `DATABASE_URL` set but no `GRPC_PORT`
- **THEN** it returns an error that names `GRPC_PORT`
- **VERIFIED BY**: `team-search/internal/config` `TestServerLoadRequiresGRPCPort`

#### Scenario: Retired enable flags are not declared

- **WHEN** the env keys declared by the server and indexer settings structs are listed
- **THEN** the list holds none of `DATABASE_ENABLED`, `KAFKA_ENABLED` and `SHUTDOWN_GRACE_SECONDS`, and no key ending in `_ENABLED` other than `OTEL_ENABLED` and `GRPC_REFLECTION_ENABLED`
- **VERIFIED BY**: `team-search/internal/config` `TestNoDependencyEnableFlags`

#### Scenario: .env.example matches the settings

- **WHEN** `.env.example` is compared with the keys of both settings structs
- **THEN** no declared key is missing from the file and no key in the file is undeclared
- **VERIFIED BY**: `team-search/internal/config` `TestEnvExampleInSync`

#### Scenario: Reflection is off by default

- **WHEN** `config.Load` runs with `ENV=local` and no `GRPC_REFLECTION_ENABLED`
- **THEN** `ReflectionEnabled` is false
- **VERIFIED BY**: `team-search/internal/config` `TestReflectionDefaultsOff`

#### Scenario: Reflection is refused in a strict environment

- **WHEN** `config.Load` runs with `ENV=dev` (any value other than `local` or `test`) and `GRPC_REFLECTION_ENABLED=true`, and also with `DATABASE_URL` unset
- **THEN** the returned error names `GRPC_REFLECTION_ENABLED` and does not name `DATABASE_URL`
- **VERIFIED BY**: `team-search/internal/config` `TestLoadRefusesReflectionInStrictEnv`

#### Scenario: The server requires a database URL in every environment

- **WHEN** `config.Load` runs with `ENV=local`, `GRPC_PORT=50052` and no `DATABASE_URL`
- **THEN** it returns an error that names `DATABASE_URL`
- **VERIFIED BY**: `team-search/internal/config` `TestServerLoadRequiresDatabaseURL`

#### Scenario: The indexer loads without a database

- **WHEN** `config.LoadIndexer` runs with `ENV=local`, `UPSTREAM_AI_ADDR` set and neither `DATABASE_URL` nor `GRPC_PORT`
- **THEN** it returns settings and a nil error
- **VERIFIED BY**: `team-search/internal/config` `TestIndexerLoadNeedsNoDatabase`

#### Scenario: An empty OPENSEARCH_URL refuses to load

- **WHEN** `config.Load` and `config.LoadIndexer` each run with `ENV=local` and every other required key set but `OPENSEARCH_URL` empty
- **THEN** each returns an error that names `OPENSEARCH_URL`
- **VERIFIED BY**: `team-search/internal/config` `TestValidateRequiresOpenSearchURL`

#### Scenario: An unset SHUTDOWN_GRACE defaults to 10s for the indexer

- **WHEN** `config.LoadIndexer` runs with `ENV=local`, `UPSTREAM_AI_ADDR` and `OPENSEARCH_URL` set and no `SHUTDOWN_GRACE`, and again with `SHUTDOWN_GRACE=0s`
- **THEN** the first returns `ShutdownGrace` equal to 10 seconds and the second returns an error that names `SHUTDOWN_GRACE`
- **VERIFIED BY**: `team-search/internal/config` `TestIndexerShutdownGraceDefaultsAndValidates`

#### Scenario: The OpenTelemetry service name defaults per binary

- **WHEN** `config.Load` and `config.LoadIndexer` run with `ENV=local`, their required keys set and no `OTEL_SERVICE_NAME`
- **THEN** the server settings carry service name `team-search` and the indexer settings carry `team-search-indexer`
- **VERIFIED BY**: `team-search/internal/config` `TestOTelServiceNameDefaultsPerBinary`

### Requirement: The server binary migrates through a migrate up subcommand and never at boot

`/server migrate up` SHALL load `config.LoadMigrate` (`ENV` and `DATABASE_URL` required; no `GRPC_PORT`, OpenSearch or Kafka key), apply the embedded migrations through the SDK `migrate.Up`, and exit 0, or exit non-zero with a message naming the failure. `app.Run` SHALL NOT apply migrations. The indexer SHALL have no `migrate` subcommand.

#### Scenario: migrate up applies the embedded migrations
- **GIVEN** an empty Postgres database
- **WHEN** `app.Migrate` runs with `ENV=local` and that `DATABASE_URL`, and then runs a second time
- **THEN** the table `saved_searches` exists after the first run, both runs return nil, and the second run changes no row of the migration version table
- **VERIFIED BY**: `team-search/internal/app` `TestMigrateUpAppliesEmbeddedMigrations`

#### Scenario: migrate up with an unreachable database fails
- **WHEN** `app.Migrate` runs with a `DATABASE_URL` that points at a closed port
- **THEN** it returns an error within 10 seconds and the `/server migrate up` process exits non-zero
- **VERIFIED BY**: `team-search/internal/app` `TestMigrateUpFailsOnUnreachableDatabase`

#### Scenario: migrate up needs only ENV and DATABASE_URL
- **WHEN** `config.LoadMigrate` runs with `ENV=production` and `DATABASE_URL` set, and no `GRPC_PORT`, `OPENSEARCH_URL` or `KAFKA_BROKERS`; and again with `DATABASE_URL` unset
- **THEN** the first returns settings and a nil error and the second returns an error that names `DATABASE_URL`
- **VERIFIED BY**: `team-search/internal/config` `TestLoadMigrateNeedsOnlyEnvAndDatabaseURL`

#### Scenario: The server does not migrate at boot
- **GIVEN** an empty Postgres database and a reachable OpenSearch stub
- **WHEN** `app.Run` starts
- **THEN** the table `saved_searches` does not exist after boot
- **VERIFIED BY**: `team-search/internal/app` `TestRunDoesNotMigrate`

### Requirement: The query server boots only with Postgres and OpenSearch reachable

`app.Run` SHALL open Postgres with `postgres.Open` and ensure the OpenSearch index before it listens. When either is unreachable, `Run` SHALL return an error and the process SHALL exit non-zero, in every `ENV`. The non-test sources of team-search SHALL contain no in-memory saved-search store, no `RequireDurableStorage` and no `DATABASE_ENABLED`.

#### Scenario: An unreachable database fails boot in a local environment

- **WHEN** `app.Run` starts with `ENV=local` and a `DATABASE_URL` that points at a closed port
- **THEN** it returns an error that wraps the postgres open failure within 10 seconds and binds no gRPC listener
- **VERIFIED BY**: `team-search/internal/app` `TestRunFailsOnUnreachableDatabase` (subtest `local`)

#### Scenario: An unreachable database fails boot in production

- **WHEN** `app.Run` starts with `ENV=production` and a `DATABASE_URL` that points at a closed port
- **THEN** it returns an error that wraps the postgres open failure within 10 seconds
- **VERIFIED BY**: `team-search/internal/app` `TestRunFailsOnUnreachableDatabase` (subtest `production`); `platform-e2e` `ops/boot_guard_search.feature` "team-search refuses to boot without a reachable database in production"

#### Scenario: An unreachable OpenSearch fails boot

- **WHEN** `app.Run` starts with a reachable Postgres and an `OPENSEARCH_URL` that points at a closed port
- **THEN** it returns an error that wraps the ensure-index failure and binds no gRPC listener
- **VERIFIED BY**: `team-search/internal/app` `TestRunFailsOnUnreachableOpenSearch`

#### Scenario: No runtime in-memory fallback exists

- **WHEN** the non-test Go sources of team-search are scanned for the identifiers `InMemorySavedSearchRepository`, `RequireDurableStorage` and the string `DATABASE_ENABLED`
- **THEN** the scan finds none
- **VERIFIED BY**: `team-search/internal/config` `TestNoRuntimeInMemoryFallback`

### Requirement: Readiness follows Postgres and OpenSearch while liveness stays independent

The query server SHALL register `postgres` and `opensearch` checkers with `healthx`. The overall gRPC health status (service name empty) SHALL be NOT_SERVING within one `healthx.DefaultInterval` after a checker fails and SERVING again within one interval after it recovers. The `liveness` service SHALL stay SERVING while a checker fails. Kafka SHALL NOT be a readiness check (the query server has no Kafka dependency). On shutdown the server SHALL drain health before it stops accepting calls, which sets both names NOT_SERVING.

#### Scenario: A failing dependency turns readiness off

- **WHEN** the OpenSearch checker starts failing while the server runs
- **THEN** a health `Check` of the empty service name answers NOT_SERVING within 5 seconds
- **VERIFIED BY**: `team-search/internal/app` `TestReadinessFollowsDependencies`

#### Scenario: Liveness stays serving while a dependency is down

- **WHEN** the Postgres checker is failing
- **THEN** a health `Check` of the service `liveness` answers SERVING
- **VERIFIED BY**: `team-search/internal/app` `TestLivenessSurvivesDependencyOutage`

#### Scenario: Readiness registers no Kafka checker
- **WHEN** the checker names registered with `healthx` by `app.Run` are listed
- **THEN** the list is exactly `postgres` and `opensearch`
- **VERIFIED BY**: `team-search/internal/app` `TestReadinessCheckersArePostgresAndOpenSearch`

#### Scenario: Shutdown drains health, then stops serving

- **WHEN** the context given to `app.Run` is cancelled
- **THEN** `Run` returns nil within `SHUTDOWN_GRACE` and a health `Check` made after the cancel answers NOT_SERVING or the connection is closed, never SERVING
- **VERIFIED BY**: `team-search/internal/app` `TestShutdownDrainsHealth`

#### Scenario: An unknown health service name is not found

- **WHEN** a health `Check` asks for the service `nope`
- **THEN** the call fails with `NotFound`
- **VERIFIED BY**: `team-search/internal/app` `TestUnknownHealthServiceNotFound`

### Requirement: gRPC reflection is served only when enabled

The query server SHALL register the gRPC reflection service only when `GRPC_REFLECTION_ENABLED` is true. With the default it SHALL NOT answer reflection calls.

#### Scenario: Reflection is not served by default

- **WHEN** a client calls `grpc.reflection.v1.ServerReflection` on a server built from default settings
- **THEN** the call fails with `Unimplemented`
- **VERIFIED BY**: `team-search/internal/app` `TestReflectionNotServedByDefault`

#### Scenario: Reflection is served locally when enabled

- **WHEN** a client calls `grpc.reflection.v1.ServerReflection` on a server built with `ENV=local` and `GRPC_REFLECTION_ENABLED=true`
- **THEN** the call lists `platform.search.v1.SearchService`
- **VERIFIED BY**: `team-search/internal/app` `TestReflectionServedWhenEnabled`

### Requirement: Saved searches are stored in Postgres and belong to one signed-in user

The four saved-search RPCs (`SaveSearch`, `ListSavedSearches`, `DeleteSavedSearch`, `RunSavedSearch`) SHALL use the Postgres store only. Every store call SHALL be scoped by the caller's user id, so a saved search of another user is indistinguishable from a missing one and returns `NotFound`. Identity SHALL be checked before scopes: a missing or anonymous principal (type anonymous, unspecified, or id `anonymous`) gets `Unauthenticated`, a service principal gets `PermissionDenied`, and a user principal missing `search:read` (list, run) or `search:write` (save, delete) gets `PermissionDenied`. `ListSavedSearches` SHALL return newest first (`created_at DESC, id DESC`), default page size 10, maximum 50, with the next cursor the decimal string of the next offset. `SaveSearch` SHALL reject a save that has neither a query nor any filter with `InvalidArgument`.

#### Scenario: A saved search survives a restart

- **GIVEN** a user saved a search through a store opened on a Postgres database
- **WHEN** a second store instance is opened on the same database and lists that user's saved searches
- **THEN** the list holds that saved search with the saved query and filters
- **VERIFIED BY**: `team-search/internal/store` `TestSavedSearchesPersistAcrossStores`

#### Scenario: Saved searches list newest first with a total

- **GIVEN** a user saved three searches, one after another
- **WHEN** the user lists with page size 2
- **THEN** the page holds the newest two in descending order, the total is 3 and the next cursor is `2`
- **VERIFIED BY**: `team-search/internal/rpc` `TestSavedSearchListNewestFirstWithCursor`

#### Scenario: A page size above the maximum is clamped

- **GIVEN** a user saved 60 searches
- **WHEN** the user lists with page size 500
- **THEN** the page holds 50 items and the next cursor is `50`
- **VERIFIED BY**: `team-search/internal/rpc` `TestSavedSearchListClampsPageSize`

#### Scenario: Another user's saved search is not found

- **GIVEN** user A saved a search
- **WHEN** user B calls `DeleteSavedSearch` and then `RunSavedSearch` with A's saved-search id
- **THEN** both calls fail with `NotFound` and A's saved search still exists
- **VERIFIED BY**: `team-search/internal/rpc` `TestSavedSearchOwnerIsolation`; `team-search/internal/store` `TestStoreScopesEveryCallByUser`

#### Scenario: An anonymous caller is unauthenticated whatever its scopes

- **WHEN** an anonymous principal that holds `search:read` and `search:write` calls each of the four RPCs
- **THEN** each call fails with `Unauthenticated` and the store is not called
- **VERIFIED BY**: `team-search/internal/rpc` `TestSavedSearchCallerIdentity_Matrix`

#### Scenario: A service principal is denied

- **WHEN** a service principal that holds `search:read` and `search:write` calls each of the four RPCs
- **THEN** each call fails with `PermissionDenied` and the store is not called
- **VERIFIED BY**: `team-search/internal/rpc` `TestSavedSearchCallerIdentity_Matrix`

#### Scenario: A user without the scope is denied

- **WHEN** a signed-in user that holds only `search:read` calls `SaveSearch`, and a user that holds only `search:write` calls `ListSavedSearches`
- **THEN** both calls fail with `PermissionDenied`
- **VERIFIED BY**: `team-search/internal/rpc` `TestSavedSearchScopes_AfterIdentity`

#### Scenario: A signed-in user with both scopes succeeds

- **WHEN** a signed-in user that holds `search:read` and `search:write` saves, lists, runs and deletes a search
- **THEN** every call succeeds and the list is empty after the delete
- **VERIFIED BY**: `team-search/internal/rpc` `TestSavedSearch_UserWithScopesSucceeds`

#### Scenario: An empty save is rejected

- **WHEN** a signed-in user calls `SaveSearch` with an empty query and no `filters_json`
- **THEN** the call fails with `InvalidArgument` and the store holds no new row
- **VERIFIED BY**: `team-search/internal/rpc` `TestSaveSearchRejectsEmpty`

#### Scenario: A store failure returns a generic internal error

- **WHEN** the store returns the error `connection reset by peer` during `ListSavedSearches`
- **THEN** the call fails with `Internal` and the message `internal error`, which does not contain `connection reset`
- **VERIFIED BY**: `team-search/internal/rpc` `TestStoreFailureReturnsGenericInternalError`

### Requirement: SearchListings and Suggest admit anonymous principals that hold search:read

`SearchListings` and `Suggest` SHALL gate with `auth.RequireScopesAllowAnonymous(ctx, "search:read")`, so the gateway's anonymous principal that holds `search:read` browses published listings, while a principal without that scope gets `PermissionDenied`. They SHALL NOT use `auth.RequireScopes`, which rejects anonymous principals. Draft and hidden visibility rules (`effectiveFilters`) stay unchanged and run after the gate.

#### Scenario: Anonymous browsing is not blocked by the SDK gate

- **WHEN** `SearchListings` is called with an anonymous principal holding `search:read` against a fake index
- **THEN** the index receives the call with a published-only filter
- **VERIFIED BY**: `team-search/internal/rpc` `TestSearchAnonymousWithScopeSucceeds`

### Requirement: Request id and trace context are carried by the SDK interceptors

The query server SHALL echo or generate `x-request-id` and continue an incoming W3C `traceparent` through the `grpcx` interceptors, and SHALL emit one `grpc.request` log line per call with method, code, duration, request id and principal id (never the request body).

#### Scenario: A request id is logged and a trace is continued

- **WHEN** a client calls `SearchListings` with metadata `x-request-id: req-42` and a valid `traceparent`
- **THEN** the call log line carries `request_id` `req-42`, the active span in the handler has the incoming trace id, and the log line holds no query text
- **VERIFIED BY**: `team-search/internal/app` `TestRequestIDAndTraceparentPropagate`

#### Scenario: A malformed traceparent does not fail the call

- **WHEN** a client calls `SearchListings` with no `x-request-id` and a malformed `traceparent`
- **THEN** the call is not rejected, a generated request id appears in the log line, and a new trace is started
- **VERIFIED BY**: `team-search/internal/app` `TestRequestIDGeneratedAndBadTraceparentIgnored`

### Requirement: Query handlers map errors through one table

Handlers of the query server SHALL return domain errors through `grpcx.Status` with one `ErrorTable`: `store.ErrSavedSearchNotFound` maps to `NotFound` with the message `saved search not found`; a status error that already carries a code (visibility `InvalidArgument`, `Unauthenticated`, `PermissionDenied`) SHALL pass unchanged; `context.Canceled` maps to `Canceled` and `context.DeadlineExceeded` to `DeadlineExceeded`; any other error maps to `Internal` with the message `internal error`.

#### Scenario: A search backend failure returns a generic internal error

- **WHEN** the OpenSearch adapter returns an error whose text contains the index host name during `SearchListings`
- **THEN** the call fails with `Internal` and the message `internal error`
- **VERIFIED BY**: `team-search/internal/rpc` `TestSearchBackendFailureReturnsGenericInternalError`

#### Scenario: A cancelled request returns Canceled

- **WHEN** the client cancels `SearchListings` while the adapter is waiting
- **THEN** the call fails with `Canceled`
- **VERIFIED BY**: `team-search/internal/rpc` `TestContextCanceledMapsToCanceled`

#### Scenario: A draft filter by a stranger keeps its visibility error

- **WHEN** a signed-in user searches with `filters["status"]="draft"` and a `seller_id` that is not their own
- **THEN** the call fails with `PermissionDenied`
- **VERIFIED BY**: `team-search/internal/rpc` `TestVisibilityErrorPassesThroughStatusTable`

#### Scenario: SearchListings called with no principal metadata

- **WHEN** `SearchListings` is called with no principal metadata at all (the gateway always sends one, so this is a direct call that bypassed it)
- **THEN** the call fails with code `Unauthenticated`
- **VERIFIED BY**: `team-search/internal/app` `TestSearch_Unauthenticated` (existing behaviour)

#### Scenario: An anonymous principal holding search:read can search and get suggestions

- **WHEN** `SearchListings` and `Suggest` are called with the gateway's anonymous principal (type anonymous) holding the scope `search:read`
- **THEN** both calls succeed and `SearchListings` returns only published listings
- **VERIFIED BY**: `team-search/internal/rpc` `TestSearchAnonymousWithScopeSucceeds` and `TestSuggestAnonymousWithScopeSucceeds`; `team-search/internal/app` `TestSearch_AnonymousWithScope` (over HTTP-equivalent gRPC from the gateway path); FEATURES `search.visibility-hides-drafts`

#### Scenario: An anonymous principal asking for status=draft keeps its visibility error

- **WHEN** the anonymous principal holding `search:read` calls `SearchListings` with `filters["status"]="draft"`
- **THEN** the call fails with the same visibility error as before this change (`Unauthenticated` for a draft filter by a caller without an identity) and no draft is returned
- **VERIFIED BY**: `team-search/internal/rpc` `TestSearchAnonymousDraftFilterKeepsVisibilityError`

#### Scenario: A search without the search scope is denied

- **WHEN** `SearchListings` or `Suggest` is called by a user principal whose only scope is `other.scope`, or by an anonymous principal that does not hold `search:read`
- **THEN** the call fails with code `PermissionDenied`
- **VERIFIED BY**: `team-search/internal/app` `TestSearch_InsufficientScope` (existing behaviour, also FEATURES `search.visibility-foreign-drafts-403`)

### Requirement: The indexer consumes listing events through the SDK consumer

The indexer SHALL consume `KAFKA_LISTING_TOPIC` (default `listing.events`) in group `KAFKA_CONSUMER_GROUP` (default `team-search-indexer`) with `consume.NewConsumer`. A handler error SHALL be retried 5 times (100 ms doubling to 30 s), after which the record SHALL be published unchanged in key and value to `<topic>.dlq`. When the DLQ publish fails the indexer SHALL NOT commit the offset past the record and SHALL return an error. Offsets SHALL commit once per fetch. A `ListingStockChanged` with an empty `listing_id`, a negative `stock` or no `occurred_at` SHALL be retried and parked, never acknowledged silently.

#### Scenario: A malformed stock event is retried and parked

- **WHEN** the indexer consumes a `ListingStockChanged` with a negative `stock` keyed `listing-1`
- **THEN** the handler runs 5 times and `listing.events.dlq` receives one record with key `listing-1` and the original value
- **VERIFIED BY**: `team-search/internal/app` `TestIndexerParksMalformedStockEvent`

#### Scenario: A listing event updates the read-model

- **WHEN** the indexer consumes a valid `ListingChanged` for a published listing
- **THEN** the read-model fake holds the document with the event's version
- **VERIFIED BY**: `team-search/internal/events` `TestListingEventHandler_VectorizesPublishedListing`

#### Scenario: A failing DLQ publish does not advance the offset

- **GIVEN** a handler that always fails and a DLQ producer that always fails
- **WHEN** the indexer consumes one record and then a new consumer starts in the same group
- **THEN** the first run returns an error and the new consumer receives the same record again
- **VERIFIED BY**: `team-search/internal/app` `TestIndexerDLQFailureDoesNotAdvanceOffset`

#### Scenario: A recoverable handler error does not park the record

- **WHEN** the handler fails twice for a record and then succeeds
- **THEN** `listing.events.dlq` receives no record and the offset is committed
- **VERIFIED BY**: `team-search/internal/app` `TestIndexerRecoversBeforeDLQ`

### Requirement: The indexer logs a key fingerprint, never the raw record key

The indexer SHALL log `key_fp`, the first 12 lowercase hex characters of HMAC-SHA256 of the record key under `LOG_KEY_FP_SECRET`, wherever it previously logged the record key, and SHALL NOT write the raw key to any log line. In a strict `ENV` `LoadIndexer` SHALL refuse an unset or empty `LOG_KEY_FP_SECRET`. An empty record key SHALL still log a `key_fp`.

#### Scenario: Retry and park logs carry the fingerprint

- **WHEN** a handler fails permanently for a record keyed `listing-123` and the record is parked
- **THEN** every captured log line has no occurrence of `listing-123` and the park line has `key_fp` equal to the HMAC fingerprint of `listing-123`
- **VERIFIED BY**: `team-search/internal/app` `TestIndexerLogsKeyFingerprintOnly`

#### Scenario: An empty key is still fingerprinted

- **WHEN** a handler fails for a record with a nil key
- **THEN** the log line has a non-empty 12-hex `key_fp`
- **VERIFIED BY**: `team-search/internal/app` `TestIndexerEmptyKeyIsFingerprinted`

#### Scenario: A strict environment refuses a missing fingerprint secret

- **WHEN** `LoadIndexer` runs with `ENV=production` and no `LOG_KEY_FP_SECRET`
- **THEN** it returns an error that names `LOG_KEY_FP_SECRET`
- **VERIFIED BY**: `team-search/internal/config` `TestIndexerLoadRequiresLogKeyFPSecretInStrictEnv`

### Requirement: The indexer needs Kafka and team-ai and classifies as a service principal

`LoadIndexer` SHALL require `UPSTREAM_AI_ADDR` in every `ENV` and `KAFKA_BROKERS` when `ENV` is strict. The classifier SHALL call `AIService.ClassifyTags` through `upstream.NewClient` as service principal `service-team-search` with the single scope `ai.classify` and a 2 second timeout. A classifier error SHALL NOT fail ingestion: the document is written with `tags_pending` and keeps the tags it already had.

#### Scenario: A missing team-ai address refuses boot

- **WHEN** `LoadIndexer` runs with `ENV=local` and no `UPSTREAM_AI_ADDR`
- **THEN** it returns an error that names `UPSTREAM_AI_ADDR`
- **VERIFIED BY**: `team-search/internal/config` `TestIndexerLoadRequiresAIAddr`

#### Scenario: Strict environments need broker addresses

- **WHEN** `LoadIndexer` runs with `ENV=staging` and no `KAFKA_BROKERS`
- **THEN** it returns an error that names `KAFKA_BROKERS`
- **VERIFIED BY**: `team-search/internal/config` `TestIndexerLoadRequiresBrokersInStrictEnv`

#### Scenario: The classifier call carries the service principal

- **WHEN** the classifier classifies a listing against a test team-ai server
- **THEN** the server sees `x-principal-id` `service-team-search`, `x-principal-type` `service` and `x-principal-scopes` `ai.classify`
- **VERIFIED BY**: `team-search/internal/taxonomy` `TestClassifierCallCarriesServicePrincipal`

#### Scenario: The classifier call never carries an end-user principal

- **WHEN** the indexer classifies a listing while a test team-ai server records the metadata of the call
- **THEN** the recorded principal type is `service` and the only recorded scope is `ai.classify`
- **VERIFIED BY**: `team-search/internal/taxonomy` `TestClassifierCallCarriesServicePrincipal`

#### Scenario: A classifier outage does not fail ingestion

- **WHEN** the classifier returns `Unavailable` while a `ListingChanged` is handled
- **THEN** the handler returns nil and the document is written with `tags_pending` true
- **VERIFIED BY**: `team-search/internal/events` `TestListingEventHandler_ClassifierOutageDoesNotFailIngestion`

### Requirement: The indexer stops on shutdown and purges tombstones on a ticker

On context cancel `app.RunIndexer` SHALL stop the consumer and the tombstone ticker and return nil within `SHUTDOWN_GRACE` (`IndexerSettings.ShutdownGrace`, default 10 s). The tombstone ticker SHALL purge tombstones older than `TOMBSTONE_TTL` every `TOMBSTONE_PURGE_INTERVAL`, starting one interval after boot, and a failed purge SHALL be logged and retried at the next tick without stopping the indexer.

#### Scenario: The indexer stops when cancelled

- **WHEN** the context given to `app.RunIndexer` is cancelled while the consumer waits for records
- **THEN** `RunIndexer` returns nil within `SHUTDOWN_GRACE`
- **VERIFIED BY**: `team-search/internal/app` `TestRunIndexerStopsOnCancel`

#### Scenario: A failed purge does not stop the indexer

- **WHEN** the purger returns an error on the first tick and succeeds on the second
- **THEN** the second tick runs and no error reaches `RunIndexer`
- **VERIFIED BY**: `team-search/internal/tombstone` `TestPurge_FailureIsLoggedNotFatal`

### Requirement: team-search is built from the tagged SDK and ships both binaries

`team-search/go.mod` SHALL require `github.com/buidangphuc/agora/platform-core/packages/go-sdk` at `v1.0.2`, SHALL contain no `replace` directive for it and SHALL pin golangci-lint with a `tool` directive. The repository SHALL contain no `third_party` directory. The `Makefile` SHALL include `../platform-core/make/go-service.mk` and the lint config `../platform-core/packages/go-sdk/.golangci.yml` by reference. The image SHALL contain `/server` and `/indexer`, with entrypoint `/server`.

#### Scenario: go.mod requires the tag without a replace

- **WHEN** `team-search/go.mod` is parsed
- **THEN** it requires the SDK at `v1.0.2`, has no `replace` for the SDK and has a golangci-lint `tool` directive
- **VERIFIED BY**: `team-search/internal/config` `TestGoModRequiresTaggedSDK`

#### Scenario: A vendored SDK copy is rejected

- **WHEN** the file tree of `team-search` is scanned
- **THEN** no `third_party` directory and no local `.golangci.yml` exists
- **VERIFIED BY**: `team-search/internal/config` `TestNoVendoredSDKOrLintCopy`

#### Scenario: The image builds both binaries

- **WHEN** `docker build` runs in `team-search`
- **THEN** the image holds `/server` and `/indexer` and its entrypoint is `/server`
- **VERIFIED BY**: `team-search/internal/config` `TestDockerfileBuildsBothBinaries` (reads the Dockerfile) and the `docker build` command of the last code task
