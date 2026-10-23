## ADDED Requirements

### Requirement: team-analytics builds on the tagged go-sdk with shared tooling

The `team-analytics` module SHALL require `github.com/buidangphuc/agora/platform-core/packages/go-sdk` at tag `v1.0.2` with no `replace` directive and no `third_party` directory, and SHALL pin `golangci-lint` as a `tool` directive.

#### Scenario: go.mod requires the SDK tag
- **WHEN** `go.mod` of team-analytics is parsed
- **THEN** it has exactly one require of the SDK module at `v1.0.2`, no `replace` directive and a `tool` directive for golangci-lint
- **VERIFIED BY**: team-analytics `internal/app` `TestGoModRequiresSDKTagWithoutReplace`

#### Scenario: A vendored SDK copy is refused
- **WHEN** the team-analytics tree is inspected for `third_party/`, a service-local `.golangci.yml` and a copy of `go-service.mk`
- **THEN** none exists
- **VERIFIED BY**: team-analytics `internal/app` `TestNoVendoredSDKOrLocalLintCopy`

### Requirement: The Makefile uses the shared service makefile and lint config

The `Makefile` SHALL include `../platform-core/make/go-service.mk` and set `LINT_CONFIG` to `../platform-core/packages/go-sdk/.golangci.yml` without a service-local copy of either file, and the standalone compose file SHALL load under a local `ENV`.

#### Scenario: The Makefile references the shared files
- **WHEN** the `Makefile` is read
- **THEN** it contains `include ../platform-core/make/go-service.mk` and `LINT_CONFIG := ../platform-core/packages/go-sdk/.golangci.yml`, and sets `SERVICE`, `GRPC_PORT` and `BINARIES` to `consumer ga4-relay ga4import`
- **VERIFIED BY**: team-analytics `internal/app` `TestMakefileReferencesSharedFiles`

#### Scenario: The standalone compose file boots under a local ENV
- **WHEN** `docker-compose.local.yaml` of team-analytics is parsed and its `team-analytics` environment is passed to `config.Load`
- **THEN** it declares `ENV=local` and `GRPC_PORT=50059`, declares no `KAFKA_ENABLED`, every key it declares is a key `Settings` binds, and `Load` returns no error
- **VERIFIED BY**: team-analytics `internal/app` `TestStandaloneComposeLoadsUnderLocalEnv`

### Requirement: Binaries only call the composition root

Each binary (`consumer`, `ga4-relay`, `ga4import`) SHALL be a `main.go` that only loads settings and calls `internal/app` or `internal/ga4import`.

#### Scenario: A binary that imports a warehouse, query, events or franz-go package is not accepted
- **WHEN** the imports of `cmd/consumer/main.go`, `cmd/ga4-relay/main.go` and `cmd/ga4import/main.go` are listed
- **THEN** none imports `internal/warehouse`, `internal/query`, `internal/events` or franz-go
- **VERIFIED BY**: team-analytics `internal/app` `TestCmdMainsOnlyCallApp`

### Requirement: Settings come from the SDK groups, ENV is required and strict environments require secrets

`config.Load` SHALL bind `Settings` from the environment through the SDK, SHALL fail when `ENV` is unset or empty, SHALL require `KAFKA_BROKERS` and `LOG_KEY_FP_SECRET` when `ENV` is staging, stage, prod or production, SHALL fill `localhost:9092` and a documented local secret otherwise, SHALL read shutdown timing from `SHUTDOWN_GRACE` and `SHUTDOWN_DRAIN_DELAY` as Go durations, SHALL also require `GA4_API_SECRET`, `GA4_MEASUREMENT_ID` and `GA4_EXPORT_DATASET` when `ENV` is strict, SHALL accept `OTEL_SERVICE_NAME`, and SHALL keep every existing cross-field validation of ports, batch size, tracking thresholds, warehouse driver fields and GA4 settings.

#### Scenario: ENV is required
- **WHEN** `Load` runs with `ENV` unset
- **THEN** it returns an error naming `ENV`
- **VERIFIED BY**: team-analytics `internal/config` `TestLoadRequiresENV`

#### Scenario: A strict ENV requires the broker address
- **WHEN** `Load` runs with `ENV=prod` and `KAFKA_BROKERS` unset
- **THEN** it returns an error naming `KAFKA_BROKERS`
- **VERIFIED BY**: team-analytics `internal/config` `TestStrictEnvRequiresKafkaBrokers`

#### Scenario: A strict ENV requires the key fingerprint secret
- **WHEN** `Load` runs with `ENV=staging` and `LOG_KEY_FP_SECRET` unset
- **THEN** it returns an error naming `LOG_KEY_FP_SECRET`
- **VERIFIED BY**: team-analytics `internal/config` `TestStrictEnvRequiresLogKeyFPSecret`

#### Scenario: A local ENV fills the defaults
- **WHEN** `Load` runs with `ENV=local` and neither `KAFKA_BROKERS` nor `LOG_KEY_FP_SECRET` set
- **THEN** `Kafka.Brokers` is `localhost:9092` and the secret is non-empty
- **VERIFIED BY**: team-analytics `internal/config` `TestLocalEnvFillsBrokerAndSecretDefaults`

#### Scenario: Shutdown timing is a Go duration
- **WHEN** `Load` runs with `SHUTDOWN_GRACE=15s`, `SHUTDOWN_DRAIN_DELAY=2s` and a stale `SHUTDOWN_GRACE_SECONDS=99`
- **THEN** `ShutdownGrace` is 15 s, `DrainDelay` is 2 s and the stale key has no effect
- **VERIFIED BY**: team-analytics `internal/config` `TestShutdownTimingIsGoDuration`

#### Scenario: A non-positive batch size is rejected
- **WHEN** `Load` runs with `BATCH_MAX_SIZE=0`
- **THEN** it returns an error naming `BATCH_MAX_SIZE`
- **VERIFIED BY**: team-analytics `internal/config` `TestValidateRejectsNonPositiveBatchSize`

#### Scenario: A missing DuckDB path is rejected
- **WHEN** `Load` runs with `WAREHOUSE_DRIVER=duckdb` and `DUCKDB_PATH` set to an empty string
- **THEN** it returns an error naming `DUCKDB_PATH`
- **VERIFIED BY**: team-analytics `internal/config` `TestValidateRejectsEmptyDuckDBPath`

#### Scenario: A strict ENV requires the GA4 relay credentials
- **WHEN** `Load` runs with `ENV=prod` and `GA4_API_SECRET` or `GA4_MEASUREMENT_ID` unset
- **THEN** it returns an error naming the unset key
- **VERIFIED BY**: team-analytics `internal/config` `TestStrictEnvRequiresGA4Credentials`

#### Scenario: A strict ENV requires the GA4 export dataset
- **WHEN** `Load` runs with `ENV=staging` and `GA4_EXPORT_DATASET` unset
- **THEN** it returns an error naming `GA4_EXPORT_DATASET`, while the same settings under `ENV=local` load without error
- **VERIFIED BY**: team-analytics `internal/config` `TestStrictEnvRequiresGA4ExportDataset`

#### Scenario: OTEL_SERVICE_NAME is an accepted key
- **WHEN** `Load` runs with `OTEL_SERVICE_NAME=analytics-eu`
- **THEN** `OTel.ServiceName` is `analytics-eu` and `.env.example` declares `OTEL_SERVICE_NAME`
- **VERIFIED BY**: team-analytics `internal/config` `TestOTelServiceNameIsAcceptedKey`

#### Scenario: .env.example documents exactly the declared keys
- **WHEN** `TestEnvExampleInSync` compares `.env.example` with the keys declared by `Settings`
- **THEN** it reports no missing and no stale key
- **VERIFIED BY**: team-analytics `internal/config` `TestEnvExampleInSync`

### Requirement: No dependency enable flag, no runtime fallback, reflection is opt-in

team-analytics SHALL NOT declare any `*_ENABLED` setting except `OTEL_ENABLED` and `GRPC_REFLECTION_ENABLED`, SHALL ignore a removed `KAFKA_ENABLED` value, SHALL NOT link any non-test `NewMemory*`, `NewInMemory*` or `Noop*` implementation into a binary, SHALL fail boot in every `ENV` when the warehouse cannot be opened, and SHALL serve gRPC reflection only when `GRPC_REFLECTION_ENABLED=true` and `ENV` is local or test.

#### Scenario: Only the allowed enable flags are declared
- **WHEN** the env keys declared by `Settings` are listed
- **THEN** the keys ending in `_ENABLED` are exactly `OTEL_ENABLED` and `GRPC_REFLECTION_ENABLED`
- **VERIFIED BY**: team-analytics `internal/config` `TestNoDependencyEnableFlags`

#### Scenario: A removed KAFKA_ENABLED value is ignored
- **WHEN** `Load` runs with `KAFKA_ENABLED=false`
- **THEN** it succeeds and the settings expose no field that reads that value
- **VERIFIED BY**: team-analytics `internal/config` `TestRemovedKafkaEnabledIsIgnored`

#### Scenario: Binaries link no in-memory implementation
- **WHEN** the non-test files in the dependency set of the three binaries are scanned for declarations named `NewMemory*`, `NewInMemory*` or `Noop*`
- **THEN** the scan finds none
- **VERIFIED BY**: team-analytics `internal/app` `TestBinariesLinkNoInMemoryImplementation`

#### Scenario: An unopenable warehouse fails boot in a local ENV
- **WHEN** `RunConsumer` runs with `ENV=local` and `DUCKDB_PATH` inside a directory that does not exist
- **THEN** it returns an error that wraps `open warehouse` before any listener or consumer starts
- **VERIFIED BY**: team-analytics `internal/app` `TestBootFailsWhenWarehouseCannotOpen`

#### Scenario: Reflection setting defaults to off
- **WHEN** `Load` runs with `ENV=local` and `GRPC_REFLECTION_ENABLED` unset
- **THEN** `ReflectionEnabled` is false
- **VERIFIED BY**: team-analytics `internal/config` `TestReflectionDefaultsToOff`

#### Scenario: A server built from default settings does not serve reflection
- **WHEN** the query server is started from default settings and a client calls `grpc.reflection.v1.ServerReflection/ServerReflectionInfo`
- **THEN** the call returns `Unimplemented`
- **VERIFIED BY**: team-analytics `internal/rpc` `TestReflectionOffByDefault`

#### Scenario: A strict ENV refuses reflection
- **WHEN** `Load` runs with `ENV=prod` and `GRPC_REFLECTION_ENABLED=true`
- **THEN** it returns an error naming `GRPC_REFLECTION_ENABLED`
- **VERIFIED BY**: team-analytics `internal/config` `TestStrictEnvRefusesReflection`

### Requirement: Mock data replaces only the outer GA4 export source

WHEN `MOCK_DATA` names `ga4export`, THE SYSTEM SHALL read the daily GA4 export from Parquet files in `GA4_EXPORT_FIXTURE_DIR` through the same importer, atomic `ga4_sessions` replace, import-run record and reconcile gauge as the BigQuery path, and SHALL log the active adapter once. `MOCK_DATA` SHALL be refused before any other validation when `ENV` is strict, an unknown adapter name SHALL be refused, and `GA4_EXPORT_SOURCE` SHALL have no effect.

#### Scenario: Mock data runs the real importer on fixtures
- **WHEN** `MOCK_DATA=ga4export` and `GA4_EXPORT_FIXTURE_DIR` holds `events_20260101.parquet` with 3 sessions and the trigger imports `2026-01-01`
- **THEN** `ga4_sessions` holds 3 rows for that date and one import run is recorded through the real importer
- **VERIFIED BY**: team-analytics `internal/ga4import` `TestMockDataGA4ExportRunsRealImporterOnFixtures`

#### Scenario: A strict ENV refuses mock data before other validation
- **WHEN** `Load` runs with `ENV=prod`, `MOCK_DATA=ga4export` and an invalid `GA4_RECONCILE_MAX_DIFF_RATIO=-1`
- **THEN** the returned error names `MOCK_DATA` and ends before the ratio error is reported
- **VERIFIED BY**: team-analytics `internal/config` `TestStrictEnvRefusesMockDataBeforeValidation`

#### Scenario: An unknown mock adapter is refused
- **WHEN** the mock set is built from `MOCK_DATA=bigquery`
- **THEN** it returns an error naming `bigquery` and listing `ga4export`
- **VERIFIED BY**: team-analytics `internal/app` `TestUnknownMockAdapterRefused`

#### Scenario: Mock data without a fixture directory fails the import
- **WHEN** `MOCK_DATA=ga4export` and `GA4_EXPORT_FIXTURE_DIR` is empty and the trigger runs
- **THEN** the response is a 5xx whose body names `GA4_EXPORT_FIXTURE_DIR`
- **VERIFIED BY**: team-analytics `internal/ga4import` `TestMockDataWithoutFixtureDirFailsNamingTheSetting`

#### Scenario: A fixture directory is ignored without mock data
- **WHEN** `MOCK_DATA` is unset, `GA4_EXPORT_FIXTURE_DIR` holds fixtures and `GA4_EXPORT_DATASET` is empty
- **THEN** the import returns 200 with 0 rows and `ga4_sessions` is unchanged
- **VERIFIED BY**: team-analytics `internal/ga4import` `TestFixtureDirIgnoredWithoutMockData`

#### Scenario: The removed GA4_EXPORT_SOURCE has no effect
- **WHEN** `Load` runs with `GA4_EXPORT_SOURCE=fixture` and `MOCK_DATA` unset
- **THEN** the import source is BigQuery and the settings expose no field that reads that value
- **VERIFIED BY**: team-analytics `internal/config` `TestRemovedGA4ExportSourceIsIgnored`

### Requirement: The tracking and order consumer commits only after a durable write and dead-letters a poison batch

The tracking and order consumer SHALL be a `consume.BatchConsumer` on `KAFKA_ANALYTICS_TOPIC` and `KAFKA_ORDER_TOPIC` in group `KAFKA_CONSUMER_GROUP`, SHALL fetch at most `BATCH_MAX_SIZE` records and wait at most `BATCH_FLUSH_INTERVAL_SECONDS` per fetch, SHALL write the tracking rows and order-fact rows of a fetch and return success only when both writes succeed so the SDK commits, SHALL return a write error unchanged so the SDK retries the same batch up to 5 attempts and then dead-letters it to `KAFKA_DLQ_TOPIC` and commits, SHALL skip an envelope that is neither a tracking event nor an order event, and SHALL count and skip an undecodable record without dead-lettering the other records of its batch.

#### Scenario: A poison batch is dead-lettered after 5 attempts and the group moves on
- **WHEN** a batch of 2 tracking records is produced to `analytics.events` on a Redpanda broker and the warehouse writer fails every write
- **THEN** both records appear on `analytics.events.dlq` within 30 s with the same keys, the writer was called 5 times and the group offset is past them
- **VERIFIED BY**: team-analytics `internal/app` `TestPoisonBatchIsDeadLetteredAfterFiveAttempts`

#### Scenario: A decoded batch is written and acknowledged
- **WHEN** the batch handler receives 3 tracking records and 2 order-fact records and the writer succeeds
- **THEN** the writer holds 3 tracking rows and the order-fact rows of the 2 records and the handler returns nil
- **VERIFIED BY**: team-analytics `internal/events` `TestBatchHandlerWritesTrackingAndOrderFactsThenAcknowledges`

#### Scenario: A write failure is returned for retry, not skipped
- **WHEN** the batch handler receives 2 tracking records and the writer returns an error
- **THEN** the handler returns that error and it does not wrap `events.ErrDeadLetter`
- **VERIFIED BY**: team-analytics `internal/events` `TestWriteFailureIsReturnedForRetry`

#### Scenario: A non-tracking envelope is skipped without a row
- **WHEN** the batch handler receives an envelope whose type is neither `platform.analytics.v1.TrackingEvent` nor an order event type
- **THEN** the writer holds 0 rows and the handler returns nil
- **VERIFIED BY**: team-analytics `internal/events` `TestNonTrackingEnvelopeIsSkipped`

#### Scenario: An undecodable tracking payload is counted and the rest of the batch is written
- **WHEN** a batch holds one record of type `platform.analytics.v1.TrackingEvent` with a corrupt payload and 2 valid tracking records
- **THEN** the writer holds 2 rows, the decode-failure counter increases by 1 and the handler returns nil
- **VERIFIED BY**: team-analytics `internal/events` `TestUndecodableRecordIsCountedAndTheRestIsWritten`

#### Scenario: A failing decode counter does not block the batch
- **WHEN** a batch holds one corrupt tracking record and 2 valid tracking records and `RecordDecodeFailures` returns an error
- **THEN** the error is logged with `key_fp`, the writer holds 2 rows and the handler returns nil
- **VERIFIED BY**: team-analytics `internal/events` `TestDecodeCounterFailureDoesNotFailTheBatch`

#### Scenario: A record with an empty key does not fail the batch
- **WHEN** the batch handler receives a corrupt record whose key is empty
- **THEN** the handler returns nil and the log line carries the `key_fp` of the empty key
- **VERIFIED BY**: team-analytics `internal/events` `TestEmptyKeyRecordIsHandled`

#### Scenario: A maximum-size batch is handled well inside the rebalance timeout
- **WHEN** a batch of `BATCH_MAX_SIZE` (500) tracking records is written to a DuckDB file in a temporary directory
- **THEN** one write completes in under 5 s, so 5 attempts plus the 4 backoff waits (about 1.5 s) stay under 27 s, half of the 60 s group rebalance timeout
- **VERIFIED BY**: team-analytics `internal/events` `TestMaxSizeBatchWriteFitsRebalanceBudget`

#### Scenario: The batch bounds come from the settings
- **WHEN** the consumer options are built from `BATCH_MAX_SIZE=500` and `BATCH_FLUSH_INTERVAL_SECONDS=2`
- **THEN** `MaxPollRecords` is 500, the fetch max wait is 2 s and the dead-letter topic is `analytics.events.dlq`
- **VERIFIED BY**: team-analytics `internal/events` `TestMainConsumerOptionsFromSettings`

### Requirement: The listing and engagement consumers use the SDK and keep their groups and start offsets

The listing consumer SHALL be a `consume.BatchConsumer` on `KAFKA_LISTING_TOPIC` in group `KAFKA_LISTING_CONSUMER_GROUP` starting at the earliest offset with dead-letter topic `KAFKA_LISTING_DLQ_TOPIC`, and SHALL skip and log an undecodable record without dead-lettering its batch. The engagement consumer SHALL be a `consume.Consumer` on `ENGAGEMENT_EVENTS_TOPIC` in group `ENGAGEMENT_CONSUMER_GROUP` whose handler SHALL return `consume.Permanent` for an undecodable envelope or an unknown engagement type, so the SDK writes it to `ENGAGEMENT_DLQ_TOPIC` and commits, and SHALL write every decodable record to `engagement_facts`, with `MaxPollRecords` set to 10 so that `MaxPollRecords x (4 backoff waits of about 1.5 s + 5 x handler time)` stays under 30 s, half of the 60 s group rebalance timeout.

#### Scenario: The listing consumer starts at the earliest offset in its own group
- **WHEN** the listing consumer spec is built from the default settings
- **THEN** it has group `team-analytics-listing-attrs`, topic `listing.events`, start-at-earliest true and dead-letter topic `listing.events.analytics.dlq`
- **VERIFIED BY**: team-analytics `internal/events` `TestListingConsumerSpecFromSettings`

#### Scenario: A poison listing batch is dead-lettered and the group moves on
- **WHEN** a batch of 2 `ListingChanged` records is produced to `listing.events` on a Redpanda broker and the listing writer fails every upsert
- **THEN** both records appear on `listing.events.analytics.dlq` within 30 s with the same keys, the writer was called 5 times and the group offset is past them
- **VERIFIED BY**: team-analytics `internal/app` `TestPoisonListingBatchIsDeadLettered`

#### Scenario: An undecodable listing record is skipped and the batch is upserted
- **WHEN** a listing batch holds 1 corrupt record and 2 valid `ListingChanged` records
- **THEN** the writer upserts 2 listing rows and the handler returns nil
- **VERIFIED BY**: team-analytics `internal/events` `TestListingBatchSkipsUndecodableRecord`

#### Scenario: Engagement polls fit the rebalance timeout
- **WHEN** the engagement consumer options are built from the default settings
- **THEN** `MaxPollRecords` is 10 and `10 x (1.5 s + 5 x 50 ms)` is 17.5 s, under 30 s
- **VERIFIED BY**: team-analytics `internal/events` `TestEngagementOptionsFitRebalanceBudget`

#### Scenario: A malformed engagement record is permanent
- **WHEN** the engagement handler receives a record whose value is not an `EventEnvelope`
- **THEN** it returns an error that wraps `events.ErrDeadLetter` (returned through `consume.Permanent`) and writes no row
- **VERIFIED BY**: team-analytics `internal/events` `TestMalformedEngagementRecordIsPermanent`

#### Scenario: An unknown engagement type is permanent
- **WHEN** the engagement handler receives a well-formed envelope of an unknown type
- **THEN** it returns an error that wraps both `ErrUnknownEngagementType` and `events.ErrDeadLetter`
- **VERIFIED BY**: team-analytics `internal/events` `TestUnknownEngagementTypeIsPermanent`

#### Scenario: A valid engagement record reaches engagement_facts
- **WHEN** the engagement handler receives a `FavoriteAdded` envelope
- **THEN** the writer holds one `favorite_added` fact for that user and listing
- **VERIFIED BY**: team-analytics `internal/events` `TestValidEngagementRecordIsWritten`

#### Scenario: A failing engagement write is returned for retry
- **WHEN** the engagement handler receives a valid record and the writer returns an error
- **THEN** the handler returns that error and it does not wrap `events.ErrDeadLetter`
- **VERIFIED BY**: team-analytics `internal/events` `TestEngagementWriteFailureIsTransient`

#### Scenario: A malformed engagement record lands on the dead-letter topic and the group moves on
- **WHEN** a record that is not an `EventEnvelope` and then a valid `FavoriteAdded` record are produced to `engagement.events` on a Redpanda broker and the engagement consumer runs
- **THEN** a record with the same key and the headers `dlq-source-topic` and `dlq-error` appears on `engagement.events.analytics.dlq` within 30 s and the favourite reaches `engagement_facts`
- **VERIFIED BY**: team-analytics `internal/app` `TestMalformedEngagementRecordIsDeadLetteredAndGroupMovesOn`

### Requirement: Dead-letter copies of tracking records carry no ga_client_id without analytics consent

The `consume.DLQProducer` of the tracking and order consumer (`KAFKA_DLQ_TOPIC`) and of the GA4 relay consumer (`GA4_RELAY_DLQ_TOPIC`) SHALL be `events.NewRedactingDLQ`. WHEN a record whose envelope type is `platform.analytics.v1.TrackingEvent` is published to a dead-letter topic and its `consent.analytics` is false, `consent` is absent or `consent.version` is 0, negative or unknown, THE SYSTEM SHALL publish it with `ga_client_id` empty and every other field, the key, the headers and the topic unchanged; WHEN `consent.analytics` is true under a known version, THE SYSTEM SHALL publish the record unchanged; a record that is not a tracking envelope or whose value cannot be decoded SHALL be published unchanged.

#### Scenario: A dead-letter copy blanks the client id without consent
- **WHEN** the redacting producer receives a tracking record with `ga_client_id=123.456`, `consent.analytics=false`, a user id and a listing id
- **THEN** the published record decodes to a `TrackingEvent` with `ga_client_id` empty and the same user id, listing id, key and headers
- **VERIFIED BY**: team-analytics `internal/events` `TestRedactingDLQBlanksGAClientIDWithoutConsent`

#### Scenario: A dead-letter copy keeps the client id with consent
- **WHEN** the redacting producer receives a tracking record with `ga_client_id=123.456`, `consent.analytics=true` and `consent.version=1`
- **THEN** the published record is byte-identical to the input value
- **VERIFIED BY**: team-analytics `internal/events` `TestRedactingDLQKeepsGAClientIDWithConsent`

#### Scenario: Absent or version-0 consent is treated as denied
- **WHEN** the redacting producer receives a tracking record with `ga_client_id=123.456` and either no `consent` or `consent.analytics=true` with `consent.version=0`
- **THEN** the published record has `ga_client_id` empty in both cases
- **VERIFIED BY**: team-analytics `internal/events` `TestRedactingDLQTreatsMissingOrVersionZeroConsentAsDenied`

#### Scenario: An undecodable or non-tracking record is published unchanged
- **WHEN** the redacting producer receives a record whose value is not an `EventEnvelope`, and another record whose envelope type is an order event
- **THEN** both are published with the original value, and the publish error of the wrapped producer is returned unchanged
- **VERIFIED BY**: team-analytics `internal/events` `TestRedactingDLQPassesOtherRecordsUnchanged`

#### Scenario: A parked tracking batch on the broker carries no client id without consent
- **WHEN** a tracking record with `ga_client_id=123.456` and `consent.analytics=false` is produced to `analytics.events` on a Redpanda broker and the warehouse writer fails every write
- **THEN** the record appears on `analytics.events.dlq` within 30 s with the same key and `ga_client_id` empty
- **VERIFIED BY**: team-analytics `internal/app` `TestPoisonBatchDLQCarriesNoGAClientIDWithoutConsent`

#### Scenario: Both consumers are wired to the redacting producer
- **WHEN** the consumer and the relay composition roots are built
- **THEN** the dead-letter producer given to the main consumer and to the relay consumer is an `events.RedactingDLQ`
- **VERIFIED BY**: team-analytics `internal/app` `TestDLQProducersAreRedacting`

### Requirement: Consumer logs carry a keyed fingerprint and never the record key

Every team-analytics log line about a consumed record SHALL carry `key_fp`, computed with `kafkax.Fingerprinter` keyed by `LOG_KEY_FP_SECRET`, and SHALL NOT carry the raw key in any attribute or message, in the tracking, order, listing, engagement and GA4 relay consumers. Record keys written to a dead-letter topic SHALL be unchanged.

#### Scenario: A decode failure logs the fingerprint and not the key
- **WHEN** a corrupt record keyed `user-123` is handled by the batch handler with secret `s1`
- **THEN** the captured log output has `key_fp` equal to `Fingerprinter(s1).Of("user-123")` and contains no occurrence of `user-123`
- **VERIFIED BY**: team-analytics `internal/events` `TestNoConsumerLogContainsRawKey`

#### Scenario: The relay logs the fingerprint and not the buyer key
- **WHEN** the relay handles an invalid envelope keyed `buyer-9`
- **THEN** the captured log output has `key_fp` and contains no occurrence of `buyer-9`
- **VERIFIED BY**: team-analytics `internal/ga4relay` `TestRelayLogsNeverContainRawKey`

#### Scenario: An empty fingerprint secret is refused at boot
- **WHEN** the consumer fingerprinter is built from an empty secret
- **THEN** construction returns an error and no consumer starts
- **VERIFIED BY**: team-analytics `internal/app` `TestEmptyLogKeyFPSecretRefusesBoot`

### Requirement: Readiness reflects the warehouse only, liveness reflects the process, shutdown drains first

The consumer SHALL serve `grpc.health.v1.Health` with an overall status that is NOT_SERVING while the DuckDB ping fails, and a `liveness` service that is SERVING independently of it until shutdown. Kafka SHALL NOT be a readiness check: no Kafka check is registered, and a broker outage SHALL leave the overall status unchanged. WHEN the process receives a stop signal, THE SYSTEM SHALL set the overall and `liveness` status to NOT_SERVING before the gRPC server stops, SHALL stop the Kafka workers only after the server has stopped, and SHALL close the warehouse after the workers.

#### Scenario: A warehouse outage flips readiness and not liveness
- **WHEN** the DuckDB ping check returns an error
- **THEN** the overall health status becomes NOT_SERVING within one health interval plus 1 s (6 s with `healthx.DefaultInterval`; the test injects a 200 ms interval through the `app` test seam and asserts NOT_SERVING within 1.2 s) and `liveness` stays SERVING
- **VERIFIED BY**: team-analytics `internal/app` `TestWarehouseOutageFlipsReadinessNotLiveness`

#### Scenario: A Kafka outage does not flip readiness
- **WHEN** the consumer runs with `KAFKA_BROKERS` pointing at a closed port and the DuckDB ping succeeds
- **THEN** the overall health status stays SERVING for 10 s (two health intervals) and the registered check names are exactly `duckdb`
- **VERIFIED BY**: team-analytics `internal/app` `TestKafkaOutageDoesNotFlipReadiness`

#### Scenario: Shutdown drains health before the server stops
- **WHEN** the run context is cancelled while a client holds a health watch
- **THEN** the watch receives NOT_SERVING for the overall service before the server stops
- **VERIFIED BY**: team-analytics `internal/app` `TestShutdownDrainsHealthBeforeServerStops`

#### Scenario: Workers outlive the server and flush on the way out
- **WHEN** the run context is cancelled while the batch consumer holds a fetched batch
- **THEN** the batch is written before the warehouse closer runs
- **VERIFIED BY**: team-analytics `internal/app` `TestWorkersStopAfterServerAndBeforeWarehouseClose`

#### Scenario: A BigQuery warehouse serves health only
- **WHEN** the consumer runs with `WAREHOUSE_DRIVER=bigquery` and a stub writer that is not a DuckDB writer
- **THEN** a call to `GetSellerFunnel` returns `Unimplemented` and the health service answers SERVING
- **VERIFIED BY**: team-analytics `internal/app` `TestNonDuckDBWarehouseServesHealthOnly`

### Requirement: The query service authorizes in each handler and does not leak internal errors

Each `AnalyticsQueryService` handler SHALL authorize before any repository call: seller RPCs (`GetSellerFunnel`, `GetRevenueBreakdown`, `GetDemandForecast`) SHALL return `Unauthenticated` for no or anonymous principal, SHALL allow a user whose id equals `seller_id` or a principal holding the exact `admin` scope, and SHALL return `PermissionDenied` otherwise; admin RPCs (`GetPlatformOrderSummary`, `ListRecentOrders`, `GetTrackingQualityReport`, `GetRecommendationPerformance`, `GetChannelFunnel`) SHALL require the exact `admin` scope. An error that is not a gRPC status or a mapped sentinel SHALL reach the caller as `Internal` with the message `internal error`; a context cancellation SHALL map to `Canceled` and a deadline to `DeadlineExceeded`. Clients SHALL NOT depend on the text of `PermissionDenied` and `Internal` errors, which changed from `insufficient_scope: missing "admin"` and driver-derived text to the SDK's `missing scope` and `internal error`; only the status code is contract.

#### Scenario: A seller reads their own funnel
- **WHEN** a user principal `s1` calls `GetSellerFunnel` for `seller_id=s1`
- **THEN** the response is the funnel and the repository is called once
- **VERIFIED BY**: team-analytics `internal/rpc` `TestSellerReadsOwnFunnel`

#### Scenario: A seller cannot read another seller's revenue
- **WHEN** a user principal `s1` calls `GetRevenueBreakdown` for `seller_id=s2`
- **THEN** the call returns `PermissionDenied` and the repository is called 0 times
- **VERIFIED BY**: team-analytics `internal/rpc` `TestSellerCannotReadAnotherSellersRevenue`

#### Scenario: An anonymous caller is unauthenticated
- **WHEN** an anonymous principal calls `GetDemandForecast`
- **THEN** the call returns `Unauthenticated` and the repository is called 0 times
- **VERIFIED BY**: team-analytics `internal/rpc` `TestAnonymousCallerIsUnauthenticated`

#### Scenario: An admin reads any seller
- **WHEN** a principal with scope `admin` calls `GetSellerFunnel` for `seller_id=s9`
- **THEN** the response is the funnel for `s9`
- **VERIFIED BY**: team-analytics `internal/rpc` `TestAdminReadsAnySeller`

#### Scenario: A service principal without admin is denied
- **WHEN** a service principal with scope `analytics.read` calls `GetSellerFunnel` for `seller_id=s1`
- **THEN** the call returns `PermissionDenied`
- **VERIFIED BY**: team-analytics `internal/rpc` `TestServicePrincipalWithoutAdminIsDenied`

#### Scenario: An admin RPC requires the exact admin scope
- **WHEN** a user principal with scope `admin:read` calls `GetPlatformOrderSummary`
- **THEN** the call returns `PermissionDenied`
- **VERIFIED BY**: team-analytics `internal/rpc` `TestAdminRPCsRequireExactAdminScope`

#### Scenario: A repository error does not leak driver text
- **WHEN** the repository returns an error whose text contains `duckdb: IO Error /data/analytics.duckdb`
- **THEN** the caller gets `Internal` with the message `internal error`
- **VERIFIED BY**: team-analytics `internal/rpc` `TestRepositoryErrorDoesNotLeak`

#### Scenario: A cancelled caller gets Canceled
- **WHEN** the request context is cancelled while the repository call is running
- **THEN** the call returns `Canceled`
- **VERIFIED BY**: team-analytics `internal/rpc` `TestCancelledContextMapsToCanceled`

#### Scenario: An unsupported capability is unavailable
- **WHEN** an admin calls `GetChannelFunnel` and the repository returns `query.ErrUnsupported`
- **THEN** the call returns `Unavailable` with the message `not available on this warehouse`
- **VERIFIED BY**: team-analytics `internal/rpc` `TestUnsupportedCapabilityIsUnavailable`

#### Scenario: An invalid window is rejected before the repository
- **WHEN** an admin calls `GetTrackingQualityReport` with `window_hours=0`
- **THEN** the call returns `InvalidArgument` and the repository is called 0 times
- **VERIFIED BY**: team-analytics `internal/rpc` `TestInvalidWindowIsRejectedBeforeRepository`

### Requirement: The GA4 relay is a gRPC-health-only consumer that dead-letters through the SDK

The relay SHALL consume `analytics.events` and `order.events` through `consume.Consumer` in group `GA4_RELAY_CONSUMER_GROUP` when `GA4_API_SECRET` and `GA4_MEASUREMENT_ID` are both set. `Relay.Handle` SHALL return nil for a processed, ignored or already-sent record, SHALL return `consume.Permanent` after incrementing `ga4_relay_failed_total{reason}` for an undecodable envelope, an `OrderPaidEvent` without order id, a 4xx send error or exhausted retries, and SHALL return a non-permanent error for a ledger or mapping-store failure so the SDK retries it up to 5 attempts, after which the SDK dead-letters the record to `GA4_RELAY_DLQ_TOPIC` and commits, counting each failed attempt in `ga4_relay_failed_total{reason="store"}`, and SHALL set `MaxPollRecords` to 1 so a GA4 outage (up to 40 s on one record) stays inside the 60 s group rebalance timeout.

#### Scenario: A processed order returns nil
- **WHEN** `Relay.Handle` receives an `OrderPaidEvent` for `o1` with a mapped buyer and the GA4 stub answers 204
- **THEN** it returns nil and the ledger contains `o1`
- **VERIFIED BY**: team-analytics `internal/ga4relay` `TestHandleSentOrderReturnsNil`

#### Scenario: A permanent 4xx is dead-lettered by the SDK
- **WHEN** `Relay.Handle` receives an `OrderPaidEvent` for `o2` and the GA4 stub answers 400
- **THEN** it returns an error that wraps `ga4relay.ErrDeadLetter` (returned through `consume.Permanent`), `ga4_relay_failed_total{reason="4xx"}` increases by 1 and the ledger does not contain `o2`
- **VERIFIED BY**: team-analytics `internal/ga4relay` `TestHandle4xxIsPermanent`

#### Scenario: Exhausted retries are permanent
- **WHEN** `Relay.Handle` receives an `OrderPaidEvent` and the GA4 stub answers 503 for all 5 attempts
- **THEN** it returns an error that wraps `ga4relay.ErrDeadLetter` and `ga4_relay_failed_total{reason="exhausted"}` increases by 1
- **VERIFIED BY**: team-analytics `internal/ga4relay` `TestHandleExhaustedRetriesIsPermanent`

#### Scenario: An invalid envelope is permanent
- **WHEN** `Relay.Handle` receives a value that is not an `EventEnvelope`
- **THEN** it returns an error that wraps `ga4relay.ErrDeadLetter` and `ga4_relay_failed_total{reason="invalid"}` increases by 1
- **VERIFIED BY**: team-analytics `internal/ga4relay` `TestHandleInvalidEnvelopeIsPermanent`

#### Scenario: A ledger failure is transient
- **WHEN** `Relay.Handle` receives an `OrderPaidEvent` and the ledger lookup returns an error
- **THEN** it returns that error, it does not wrap `ga4relay.ErrDeadLetter` and the GA4 stub receives 0 requests
- **VERIFIED BY**: team-analytics `internal/ga4relay` `TestHandleLedgerFailureIsTransient`

#### Scenario: A mapping store failure is transient
- **WHEN** the analytics-events handler receives a consented tracking event and the mapping store returns an error
- **THEN** it returns that error and it does not wrap `ga4relay.ErrDeadLetter`
- **VERIFIED BY**: team-analytics `internal/ga4relay` `TestMappingStoreFailureIsTransient`

#### Scenario: A rejected paid order lands on the relay dead-letter topic
- **WHEN** an `OrderPaidEvent` is produced to `order.events` on a Redpanda broker, the relay runs against a GA4 stub answering 400 and a second valid order follows
- **THEN** the first record appears on `analytics.ga4-relay.dlq` within 30 s with header `dlq-source-topic=order.events` and the second order is sent once
- **VERIFIED BY**: team-analytics `internal/app` `TestRejectedOrderIsDeadLetteredAndNextOrderSent`

#### Scenario: A redelivered order is sent once
- **WHEN** `Relay.Handle` receives the same `OrderPaidEvent` for `o1` 3 times
- **THEN** the GA4 stub receives exactly 1 request
- **VERIFIED BY**: team-analytics `internal/ga4relay` `TestHandleRedeliveryIsIdempotent`

#### Scenario: The relay polls one record at a time
- **WHEN** the relay consumer options are built from the default settings
- **THEN** `MaxPollRecords` is 1 and one record handled at the worst case of 40 s fits the 60 s group rebalance timeout
- **VERIFIED BY**: team-analytics `internal/ga4relay` `TestRelayConsumerOptionsFitRebalanceBudget`

#### Scenario: The relay health reflects the relay DuckDB file only when enabled
- **WHEN** the relay is enabled and its DuckDB ping fails
- **THEN** the overall health status becomes NOT_SERVING within one health interval plus 1 s (the test injects a 200 ms interval and asserts within 1.2 s) and `liveness` stays SERVING
- **VERIFIED BY**: team-analytics `internal/app` `TestRelayReadinessFollowsDuckDBWhenEnabled`

#### Scenario: A Kafka outage does not flip relay readiness
- **WHEN** the relay is enabled with `KAFKA_BROKERS` pointing at a closed port and its DuckDB ping succeeds
- **THEN** the overall health status stays SERVING for 10 s and the registered check names are exactly `duckdb`
- **VERIFIED BY**: team-analytics `internal/app` `TestRelayReadinessIgnoresKafka`
