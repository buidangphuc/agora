## ADDED Requirements

### Requirement: team-domain boots on go-sdk with explicit wiring

team-domain SHALL wire go-sdk v0.1.1 in `internal/app.Run(ctx, settings)` per ADR-0016, with no addon registry and no enable flags for always-on capabilities: the database, the Kafka publisher and the outbox relayer are always wired (`DATABASE_ENABLED`, `KAFKA_ENABLED` and `OUTBOX_ENABLED` no longer exist). It SHALL refuse to start when `DATABASE_URL` is unset or the database is unreachable, in any `ENV`. The outbox relayer and the reservation sweeper SHALL run as lifecycle components.

#### Scenario: Missing database URL is rejected
- **WHEN** the service starts without `DATABASE_URL`
- **THEN** settings loading fails with an error naming `DATABASE_URL` before any port opens
- **VERIFIED BY**: `team-domain/internal/app` `TestBootRequiresDatabase`

#### Scenario: Unreachable database fails boot even locally
- **WHEN** the service starts with `ENV=local` and `DATABASE_URL` pointing at a closed port
- **THEN** `app.Run` returns an error
- **VERIFIED BY**: `TestBootFailsWhenDatabaseUnreachable`

### Requirement: Shutdown drains before stopping background work

On shutdown, team-domain SHALL set health NOT_SERVING first, then stop the gRPC server, then stop the relayer and sweeper, then close the publisher and the pool.

#### Scenario: Relayer stops after the server and before the pool closes
- **WHEN** the running service's context is cancelled
- **THEN** health becomes NOT_SERVING, the relayer's Run returns, and only then is the pool closed
- **VERIFIED BY**: `TestShutdownStopsRelayerBeforePoolClose`

#### Scenario: The publisher is not used after it is closed
- **WHEN** shutdown closes the Kafka publisher
- **THEN** it is closed exactly once and no publish happens after close
- **VERIFIED BY**: `TestShutdownStopsRelayerBeforePoolClose`

### Requirement: Listing behaviour is unchanged

The migration SHALL NOT change RPC, event or storage behaviour. All existing tests SHALL move with their packages and pass with unchanged assertions. The only allowed edits are test assertions on auth status message text, which must keep the same codes.

#### Scenario: Existing suites stay green after the move
- **WHEN** `make check` runs after the restructure, with Docker for the Postgres tests
- **THEN** every pre-existing test in handler, service, repository, events, config and grpcserver, now in rpc, listing, store, events and config, passes
- **VERIFIED BY**: `team-domain` `make check`

#### Scenario: Anonymous callers are still denied stock RPCs with PermissionDenied
- **WHEN** a stock RPC is called with the gateway's anonymous principal
- **THEN** the caller receives `PermissionDenied`, not `Unauthenticated`, so anonymous browsing with public scopes keeps working
- **VERIFIED BY**: `TestAnonymousStockCallIsPermissionDenied`
