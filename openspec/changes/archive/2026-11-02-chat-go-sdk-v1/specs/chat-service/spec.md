## ADDED Requirements

### Requirement: The service binary applies migrations through a migrate up subcommand and never at boot

`team-chat/cmd/server` SHALL accept the subcommand `migrate up`, which SHALL apply the embedded migrations (`migrations.Files`) to `DATABASE_URL` with `migrate.Up`, exit 0 when applied or when nothing is pending, and exit non-zero with the cause when it fails. `app.Run` SHALL NOT call `migrate.Up` or run any DDL; a service started against an unmigrated database SHALL make `SendMessage` fail with `Internal` and SHALL NOT create any table. The `migrate up` subcommand SHALL be configured by `ENV` and `DATABASE_URL` only (a separate `config.LoadMigrate`), and SHALL NOT require Kafka, `GRPC_PORT` or any other variable.

#### Scenario: migrate up applies all migrations and is idempotent
- **WHEN** the `migrate up` subcommand runs twice against an empty database with only `ENV=local` and `DATABASE_URL` set
- **THEN** both runs exit 0, and after the first the table `chat_outbox_events` matches `outbox.SchemaSQL` and `schema_migrations` holds version 12
- **VERIFIED BY**: `team-chat/internal/app` `TestMigrateUpAppliesAndIsIdempotent` (Postgres)

#### Scenario: migrate up fails loudly on an unreachable database
- **WHEN** `migrate up` runs with a `DATABASE_URL` that refuses connections
- **THEN** it returns a non-zero exit code and an error naming the database, within 10 seconds
- **VERIFIED BY**: `team-chat/internal/app` `TestMigrateUpFailsOnUnreachableDatabase`

#### Scenario: Service boot never migrates
- **WHEN** `app.Run` starts against an empty database with no tables
- **THEN** no table is created (`schema_migrations` and `chat_outbox_events` do not exist after boot) and a `SendMessage` returns `Internal` with the text `internal error`
- **VERIFIED BY**: `team-chat/internal/app` `TestBootDoesNotMigrate` (Postgres)

### Requirement: Chat RPCs act only for an authenticated thread participant

Every `chat.v1.ChatService` RPC SHALL read the caller from the gateway principal metadata with `auth.RequirePrincipal` and SHALL use the principal id, never a request field, as the acting user. The gateway's anonymous principal (id `anonymous` or type ANONYMOUS) and a missing principal SHALL be rejected with `Unauthenticated`. `GetThreadMessages`, `SendMessage` and `MarkThreadRead` SHALL return `PermissionDenied` when the caller is neither the buyer nor the seller of the thread. `SearchMessages` SHALL return only messages of threads where the caller is a participant, and a blank query SHALL return an empty result without a database query. `GetOrCreateThread` SHALL return `InvalidArgument` for an empty seller id and for a seller id equal to the caller.

#### Scenario: A participant sends and reads a message
- **WHEN** the buyer calls `GetOrCreateThread` for a seller and `SendMessage` with content `hello`, and the seller then calls `GetThreadMessages`
- **THEN** the seller's response contains one message with content `hello` and sender id equal to the buyer id
- **VERIFIED BY**: `team-chat/internal/rpc` `TestChatLifecycleOverGRPC`

#### Scenario: An anonymous caller is rejected
- **WHEN** `ListThreads` is called with principal metadata id `anonymous` and type `anonymous`, and again with no principal metadata
- **THEN** both calls return `Unauthenticated`
- **VERIFIED BY**: `team-chat/internal/rpc` `TestAnonymousAndMissingPrincipalAreUnauthenticated`

#### Scenario: A non-participant cannot read, send or mark read
- **WHEN** a third user calls `GetThreadMessages`, `SendMessage` and `MarkThreadRead` on another pair's thread
- **THEN** each call returns `PermissionDenied` and no message or outbox row is stored
- **VERIFIED BY**: `team-chat/internal/rpc` `TestNonParticipantIsDeniedOnEveryThreadRPC`

#### Scenario: Search is limited to the caller's threads
- **WHEN** two threads of different pairs hold the text `needle` and a participant of the first searches for `needle`
- **THEN** the result contains only the message of the first thread
- **VERIFIED BY**: `team-chat/internal/store` `TestSearchMessagesScopedToParticipant` (Postgres)

#### Scenario: A blank search returns nothing
- **WHEN** `SearchMessages` is called with query `   `
- **THEN** the response has zero messages and zero total, and the repository received no search call
- **VERIFIED BY**: `team-chat/internal/chat` `TestSearchMessagesBlankQueryIsEmpty`

#### Scenario: A thread with oneself or without a seller is rejected
- **WHEN** `GetOrCreateThread` is called with an empty seller id, and again with the caller's own id as seller id
- **THEN** both calls return `InvalidArgument`
- **VERIFIED BY**: `team-chat/internal/rpc` `TestGetOrCreateThreadRejectsEmptyAndSelfSeller`

#### Scenario: An empty message is rejected
- **WHEN** `SendMessage` is called with content `` or with an empty thread id
- **THEN** the call returns `InvalidArgument` and nothing is stored
- **VERIFIED BY**: `team-chat/internal/rpc` `TestSendMessageRejectsEmptyContentAndThread`

### Requirement: Page sizes keep their defaults and maximum

The rpc layer SHALL resolve the page parameters of `ListThreads`, `GetThreadMessages` and `SearchMessages` once, before calling `chat`. A page size below 1 or above 100 SHALL be replaced by the default (20 for `ListThreads`, 50 for `GetThreadMessages` and `SearchMessages`); a page size from 1 to 100 SHALL be honoured. A cursor that is empty, non-numeric or below 1 SHALL be treated as page 1. The page cursor SHALL stay the decimal page number: `next_cursor` is `page+1` as a decimal string when `page * effective_page_size` is below the total, and empty otherwise, where the effective page size is the one after the fallback and the same one used for the query offset. This corrects today's handler, which computes `next_cursor` with the requested size (for example 101) while the query uses the default.

#### Scenario: Page size defaults apply
- **WHEN** `ListThreads`, `GetThreadMessages` and `SearchMessages` are called with page size 0 over 120 rows each
- **THEN** `ListThreads` returns 20 rows and the other two return 50 rows each, and all three report the total 120
- **VERIFIED BY**: `team-chat/internal/rpc` `TestPageSizeDefaults`

#### Scenario: A page size that exceeds the maximum is rejected in favour of the default
- **WHEN** `GetThreadMessages` is called with page size 101 and page 0 over 120 rows
- **THEN** it returns 50 rows from offset 0 and does not honour the value 101
- **VERIFIED BY**: `team-chat/internal/rpc` `TestPageSizeAboveMaximumUsesDefault`

#### Scenario: Next cursor uses the effective page size
- **WHEN** `GetThreadMessages` is called with page size 101 and cursor `2` over 120 rows
- **THEN** it returns 50 rows from offset 50 and `next_cursor` is `3`, because 2 * 50 is below 120
- **VERIFIED BY**: `team-chat/internal/rpc` `TestNextCursorUsesEffectivePageSize`

#### Scenario: The page size boundaries are 1 and 100
- **WHEN** `GetThreadMessages` is called with page size 100, then 1, then 0 over 120 rows
- **THEN** the responses hold 100 rows, 1 row and 50 rows, and the first `next_cursor` is `2`
- **VERIFIED BY**: `team-chat/internal/rpc` `TestPageSizeBoundaries`

#### Scenario: ListThreads paginates with its own default
- **WHEN** `ListThreads` is called with page size 101 and cursor `2` over 50 threads, and again with page size 0 and cursor `3`
- **THEN** the first returns 20 rows from offset 20 with `next_cursor` `3`, and the second returns 10 rows with an empty `next_cursor`
- **VERIFIED BY**: `team-chat/internal/rpc` `TestListThreadsPagination`

#### Scenario: SearchMessages paginates with its own default
- **WHEN** `SearchMessages` is called with page size 101 and cursor `2` over 120 matches, and again with cursor `abc` and page size -1
- **THEN** the first returns 50 rows from offset 50 with `next_cursor` `3`, and the second returns the first 50 rows with `next_cursor` `2`
- **VERIFIED BY**: `team-chat/internal/rpc` `TestSearchMessagesPagination`

#### Scenario: The cursor is the next page number
- **WHEN** `GetThreadMessages` is called with page size 50 and no cursor over 120 rows, then with the returned cursor
- **THEN** the first `next_cursor` is `2`, the second is `3`, and the third response has an empty `next_cursor`
- **VERIFIED BY**: `team-chat/internal/rpc` `TestNextCursorIsNextPageNumber`

#### Scenario: Bad cursor and negative page size fall back
- **WHEN** `GetThreadMessages` is called with cursor `abc`, and again with cursor `0`, each with page size -1 over 120 rows
- **THEN** both return page 1 (offset 0) with 50 rows, the default size, and `next_cursor` is `2`
- **VERIFIED BY**: `team-chat/internal/rpc` `TestBadCursorAndNegativePageSizeFallBack`

### Requirement: Thread, message and quick-reply behaviour is unchanged

The port SHALL keep today's thread, message and quick-reply behaviour. `ListQuickReplies` SHALL use the caller's id when `seller_id` is empty and SHALL return the seeded defaults when the seller has configured none. `SendMessage` SHALL accept `message_type`, `listing_id` and `payload` (LISTING_CARD and QUICK_REPLY), SHALL return them on read, and a plain TEXT message without them SHALL still work. `MarkThreadRead` SHALL clear the caller's unread count; `ListThreads` SHALL report each thread's unread count, `last_message_text` and `last_message_at`. `GetOrCreateThread` SHALL return the same thread for the same buyer, seller and listing. `SendMessage` and `MarkThreadRead` on an unknown thread SHALL return `NotFound`. `sender_name` SHALL be `"User "` plus the first 6 characters of the sender id when the id is longer than 6 characters, and `"Người dùng"` otherwise; this derivation lives in the rpc layer (today in `handler.SendMessage`) and moves with it into `rpc`, not into `chat`.

#### Scenario: Quick replies default to the caller and to seeded defaults
- **WHEN** `ListQuickReplies` is called with an empty `seller_id` by a seller who configured none, and again by a seller who configured one reply
- **THEN** the first returns the seeded default replies for the caller's own id and the second returns exactly its configured reply
- **VERIFIED BY**: `team-chat/internal/rpc` `TestChatHandler_ListQuickReplies`

#### Scenario: Listing card and quick reply messages round-trip
- **WHEN** a message is sent with message type LISTING_CARD, a listing id and a payload, then one with QUICK_REPLY, and the thread is read
- **THEN** both messages come back with the same type, listing id and payload
- **VERIFIED BY**: `team-chat/internal/rpc` `TestChatHandler_SendListingCardRoundtrip`

#### Scenario: A plain text message keeps working
- **WHEN** `SendMessage` is called with only a thread id and content
- **THEN** the stored message has type TEXT, an empty listing id and an empty payload
- **VERIFIED BY**: `team-chat/internal/rpc` `TestChatHandler_SendPlainTextBackCompat`

#### Scenario: Marking read clears the unread count
- **WHEN** the buyer sends two messages, the seller calls `ListThreads`, then `MarkThreadRead`, then `ListThreads` again
- **THEN** the seller's unread count is 2 and then 0, and the buyer's own count stays 0
- **VERIFIED BY**: `team-chat/internal/rpc` `TestChatHandler_GetThreadMessagesAndMarkRead`

#### Scenario: Thread list reports last message and unread
- **WHEN** a thread holds messages `a` then `b` and `ListThreads` is called by the seller
- **THEN** the row has `last_message_text` `b`, a non-empty `last_message_at` and unread count 2
- **VERIFIED BY**: `team-chat/internal/rpc` `TestChatHandler_ListThreads`; `team-chat/internal/store` `TestUnreadCounts`

#### Scenario: The same thread is returned for the same triple
- **WHEN** the buyer calls `GetOrCreateThread` twice with the same seller and listing, and once with another listing
- **THEN** the first two return the same thread id and the third a different one
- **VERIFIED BY**: `team-chat/internal/rpc` `TestChatHandler_GetOrCreateThread`

#### Scenario: An unknown thread is NotFound for send and mark read
- **WHEN** `SendMessage` and `MarkThreadRead` are called with a thread id that does not exist
- **THEN** both return `NotFound` and no outbox row is written
- **VERIFIED BY**: `team-chat/internal/rpc` `TestUnknownThreadIsNotFoundForSendAndMarkRead`

#### Scenario: The sender name is derived from the id
- **WHEN** the user with id `abcdef123456` sends a message
- **THEN** the stored message has `sender_name` `User abcdef`
- **VERIFIED BY**: `team-chat/internal/rpc` `TestSenderNameIsDerivedFromID`

#### Scenario: A short id gets the fallback sender name
- **WHEN** a user with id `abcdef` (6 characters) sends a message, and a user with id `abcdefg` (7 characters) sends another
- **THEN** the first stored message has `sender_name` `Người dùng` and the second `User abcdef`
- **VERIFIED BY**: `team-chat/internal/rpc` `TestSenderNameIsDerivedFromID`

#### Scenario: A non-participant cannot change another pair's unread count
- **WHEN** a third user calls `MarkThreadRead` on a thread whose buyer and seller are other users
- **THEN** the call returns `PermissionDenied` (forbidden, 403 equivalent) and the thread's unread count is unchanged
- **VERIFIED BY**: `team-chat/internal/rpc` `TestNonParticipantIsDeniedOnEveryThreadRPC`

### Requirement: Every stored message yields exactly one chat.events record, delivered in thread order

`SendMessage` SHALL write the message, update the thread and insert one outbox row in `chat_outbox_events` in one database transaction; the row SHALL carry a `platform.events.v1.EventEnvelope` with `event_id` deterministic from the message id, type `platform.chat.v1.ChatMessage`, the recipient id and seller id, and `aggregate_id` equal to the thread id. A failure to build or insert the row SHALL roll back the message. The relayer SHALL be an `outbox.Relayer` that is always running, SHALL publish to `KAFKA_CHAT_TOPIC` keyed by thread id, SHALL keep the order of one thread by `seq`, and SHALL have no attempt limit: a failed publish SHALL retry with capped backoff and a row SHALL be parked only after a permanent record error.

#### Scenario: A message and its event commit together
- **WHEN** the buyer sends a message with recipient the seller
- **THEN** one message row and one outbox row exist, and the envelope's payload decodes to a `ChatMessage` with that recipient id and the thread's seller id
- **VERIFIED BY**: `team-chat/internal/store` `TestSaveMessageIsAtomicWithOutbox` (Postgres)

#### Scenario: A failed outbox insert stores nothing
- **WHEN** the outbox row builder returns an error while a message is saved
- **THEN** the call returns an error and neither the message nor an outbox row exists
- **VERIFIED BY**: `team-chat/internal/store` `TestSaveMessageRollsBackWhenOutboxFails` (Postgres)

#### Scenario: A message sent while Kafka is down is delivered after recovery
- **WHEN** a message is stored while the publisher fails 25 consecutive attempts, and then the publisher succeeds
- **THEN** the row is published once, still pending after the 10th failure (relayer backoff set to base 10 ms and cap 50 ms in the test, so the run is bounded at 5 s; the SDK default cap is 5 minutes), never parked, and the record key equals the thread id
- **VERIFIED BY**: `team-chat/internal/app` `TestRelayerHasNoAttemptLimit` (Postgres, `testkit.Recorder`)

#### Scenario: A permanent record error parks the row only
- **WHEN** the publisher returns an `outbox.Permanent` error for one row while another thread has a pending row
- **THEN** the first row's status is `failed`, and the other thread's row is published
- **VERIFIED BY**: `team-chat/internal/app` `TestPermanentErrorParksOneRowOnly` (Postgres, `testkit.Recorder`)

#### Scenario: Messages of one thread publish in send order
- **WHEN** five messages of one thread are sent, the first publish attempt fails and later ones succeed
- **THEN** the records arrive in the order the messages were stored
- **VERIFIED BY**: `team-chat/internal/app` `TestThreadEventsPublishInSendOrder` (Postgres, `testkit.Recorder`)

#### Scenario: A denied sender writes no event
- **WHEN** a user who is not a participant calls `SendMessage` and receives `PermissionDenied` (unauthorized)
- **THEN** the outbox table holds no row for that call
- **VERIFIED BY**: `team-chat/internal/chat` `TestDeniedSenderEnqueuesNoEvent`

#### Scenario: The event id is stable per message
- **WHEN** `BuildMessageOutboxRow` runs twice for the same message id
- **THEN** both rows have the same `EventID` and the second insert is a no-op
- **VERIFIED BY**: `team-chat/internal/events` `TestEventIDIsDeterministicPerMessage`

#### Scenario: The event carries the caller and request id
- **WHEN** `BuildMessageOutboxRow` runs with a context holding principal `u1` and request id `r1`
- **THEN** the envelope has principal id `u1`, `request_id` `r1`, and the row's `RequestID` is `r1`
- **VERIFIED BY**: `team-chat/internal/events` `TestEnvelopeCarriesPrincipalAndRequestID`

### Requirement: The outbox table is upgraded to the SDK v1 schema without losing rows or order

Migrations `0004` to `0012` SHALL move `chat_outbox_events` from the v0 shape to the shape of `outbox.SchemaSQL` (columns `topic`, `traceparent`, `seq`; `seq` NOT NULL with a sequence default; indexes `chat_outbox_events_status_idx`, `chat_outbox_events_agg_idx`, `chat_outbox_events_pending_idx`; the v0 claim index dropped), and migration `0012` SHALL requeue every row parked as `failed` (status `pending`, attempts and last error cleared). Rows that exist before the migration SHALL receive `seq` in `(created_at, event_id)` order and SHALL still be relayed. Each statement that cannot run in a transaction (`CREATE INDEX CONCURRENTLY`) SHALL be in its own migration file. Every migration SHALL have a down file (the `0012` down file is a no-op, a requeue is not reversible). The backfill (`0005`) numbers all remaining rows in one transaction and SHALL finish in under 5 seconds on 10,000 rows; the only locks are on `chat_outbox_events`, which no v1 writer uses because the first rollout scales the old pods to 0 before any new pod starts (a one-time Recreate; two relayers never run together).

#### Scenario: Old rows keep their order after the upgrade
- **WHEN** migrations `0001`..`0003` are applied, three pending v0 rows are inserted with increasing `created_at`, and `0004`..`0012` are applied
- **THEN** the three rows have `seq` ascending in `created_at` order and the relayer publishes them in that order
- **VERIFIED BY**: `team-chat/internal/store` `TestOutboxUpgradeKeepsRowOrder` (Postgres)

#### Scenario: The backfill of 10,000 rows is bounded
- **WHEN** migrations `0001`..`0003` are applied, 10,000 pending v0 rows are inserted, and `0004`..`0012` are applied
- **THEN** every row has a distinct non-null `seq` and the run of `0005` takes under 5 seconds
- **VERIFIED BY**: `team-chat/internal/store` `TestOutboxBackfillIsBounded` (Postgres)

#### Scenario: The upgraded table matches the SDK schema
- **WHEN** all migrations are applied to an empty database
- **THEN** every column and index named in `outbox.SchemaSQL("chat_outbox_events")` exists, `seq` is NOT NULL and the v0 claim index does not exist
- **VERIFIED BY**: `team-chat/internal/store` `TestOutboxSchemaMatchesSDK` (Postgres)

#### Scenario: Rows parked as failed are requeued by the upgrade
- **WHEN** migrations `0001`..`0003` are applied, two v0 rows with status `failed` and one `pending` row are inserted, and `0004`..`0012` are applied
- **THEN** all three rows have status `pending` and the relayer publishes all three, in `seq` order
- **VERIFIED BY**: `team-chat/internal/store` `TestOutboxUpgradeRequeuesFailedRows` (Postgres)

#### Scenario: A concurrent index migration is not wrapped in a transaction
- **WHEN** the migration files `0008`, `0009` and `0010` are read
- **THEN** each contains exactly one statement, a `CREATE INDEX CONCURRENTLY IF NOT EXISTS`, and no `BEGIN`
- **VERIFIED BY**: `team-chat/migrations` `TestConcurrentIndexMigrationsAreSingleStatement`

#### Scenario: Every up migration has a down migration
- **WHEN** the embedded migration files are listed
- **THEN** each `NNNN_name.up.sql` has a matching `NNNN_name.down.sql`, and `migrate.Up` followed by the down files leaves no `chat_outbox_events` v1 column
- **VERIFIED BY**: `team-chat/migrations` `TestEveryUpHasDown`

#### Scenario: Published rows older than 7 days are purged on a schedule
- **WHEN** the service runs with the purge interval set to 100 ms in the test, and the table holds a published row aged 8 days, a published row aged 6 days and a pending row aged 8 days
- **THEN** within 2 s the 8-day published row is deleted by `outbox.Store.Purge(ctx, 7*24h)`, and the 6-day published row and the pending row remain
- **VERIFIED BY**: `team-chat/internal/app` `TestPublishedRowsOlderThanSevenDaysArePurged` (Postgres)

#### Scenario: A purge failure does not stop the service
- **WHEN** the purge call returns an error while a message is sent
- **THEN** the error is logged without message content, the send succeeds and the next scheduled purge runs
- **VERIFIED BY**: `team-chat/internal/app` `TestPurgeFailureDoesNotStopService`

This requirement refines `buyer-notifications` "Chat message events are written through an outbox": creation order within a claimed batch stays true and is strengthened to per-thread `seq` order across batches and retries; the other clauses are unchanged, so no MODIFIED delta is needed.

### Requirement: Postgres, Kafka and the relayer are always on and there is no runtime fallback

team-chat SHALL NOT read `DATABASE_ENABLED`, `KAFKA_ENABLED`, `OUTBOX_ENABLED` or any other variable that switches a dependency off. `app.Run` SHALL fail with an error naming the dependency when the database is unreachable or the Kafka client cannot be built, in every `ENV`, and SHALL NOT start the gRPC server in that case. No non-test source file SHALL define or construct an in-memory repository, an in-memory outbox or a no-op publisher.

#### Scenario: An unreachable database fails boot in a local ENV
- **WHEN** `app.Run` starts with `ENV=local` and a `DATABASE_URL` that refuses connections
- **THEN** it returns an error containing `open database` within 10 seconds and nothing listens on the gRPC port
- **VERIFIED BY**: `team-chat/internal/app` `TestRunFailsWhenDatabaseUnreachable`

#### Scenario: A Kafka client that cannot be built fails boot
- **WHEN** `app.Run` starts with a publisher factory that returns an error
- **THEN** it returns an error containing `kafka publisher`, the pool is closed and nothing listens on the gRPC port
- **VERIFIED BY**: `team-chat/internal/app` `TestRunFailsWhenPublisherCannotBeBuilt`

#### Scenario: Removed enable flags are ignored
- **WHEN** settings load with `DATABASE_ENABLED=false`, `KAFKA_ENABLED=false`, `OUTBOX_ENABLED=false` and no `DATABASE_URL`
- **THEN** `config.Load` returns an error naming `DATABASE_URL`
- **VERIFIED BY**: `team-chat/internal/config` `TestRemovedEnableFlagsAreIgnored`

#### Scenario: No source reads an enable flag or builds a fallback
- **WHEN** the string literals and identifiers of every non-test `.go` file under `team-chat` are scanned
- **THEN** the only name ending in `_ENABLED` is `OTEL_ENABLED` and `GRPC_REFLECTION_ENABLED`, and no `NewInMemory`, `Noop` or `RequireDurableStorage` identifier exists
- **VERIFIED BY**: `team-chat/internal/config` `TestNoEnableFlagOrFallbackInSource`

### Requirement: Configuration uses the SDK variables and refuses unsafe values

team-chat settings SHALL load with `sdkconfig.Load` into the groups `Runtime`, `GRPC`, `Postgres`, `OTel`, `Kafka` and `Outbox` plus `KAFKA_CHAT_TOPIC` (default `chat.events`). `OTEL_SERVICE_NAME` and `OTEL_EXPORTER_OTLP_ENDPOINT` SHALL stay the OTel group variables, and `Load` SHALL set the service name to `team-chat` when `OTEL_SERVICE_NAME` is empty (the SDK group has no default). In a local ENV with `KAFKA_BROKERS` unset, `Load` SHALL use the SDK default `localhost:9092`; no setting boots the service without a Kafka client. `ENV`, `GRPC_PORT` and `DATABASE_URL` SHALL be required. In a strict `ENV` (staging, prod and their spellings) `Load` SHALL refuse `GRPC_REFLECTION_ENABLED=true` before any group validation and SHALL require `KAFKA_BROKERS`. gRPC reflection SHALL be off unless `GRPC_REFLECTION_ENABLED=true`. `.env.example` SHALL list exactly the `env` tags of `Settings`.

#### Scenario: Required variables are enforced
- **WHEN** settings load with `ENV=local` and no `GRPC_PORT`, and again with no `ENV`
- **THEN** each `Load` returns an error naming the missing variable, and `GRPC_PORT` has no default
- **VERIFIED BY**: `team-chat/internal/config` `TestRequiredVariables`

#### Scenario: Defaults match the documented values
- **WHEN** settings load with only `ENV=local`, `GRPC_PORT=50057` and `DATABASE_URL` set
- **THEN** the chat topic is `chat.events`, outbox poll interval 1s, batch size 100, claim lock 60s, reflection is false, `DB_MAX_CONNS` is 10, shutdown grace is 10s, `LOG_JSON` is true and OTel is disabled even when `OTEL_EXPORTER_OTLP_ENDPOINT` is set
- **VERIFIED BY**: `team-chat/internal/config` `TestDefaults`

#### Scenario: Strict ENV refuses reflection before other validation
- **WHEN** settings load with `ENV=prod`, `GRPC_REFLECTION_ENABLED=true` and no `DATABASE_URL`
- **THEN** `Load` returns an error naming `GRPC_REFLECTION_ENABLED` that does not name `DATABASE_URL`
- **VERIFIED BY**: `team-chat/internal/config` `TestStrictEnvRefusesReflectionFirst`

#### Scenario: Strict ENV requires Kafka brokers
- **WHEN** settings load with `ENV=staging` and `KAFKA_BROKERS` unset
- **THEN** `Load` returns an error naming `KAFKA_BROKERS`
- **VERIFIED BY**: `team-chat/internal/config` `TestStrictEnvRequiresKafkaBrokers`

#### Scenario: A local ENV without brokers uses the SDK default broker
- **WHEN** settings load with `ENV=local`, `GRPC_PORT`, `DATABASE_URL` and `KAFKA_BROKERS` unset
- **THEN** `Load` succeeds with `KAFKA_BROKERS` resolved to `localhost:9092` by the SDK, and there is no switch that boots without a Kafka client
- **VERIFIED BY**: `team-chat/internal/config` `TestLocalEnvDefaultsKafkaBrokers`

#### Scenario: OTel names keep their variables and the service name default
- **WHEN** settings load with `OTEL_SERVICE_NAME` unset, and again with it set to `chat-x`, and `OTEL_EXPORTER_OTLP_ENDPOINT` set to `http://otel:4317` in both
- **THEN** the OTel service name is `team-chat` and then `chat-x`, the endpoint is `http://otel:4317`, and OTel stays disabled because `OTEL_ENABLED` is unset
- **VERIFIED BY**: `team-chat/internal/config` `TestOTelNamesAndServiceNameDefault`

#### Scenario: Reflection is not served by default
- **WHEN** the gRPC server is built with default settings and a reflection client lists services
- **THEN** the call returns `Unimplemented`
- **VERIFIED BY**: `team-chat/internal/app` `TestReflectionOffByDefault`

#### Scenario: .env.example is in sync
- **WHEN** the `env` tags of `Settings` and the keys of `.env.example` are compared
- **THEN** both sets are equal and none of the removed names (`SERVER_PORT`, `RUNTIME_LOG_LEVEL`, `POSTGRES_HOST`, `KAFKA_ENABLED`, `OUTBOX_ENABLED`, `OUTBOX_MAX_ATTEMPTS`, `UPSTREAM_LISTING_ADDR`) appears
- **VERIFIED BY**: `team-chat/internal/config` `TestEnvExampleInSync`

### Requirement: Readiness depends on Postgres only and liveness on nothing

The service SHALL register one readiness check, `postgres.Checker(pool)`, and no Kafka check. The overall health status SHALL be SERVING while Postgres answers and NOT_SERVING within two health intervals after it stops answering (2 s with a 1 s health interval in tests; `healthx.DefaultInterval` is 5 s in production). The `liveness` health service SHALL be SERVING from construction regardless of any check and NOT_SERVING only after drain at shutdown.

#### Scenario: A Kafka outage keeps the service ready
- **WHEN** the publisher fails every attempt and `SendMessage` is called
- **THEN** the call succeeds and the overall health status stays SERVING
- **VERIFIED BY**: `team-chat/internal/app` `TestKafkaOutageKeepsReadiness` (Postgres)

#### Scenario: A Postgres outage flips readiness but not liveness
- **WHEN** the pool is closed while the service runs
- **THEN** within 2 s (health interval 1 s) the overall status is NOT_SERVING and the `liveness` status is SERVING
- **VERIFIED BY**: `team-chat/internal/app` `TestPostgresOutageFlipsReadinessNotLiveness` (Postgres)

#### Scenario: Shutdown drains health before stopping gRPC
- **WHEN** the context ends while a `SendMessage` is in flight and `SHUTDOWN_DRAIN_DELAY` is 200ms
- **THEN** the overall and `liveness` status become NOT_SERVING before the gRPC server stops, the in-flight call completes, and the relayer stops after the server
- **VERIFIED BY**: `team-chat/internal/app` `TestShutdownOrder` (Postgres)

### Requirement: Errors reach callers without internal detail

An error that is not a gRPC status or a known domain error SHALL be logged server-side and returned as `Internal` with the message `internal error`; driver, SQL and user-id text SHALL NOT appear in the message. A cancelled request SHALL return `Canceled` and an expired deadline `DeadlineExceeded`. A unique violation (SQLSTATE 23505) SHALL return `AlreadyExists`. `chat.ErrUnauthorizedChat` SHALL map to `PermissionDenied` and `store.ErrThreadNotFound` to `NotFound`.

#### Scenario: A database error is masked
- **WHEN** the repository returns an error containing `pq: relation "chat_messages" does not exist`
- **THEN** the caller receives `Internal` with message `internal error` and the text `chat_messages` appears only in the server log
- **VERIFIED BY**: `team-chat/internal/rpc` `TestInternalErrorIsMasked`

#### Scenario: Cancellation and deadline are not Internal
- **WHEN** the repository returns `context.Canceled`, and again `context.DeadlineExceeded`
- **THEN** the codes are `Canceled` and `DeadlineExceeded`
- **VERIFIED BY**: `team-chat/internal/rpc` `TestContextErrorsKeepTheirCodes`

#### Scenario: A unique violation is AlreadyExists
- **WHEN** the repository returns a `pgconn.PgError` with code `23505`
- **THEN** the code is `AlreadyExists`
- **VERIFIED BY**: `team-chat/internal/rpc` `TestUniqueViolationIsAlreadyExists`

#### Scenario: A non-participant gets PermissionDenied without thread detail
- **WHEN** a non-participant calls `GetThreadMessages` on an existing thread
- **THEN** the code is `PermissionDenied` (forbidden) and the message does not contain the buyer id or the seller id
- **VERIFIED BY**: `team-chat/internal/rpc` `TestPermissionDeniedLeaksNoParticipantIDs`

#### Scenario: An unknown thread is NotFound
- **WHEN** `GetThreadMessages` is called with a thread id that does not exist
- **THEN** the code is `NotFound`
- **VERIFIED BY**: `team-chat/internal/rpc` `TestUnknownThreadIsNotFound`

### Requirement: Message content never reaches logs

team-chat SHALL NOT write message `content`, `payload` or `sender_name`, or the Kafka record key, to any log record, including error logs.

#### Scenario: A sent message leaves no content in the logs
- **WHEN** a message with content `s3cret-needle` and sender name `Ana Nguyen` is sent, relayed, and one repository call fails
- **THEN** the captured log output contains neither `s3cret-needle` nor `Ana Nguyen`
- **VERIFIED BY**: `team-chat/internal/app` `TestLogsNeverContainMessageContent` (Postgres)

### Requirement: team-chat requires a tagged SDK and shares make and lint files by reference

`team-chat/go.mod` SHALL `require github.com/buidangphuc/agora/platform-core/packages/go-sdk v1.0.2`, SHALL NOT contain a `replace` for it and SHALL pin `golangci-lint` as a `tool`. The repository SHALL NOT contain `team-chat/third_party`, `team-chat/.golangci.yml` or `team-chat/make/`. `team-chat/Makefile` SHALL set `SERVICE := team-chat`, `GRPC_PORT := 50057` and `LINT_CONFIG := ../platform-core/packages/go-sdk/.golangci.yml` and include `../platform-core/make/go-service.mk`. `cmd/server/main.go` SHALL only load settings and call `internal/app.Run`, or `internal/app.Migrate` for the arguments `migrate up`.

#### Scenario: go.mod requires the tag without replace
- **WHEN** `team-chat/go.mod` is parsed
- **THEN** it requires the SDK at `v1.0.2`, has no `replace` line, and lists `github.com/golangci/golangci-lint` under `tool`
- **VERIFIED BY**: `team-chat/internal/config` `TestGoModRequiresTagWithoutReplace`

#### Scenario: Make and lint files are referenced, not copied
- **WHEN** the `team-chat` directory is listed and its `Makefile` read
- **THEN** no `third_party`, `.golangci.yml` or `make` entry exists, and the Makefile includes the shared make file and sets the shared `LINT_CONFIG`
- **VERIFIED BY**: `team-chat/internal/config` `TestSharedMakeAndLintAreReferenced`

#### Scenario: main stays a thin entry point
- **WHEN** `cmd/server/main.go` is read
- **THEN** it imports `internal/app` and `internal/config`, calls `app.Migrate` for the arguments `migrate up` and `app.Run` otherwise, and defines no other function than `main` and `run`
- **VERIFIED BY**: `team-chat/internal/config` `TestMainOnlyCallsAppRun`

### Requirement: Personal data handling is unchanged

Message content, `payload`, `sender_name` and the thread participant ids are personal data processed to deliver messages between a buyer and a seller; the basis is legitimate interest (settled in design Clarifications) and retention is unchanged by this change. Outbox payloads and logs SHALL carry no more personal data than today.

#### Scenario: The event envelope carries no more than today
- **WHEN** `BuildMessageOutboxRow` builds a row for a message with content `s3cret-needle`
- **THEN** the envelope payload decodes to a `ChatMessage` holding exactly the fields it holds on main and its bytes do not contain `s3cret-needle` unless `ChatMessage` already carries content on main
- **VERIFIED BY**: `team-chat/internal/events` `TestEnvelopeCarriesNoMoreThanToday`

#### Scenario: A non-participant cannot obtain message content
- **WHEN** a third user calls `GetThreadMessages` on a thread holding `s3cret-needle`
- **THEN** the call returns `PermissionDenied` (forbidden, not a participant) and the response carries no message
- **VERIFIED BY**: `team-chat/internal/rpc` `TestPermissionDeniedLeaksNoParticipantIDs`
