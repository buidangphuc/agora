## ADDED Requirements

### Requirement: Settings come from the SDK groups and name every variable

team-engagement SHALL load its settings with `config.Load` of go-sdk v1.0.2 from the groups `Runtime`, `GRPC`, `Postgres`, `OTel`, `Kafka`, `Outbox`, `Audit`, `LogKeyFP` and the service groups `Events` (`KAFKA_CONSUMER_GROUP`, `KAFKA_LISTING_TOPIC`, `ENGAGEMENT_EVENTS_TOPIC`), `Upstream` (`UPSTREAM_ORDER_ADDR`) and `Reports` (`REPORT_RETENTION_DAYS`, `REPORT_SWEEP_INTERVAL`). `ENV`, `GRPC_PORT` (positive, no SDK default), `DATABASE_URL`, `UPSTREAM_ORDER_ADDR` and `UPSTREAM_AUDIT_ADDR` SHALL be required in every `ENV`: every upstream the service calls has a required address and there is no mode without it; the SDK `Audit` group alone would accept an empty address in `local` and `test`, so the service SHALL add its own check after the SDK groups. The service image and `.env.example` SHALL keep supplying `GRPC_PORT=50054`. The service SHALL NOT declare `DATABASE_ENABLED`, `KAFKA_ENABLED`, `ENGAGEMENT_OUTBOX_RELAY_INTERVAL`, `SHUTDOWN_GRACE_SECONDS` or `UPSTREAM_CALL_TIMEOUT_SECONDS`: every call to team-order and team-audit is bounded by the SDK's fixed 2 s. `Load` SHALL refuse `GRPC_REFLECTION_ENABLED=true` whenever `ENV` is anything other than `local` or `test` (staging, prod and any unrecognised value such as `dev`). When `ENV` is staging, stage, prod or production (case-insensitive), `Load` SHALL also refuse an empty `KAFKA_BROKERS` or `LOG_KEY_FP_SECRET`; an unrecognised `ENV` is not strict for those two. `.env.example` SHALL list exactly the declared variables.

#### Scenario: Engagement SDK: Missing ENV is refused
- **WHEN** `Load` runs with `ENV` unset and `GRPC_PORT=50054`, `DATABASE_URL`, `UPSTREAM_ORDER_ADDR` and `UPSTREAM_AUDIT_ADDR` set
- **THEN** it returns an error that names `ENV`
- **VERIFIED BY**: team-engagement `internal/config` `TestLoadRefusesMissingEnv`

#### Scenario: Engagement SDK: Empty database url is refused in every ENV
- **WHEN** `Load` runs with `ENV=local`, `GRPC_PORT=50054`, `UPSTREAM_ORDER_ADDR` and `UPSTREAM_AUDIT_ADDR` set and an empty `DATABASE_URL`
- **THEN** it returns an error that names `DATABASE_URL`
- **VERIFIED BY**: team-engagement `internal/config` `TestLoadRefusesEmptyDatabaseURL`

#### Scenario: Engagement SDK: Empty order address is refused in every ENV
- **WHEN** `Load` runs with `ENV=local`, `GRPC_PORT=50054`, `DATABASE_URL` and `UPSTREAM_AUDIT_ADDR` set and an empty `UPSTREAM_ORDER_ADDR`
- **THEN** it returns an error that names `UPSTREAM_ORDER_ADDR`
- **VERIFIED BY**: team-engagement `internal/config` `TestLoadRefusesEmptyOrderAddr`

#### Scenario: Engagement SDK: Empty audit address is refused in every ENV
- **WHEN** `Load` runs with otherwise valid settings and an empty `UPSTREAM_AUDIT_ADDR`, once with `ENV=local`, once with `ENV=test` and once with `ENV=dev`
- **THEN** each call returns an error that names `UPSTREAM_AUDIT_ADDR`
- **VERIFIED BY**: team-engagement `internal/config` `TestLoadRefusesEmptyAuditAddr`

#### Scenario: Engagement SDK: Missing port is refused
- **WHEN** `Load` runs with `ENV=local`, `DATABASE_URL`, `UPSTREAM_ORDER_ADDR` and `UPSTREAM_AUDIT_ADDR` set and `GRPC_PORT` unset, and then with `GRPC_PORT=` empty
- **THEN** both calls return an error that names `GRPC_PORT`
- **VERIFIED BY**: team-engagement `internal/config` `TestLoadRefusesMissingGRPCPort`

#### Scenario: Engagement SDK: Removed switches are not declared and have no effect
- **WHEN** `Load` runs with valid settings including `GRPC_PORT=50054` and also `DATABASE_ENABLED=false`, `KAFKA_ENABLED=false`, `ENGAGEMENT_OUTBOX_RELAY_INTERVAL=1h` and `UPSTREAM_CALL_TIMEOUT_SECONDS=30`
- **THEN** it succeeds with the database still required, `DeclaredEnvKeys` contains none of the five removed names, and the outbox poll interval is the `OUTBOX_POLL_INTERVAL` default of 1 s
- **VERIFIED BY**: team-engagement `internal/config` `TestRemovedSwitchesAreNotDeclared`

#### Scenario: Engagement SDK: Strict ENV refuses reflection
- **WHEN** `Load` runs with otherwise valid settings and `GRPC_REFLECTION_ENABLED=true`, once with `ENV=staging` and once with `ENV=prod`
- **THEN** each call returns an error that names `GRPC_REFLECTION_ENABLED`
- **VERIFIED BY**: team-engagement `internal/config` `TestStrictEnvRefusesReflection`

#### Scenario: Engagement SDK: Unrecognised ENV refuses reflection but is not strict
- **WHEN** `Load` runs with `ENV=dev`, `GRPC_REFLECTION_ENABLED=true` and then with `ENV=dev`, `GRPC_REFLECTION_ENABLED=false` and `KAFKA_BROKERS` and `LOG_KEY_FP_SECRET` empty (and `UPSTREAM_AUDIT_ADDR` set)
- **THEN** the first call returns an error that names `GRPC_REFLECTION_ENABLED` and the second succeeds with the SDK local Kafka default
- **VERIFIED BY**: team-engagement `internal/config` `TestUnrecognisedEnvRefusesReflectionButIsNotStrict`

#### Scenario: Engagement SDK: Local and test ENV allow reflection
- **WHEN** `Load` runs with `GRPC_REFLECTION_ENABLED=true` and `ENV=local`, and then with `ENV=test`
- **THEN** both calls succeed with reflection true
- **VERIFIED BY**: team-engagement `internal/config` `TestLocalAndTestAllowReflection`

#### Scenario: Engagement SDK: Strict ENV requires brokers, audit address and fingerprint secret
- **WHEN** `Load` runs with `ENV=prod`, `GRPC_PORT=50054`, `DATABASE_URL` and `UPSTREAM_ORDER_ADDR` set and `KAFKA_BROKERS`, `UPSTREAM_AUDIT_ADDR` and `LOG_KEY_FP_SECRET` all empty
- **THEN** it returns one error that names all three variables (the audit address is refused in every `ENV`, brokers and secret only in a strict one)
- **VERIFIED BY**: team-engagement `internal/config` `TestStrictEnvRequiresBrokersAuditAndSecret`

#### Scenario: Engagement SDK: Local defaults
- **WHEN** `Load` runs with `ENV=local`, `GRPC_PORT=50054`, a `DATABASE_URL`, a `UPSTREAM_ORDER_ADDR` and a `UPSTREAM_AUDIT_ADDR` only
- **THEN** it succeeds, `GRPC_PORT` is 50054, reflection is false, the listing topic is `listing.events`, the consumer group is `team-engagement-feed`, the events topic is `engagement.events` and `REPORT_RETENTION_DAYS` is 730
- **VERIFIED BY**: team-engagement `internal/config` `TestLoadLocalDefaults`

#### Scenario: Engagement SDK: Example file matches the declared variables
- **WHEN** `.env.example` and the declared variables of the settings are compared in both directions
- **THEN** no variable is missing on either side
- **VERIFIED BY**: team-engagement `internal/config` `TestEnvExampleInSync`

### Requirement: The service follows the SDK layout and keeps no runtime fallback

The module SHALL require `github.com/buidangphuc/agora/platform-core/packages/go-sdk` at tag `v1.0.2` with no `replace` directive and no `third_party` directory. The Makefile SHALL include `../platform-core/make/go-service.mk` and lint with `../platform-core/packages/go-sdk/.golangci.yml`; `golangci-lint` SHALL be pinned as a `tool` in `go.mod`. `cmd/server/main.go` SHALL only dispatch the `migrate up` subcommand to `internal/app.Migrate` or load settings and call `internal/app.Run`. The packages `bootstrap`, `grpcserver`, `observability`, `interceptor`, `handler`, `service`, `repository` and `consumer` SHALL NOT exist. Non-test Go files SHALL NOT contain `InMemory`, `Noop` or `RequireDurableStorage`, no non-test file SHALL import the fake-store package `internal/store/storetest`, and the service SHALL NOT read an environment variable that switches Postgres, Kafka, the outbox, a consumer or the sweep on or off.

#### Scenario: Engagement SDK: Module requires the tag and nothing else
- **WHEN** `go.mod` is parsed and the directory tree is listed
- **THEN** the SDK requirement is exactly `v1.0.2`, there is no `replace` line and no `third_party` directory
- **VERIFIED BY**: team-engagement `internal/docs` `TestModuleUsesTaggedSDK`

#### Scenario: Engagement SDK: Old packages are gone
- **WHEN** the packages under `internal/` are listed
- **THEN** none of `bootstrap`, `grpcserver`, `observability`, `interceptor`, `handler`, `service`, `repository`, `consumer` is present and `app`, `config`, `rpc`, `engagement`, `store`, `events`, `upstream` are
- **VERIFIED BY**: team-engagement `internal/docs` `TestLayoutFollowsADR0016`

#### Scenario: Engagement SDK: No fallback code in the binary
- **WHEN** every non-test `.go` file under `cmd` and `internal` is scanned
- **THEN** none contains `InMemory`, `Noop`, `RequireDurableStorage`, `DATABASE_ENABLED`, `KAFKA_ENABLED` or `_ENABLED` other than `OTEL_ENABLED` and `GRPC_REFLECTION_ENABLED`, and none outside `internal/store/storetest` imports `internal/store/storetest`
- **VERIFIED BY**: team-engagement `internal/docs` `TestNoRuntimeFallbacks`

#### Scenario: Engagement SDK: A reintroduced enable flag fails the scan
- **WHEN** the scan runs over a fixture tree whose non-test file declares `KAFKA_ENABLED` or constructs `NewInMemoryRepository`
- **THEN** the scan reports a violation for that file and the layout gate fails
- **VERIFIED BY**: team-engagement `internal/docs` `TestNoRuntimeFallbacksDetectsViolation`

#### Scenario: Engagement SDK: Makefile uses the shared files
- **WHEN** the Makefile is read
- **THEN** it includes `../platform-core/make/go-service.mk`, sets `LINT_CONFIG` to `../platform-core/packages/go-sdk/.golangci.yml`, and no `.golangci.yml` exists in the service
- **VERIFIED BY**: team-engagement `internal/docs` `TestMakefileUsesSharedFiles`

### Requirement: Migrations are applied by a subcommand and never at boot

The binary SHALL implement `migrate up`: it SHALL apply every pending migration of the embedded `migrations` files with the SDK `migrate.Up` against `DATABASE_URL` and exit 0, also when nothing is pending, and exit non-zero with the error when the database is unreachable or a migration fails. The subcommand SHALL load only the `Runtime` and `Postgres` settings groups, so a migrate job needs neither Kafka nor upstream variables. `app.Run` SHALL NOT apply any migration. The migrations SHALL be embedded in the image (`migrations/embed.go`), so the same image serves the service and its migrate job.

#### Scenario: Engagement SDK: migrate up applies the embedded migrations
- **WHEN** `app.Migrate` runs against an empty Postgres with the embedded files `0001` to `0014`
- **THEN** it returns nil, every table of `0001` to `0014` exists including `engagement_outbox_events`, and the version recorded by golang-migrate is 14
- **VERIFIED BY**: team-engagement `internal/app` `TestMigrateUpAppliesEmbeddedMigrations`

#### Scenario: Engagement SDK: migrate up with nothing pending succeeds
- **WHEN** `app.Migrate` runs a second time against the same database
- **THEN** it returns nil and the recorded version is still 14
- **VERIFIED BY**: team-engagement `internal/app` `TestMigrateUpTwiceIsNoOp`

#### Scenario: Engagement SDK: migrate up fails loudly
- **WHEN** `app.Migrate` runs with a `DATABASE_URL` that points at a closed port, and `main` runs with the argument `migrate up` and that variable
- **THEN** `app.Migrate` returns a non-nil error and the process exits with a non-zero status
- **VERIFIED BY**: team-engagement `internal/app` `TestMigrateUpUnreachableDatabaseFails`

#### Scenario: Engagement SDK: The service never migrates at boot
- **WHEN** `app.Run` starts against a database on which no migration was applied
- **THEN** no table and no `schema_migrations` row exists after `app.Run` returned
- **VERIFIED BY**: team-engagement `internal/app` `TestRunDoesNotMigrate`

#### Scenario: Engagement SDK: The migrate job needs only the database url
- **WHEN** the `migrate up` settings are loaded with only `ENV=local` and `DATABASE_URL` set
- **THEN** loading succeeds and `UPSTREAM_ORDER_ADDR`, `UPSTREAM_AUDIT_ADDR` and `KAFKA_BROKERS` are not required
- **VERIFIED BY**: team-engagement `internal/config` `TestMigrateSettingsNeedOnlyDatabase`

### Requirement: The database is required at boot and readiness follows it

`app.Run` SHALL open Postgres with `postgres.Open` in every `ENV` and SHALL return an error before the gRPC port opens when the database is unreachable. The health server SHALL register one readiness check, `postgres`; Kafka SHALL NOT be a readiness check (readiness is Postgres only, because engagement has no required upstream health check: team-order and team-audit are called per request and fail the request, not the pod). The overall status (service name empty) SHALL be NOT_SERVING while the check fails. The `liveness` service SHALL stay SERVING while the check fails. A Kafka outage SHALL NOT change either status.

#### Scenario: Engagement SDK: Unreachable database fails boot
- **WHEN** `app.Run` starts with a `DATABASE_URL` that points at a closed port
- **THEN** it returns an error within 6 s and the gRPC port is never opened
- **VERIFIED BY**: team-engagement `internal/app` `TestRunFailsFastWhenDatabaseUnreachable`

#### Scenario: Engagement SDK: Database outage flips readiness but not liveness
- **WHEN** the running service loses its Postgres connection
- **THEN** within two health intervals `Check("")` returns NOT_SERVING while `Check("liveness")` still returns SERVING
- **VERIFIED BY**: team-engagement `internal/app` `TestHealthLivenessSurvivesDatabaseOutage`

#### Scenario: Engagement SDK: Kafka outage keeps the service ready
- **WHEN** the broker stops while the service runs and a buyer calls `AddFavorite`
- **THEN** `Check("")` stays SERVING, the call returns OK and one pending `FavoriteAdded` row exists in `engagement_outbox_events`
- **VERIFIED BY**: team-engagement `internal/app` `TestKafkaOutageKeepsReadinessAndQueuesFact`

#### Scenario: Engagement SDK: Shutdown drains before workers stop
- **WHEN** the process receives SIGTERM while an RPC that writes an outbox row is in flight
- **THEN** both health names report NOT_SERVING, the in-flight RPC completes with OK, and only after the gRPC server returned do the relayer, the retention job, the sweep and the listing consumer stop
- **VERIFIED BY**: team-engagement `internal/app` `TestShutdownOrder`

#### Scenario: Engagement SDK: Liveness answers fast
- **WHEN** `Check("liveness")` is called 1000 times while Postgres is down
- **THEN** every answer is SERVING and the 99th percentile latency is under 5 ms
- **VERIFIED BY**: team-engagement `internal/app` `TestLivenessCheckLatency`

#### Scenario: Engagement SDK: A committed fact reaches the topic within 2 s
- **WHEN** a buyer calls `AddFavorite` with `OUTBOX_POLL_INTERVAL` at its default of 1 s and a healthy broker
- **THEN** the `FavoriteAdded` envelope is read from `engagement.events` within 2 s of the call returning
- **VERIFIED BY**: team-engagement `internal/events` `TestFactReachesTopicWithinTwoSeconds`

### Requirement: gRPC errors go through one error table

Every error that a handler gets back from the service layer or the store SHALL be converted by `grpcx.Status` with one `ErrorTable` whose rules carry the code and the client-visible message of today's handlers (the SDK would otherwise send the sentinel's own text). The table SHALL contain exactly these rules, in this order (every sentinel declared in `internal/engagement` and `internal/store` appears in a row, and `TestErrorTableKeepsDomainCodes` iterates the same list):

| Sentinel | Code | Message |
|---|---|---|
| `ErrOrderNotEligible` | `FailedPrecondition` | `order is not eligible for review` |
| `ErrOrderUnavailable` | `Unavailable` | `order verification unavailable` |
| `ErrDuplicateReview` | `AlreadyExists` | `review already exists for this order and listing` |
| `ErrOpenDisputeExists` | `AlreadyExists` | `an open dispute already exists for this return` |
| `ErrInvalidCursor` (the malformed feed cursor of `ListFollowedListings`, the only RPC that returns it) | `InvalidArgument` | `invalid cursor` |
| `ErrReviewNotFound` | `NotFound` | `review not found` |
| `ErrQuestionNotFound` | `NotFound` | `question not found` |
| `ErrCollectionNotFound` | `NotFound` | `collection not found` |
| `ErrReportNotFound` | `NotFound` | `report not found` |
| `ErrDisputeNotFound` | `NotFound` | `dispute not found` |
| `upstream.ErrOrderNotFound` | `NotFound` | `order not found` |
| `upstream.ErrReturnNotFound` | `NotFound` | `return not found` |
| `ErrDisputeClosed` | `FailedPrecondition` | `dispute is already closed` |
| `ErrInvalidTransition` | `FailedPrecondition` | `invalid dispute status transition` |
| `ErrDisputeNotInState` | `FailedPrecondition` | `dispute is not in the expected status` |
| `ErrReportClosed` | `FailedPrecondition` | `report is already closed` |
| `ErrOwnListing` | `FailedPrecondition` | the sentinel text |
| `ErrInvalidDisputeStatus` | `InvalidArgument` | `invalid dispute status` |
| `ErrRulingRequired`, `ErrRulingNotAllowed`, `ErrSameClaimantAndDef` | `InvalidArgument` | the sentinel text |
| `ErrInvalidRating`, `ErrEmptyListing`, `ErrEmptyReviewID`, `ErrEmptySeller`, `ErrReasonTooLong` (review) | `InvalidArgument` | the sentinel text |
| `ErrEmptyQuestionText`, `ErrEmptyAnswerText`, `ErrEmptyQuestionID`, `ErrEmptyUserID` (Q&A) | `InvalidArgument` | the sentinel text |
| `ErrEmptyDisputeID`, `ErrEmptyOrderID`, `ErrEmptyClaimantID`, `ErrEmptyDefendantID`, `ErrEmptyReason` (dispute) | `InvalidArgument` | the sentinel text |
| `ErrEmptyCollectionName`, `ErrEmptyCollectionID` (wishlist) | `InvalidArgument` | the sentinel text |
| `ErrInvalidReportTarget`, `ErrInvalidReportReason`, `ErrReportNoteTooLong`, `ErrEmptyReportID`, `ErrInvalidReportStatus`, `ErrResolutionNoteRequired`, `ErrResolutionNoteTooLong` (report) | `InvalidArgument` | the sentinel text |
| `ErrReportCapReached` | `ResourceExhausted` | the sentinel text |

`upstream.ErrOrderIDRequired` has no rule: it is a guard of the verifier, and the review service rejects an empty order id with `ErrEmptyOrderID` before it calls the verifier, so it never reaches the table (if it did it would become `Internal` `internal error`). `ErrAuditUnavailable` and `ErrOrderClientNotConfigured` are deleted. Cursor errors keep exactly today's behaviour, RPC by RPC: `ListFollowedListings` answers `InvalidArgument` `invalid cursor` through `ErrInvalidCursor` (the table row); `ListReports` answers `InvalidArgument` `invalid cursor` and `ListReviews` `InvalidArgument` `invalid page cursor` for an offset cursor that is not a non-negative integer, both as explicit status errors written by the handler (not table rows, not `ErrInvalidCursor`); `ListDisputes` with a cursor that is not base64url of `<unix-micro>|<id>` returns `Internal` `internal error` today, because the store's parse error is not a sentinel, and it SHALL keep doing so (accepted: no behaviour change, no new sentinel); `ListFavorites`, `ListFollowedSellers`, `GetRecentlyViewed` and `ListCollectionItems` use an id keyset cursor that is never validated (`ListCollections` has no paging), and answer OK with an empty page for a cursor that matches nothing.

Unmatched SQLSTATE 23505 SHALL map to `AlreadyExists` with the SDK message `already exists`, `context.Canceled` to `Canceled` and `context.DeadlineExceeded` to `DeadlineExceeded`. A status error passes through unchanged (the audit gate's `Unavailable` `audit trail unavailable` and the auth helpers' errors). An error that matches no rule SHALL become `Internal` with the fixed message `internal error`; the text of a wrapped driver or upstream error SHALL NOT reach the caller. The `CreateDispute` outcomes that depend on a team-order lookup rather than on a sentinel SHALL stay explicit `status.Error` calls with today's code and text: unknown order or order not owned by the caller `NotFound` `order not found`, defendant not the order's seller `PermissionDenied` (`InvalidArgument` when a `return_id` is sent) `defendant must be the order's seller`, a return of another order `NotFound` `return not found`, a return that cannot be escalated `FailedPrecondition` `return cannot be escalated`, a failing order or return lookup `Unavailable` `order verification unavailable` or `return verification unavailable`. Request-validation errors that name a field (`listing_id is required`) SHALL stay explicit `InvalidArgument` calls. The party checks SHALL stay explicit `status.Error` calls with today's code and text: a caller who is neither buyer nor seller nor admin gets `NotFound` `order not found` from `CreateDispute` and `NotFound` `dispute not found` from `GetDispute` (identical to a missing id, so dispute ids cannot be probed), and `PermissionDenied` `not a party to this order` from `ListOrderDisputes` (an unknown order id gets the same answer); a failing order lookup in `ListOrderDisputes` is `Unavailable` `order verification unavailable`.

#### Scenario: Engagement SDK: Unique violation is AlreadyExists
- **WHEN** a store call returns a `pgconn.PgError` with SQLSTATE 23505 that no sentinel wraps
- **THEN** the RPC returns `AlreadyExists` with message `already exists` that contains neither the constraint name nor the table name
- **VERIFIED BY**: team-engagement `internal/rpc` `TestErrorTableUniqueViolationAlreadyExists`

#### Scenario: Engagement SDK: Duplicate review keeps its message
- **WHEN** a buyer reviews the same order and listing twice, the second insert failing with SQLSTATE 23505 wrapped by `ErrDuplicateReview`
- **THEN** the second call returns `AlreadyExists` with message `review already exists for this order and listing`
- **VERIFIED BY**: team-engagement `internal/rpc` `TestCreateReviewDuplicateKeepsMessage`

#### Scenario: Engagement SDK: Ineligible order keeps its message
- **WHEN** `CreateReview` is called for an order that is not delivered or not the caller's
- **THEN** the call returns `FailedPrecondition` with message `order is not eligible for review`
- **VERIFIED BY**: team-engagement `internal/rpc` `TestCreateReviewIneligibleKeepsMessage`

#### Scenario: Engagement SDK: Open dispute for a return keeps its message
- **WHEN** a buyer opens a second dispute for a `return_id` that already has an OPEN dispute, the store failing with SQLSTATE 23505 wrapped by `ErrOpenDisputeExists`
- **THEN** the call returns `AlreadyExists` with message `an open dispute already exists for this return`
- **VERIFIED BY**: team-engagement `internal/rpc` `TestCreateDisputeOpenExistsKeepsMessage`

#### Scenario: Engagement SDK: Unknown or foreign order is not found
- **WHEN** a buyer calls `CreateDispute` for an order id unknown to team-order, and then for an order whose buyer is someone else
- **THEN** both calls return `NotFound` with message `order not found` and no dispute row exists
- **VERIFIED BY**: team-engagement `internal/rpc` `TestCreateDisputeOrderNotFoundKeepsMessage`

#### Scenario: Engagement SDK: Unknown return or return of another order is not found
- **WHEN** a buyer calls `CreateDispute` with a `return_id` unknown to team-order, and then with a return that belongs to another order
- **THEN** both calls return `NotFound` with message `return not found`
- **VERIFIED BY**: team-engagement `internal/rpc` `TestCreateDisputeReturnNotFoundKeepsMessage`

#### Scenario: Engagement SDK: A return that cannot be escalated is refused
- **WHEN** a buyer calls `CreateDispute` with a `return_id` whose team-order answer has `can_escalate` false
- **THEN** the call returns `FailedPrecondition` with message `return cannot be escalated` and no dispute row exists
- **VERIFIED BY**: team-engagement `internal/rpc` `TestCreateDisputeReturnCannotEscalateKeepsMessage`

#### Scenario: Engagement SDK: Failing order or return lookup is Unavailable
- **WHEN** team-order returns an error (not `NotFound`) for `GetOrder`, and in a second case for `GetReturn`
- **THEN** the calls return `Unavailable` with messages `order verification unavailable` and `return verification unavailable` respectively and no dispute row exists
- **VERIFIED BY**: team-engagement `internal/rpc` `TestCreateDisputeLookupFailureKeepsMessage`

#### Scenario: Engagement SDK: Order listing with a failing lookup is Unavailable
- **WHEN** a buyer calls `ListOrderDisputes` and team-order returns an error that is not `NotFound`
- **THEN** the call returns `Unavailable` with message `order verification unavailable`
- **VERIFIED BY**: team-engagement `internal/rpc` `TestListOrderDisputesLookupFailureKeepsMessage`

#### Scenario: Engagement SDK: Unknown question is not found
- **WHEN** `AnswerQuestion` is called with an unknown question id, with and without `is_shop_reply`
- **THEN** both calls return `NotFound` with message `question not found`
- **VERIFIED BY**: team-engagement `internal/rpc` `TestAnswerQuestionNotFoundKeepsMessage`

#### Scenario: Engagement SDK: Context errors keep their codes
- **WHEN** a store call returns `context.Canceled` and another returns `context.DeadlineExceeded`
- **THEN** the RPCs return `Canceled` and `DeadlineExceeded` respectively
- **VERIFIED BY**: team-engagement `internal/rpc` `TestErrorTableContextErrors`

#### Scenario: Engagement SDK: Unmapped error is a fixed Internal
- **WHEN** a store call returns `errors.New("connection refused to 10.0.0.7:5432")`
- **THEN** the RPC returns `Internal` with message `internal error` and the text "10.0.0.7" appears in the server log only
- **VERIFIED BY**: team-engagement `internal/rpc` `TestErrorTableHidesInternalDetail`

#### Scenario: Engagement SDK: Domain sentinels keep their codes and messages
- **WHEN** each sentinel of the table is returned by the service layer, bare and wrapped with `fmt.Errorf("...: %w", err)`
- **THEN** the RPC code and message equal the row of the table for every sentinel, and the wrapping text appears in neither
- **VERIFIED BY**: team-engagement `internal/rpc` `TestErrorTableKeepsDomainCodes`

#### Scenario: Engagement SDK: A status error from the audit gate passes through
- **WHEN** the audit gate returns `Unavailable` `audit trail unavailable` to a handler
- **THEN** the RPC returns that code and message unchanged and not `Internal`
- **VERIFIED BY**: team-engagement `internal/rpc` `TestAuditGateStatusPassesThroughTable`

### Requirement: Principals come from the SDK and every RPC keeps its present gate

The server SHALL resolve the caller from the gateway-forwarded `x-principal-*` metadata with `sdk/auth`. Each of the 36 `EngagementService` RPCs SHALL keep exactly the gate it has today, listed here; no RPC gains or loses a gate in this change:

| Gate | RPCs |
|---|---|
| `admin` (authenticated caller holding the `admin` scope; anonymous gets `Unauthenticated`, a caller without the scope `PermissionDenied`, both before request validation and before any upstream call) | `HideReview`, `ResolveDispute`, `ListDisputes`, `ListReports`, `ResolveReport` |
| `engagement:write` (no principal gets `Unauthenticated`, a principal without the scope `PermissionDenied`, both before any store call) | `AddFavorite`, `RemoveFavorite`, `FollowSeller`, `UnfollowSeller`, `CreateReview`, `MarkReviewHelpful`, `AskQuestion`, `AnswerQuestion`, `CreateDispute`, `CheckIn`, `CreateReport`, `CreateCollection`, `AddToCollection`, `RemoveFromCollection` |
| `engagement:read` (same answers as above) | `IsFavorite`, `ListFavorites`, `IsFollowing`, `ListFollowedSellers`, `ListFollowedListings`, `GetDispute`, `ListOrderDisputes`, `GetLoyalty`, `ListCollections`, `ListCollectionItems` |
| none (public: no principal and no scope required, an anonymous caller is served) | `RecordView`, `GetRecentlyViewed`, `GetListingStats`, `ListReviews`, `GetListingRatingSummary`, `GetShopRatingSummary`, `ListQuestionsByListing` |

The 5 + 14 + 10 + 7 rows cover all 36 RPCs. Behaviour of the public RPCs for an anonymous caller stays as today: `RecordView` counts the view with an empty user id, `GetRecentlyViewed` returns an empty list with no error, the others do not look at the caller. The gates are enforced with `auth.RequireScopes` and `auth.RequireAdmin`-equivalent helpers over `sdk/auth`; the client-visible text of an auth refusal MAY differ from today's text (`no principal on context`, `insufficient_scope: missing ...`), the codes SHALL NOT. Dispute reads and creation SHALL stay limited to the parties of the order and to admin; a stranger SHALL get the `NotFound` or `PermissionDenied` answers listed in the error table requirement, never a dispute row. `is_shop_reply` SHALL be honoured only for the listing owner; any other caller gets an ordinary answer (no denial), and an unknown question stays `NotFound`.

#### Scenario: Engagement SDK: Anonymous caller is unauthenticated on admin RPCs
- **WHEN** a call without principal metadata reaches each of the five admin RPCs
- **THEN** each returns `Unauthenticated` and no audit event is written
- **VERIFIED BY**: team-engagement `internal/rpc` `TestAdminRpcsRejectAnonymous`

#### Scenario: Engagement SDK: Non-admin is denied on admin RPCs
- **WHEN** a user principal with scope `user` calls each of the five admin RPCs with an invalid request body
- **THEN** each returns `PermissionDenied` (not `InvalidArgument`) and no upstream is called
- **VERIFIED BY**: team-engagement `internal/rpc` `TestAdminRpcsDenyNonAdminBeforeValidation`

#### Scenario: Engagement SDK: Read scope alone cannot write
- **WHEN** a user principal holding only scope `engagement:read` calls each of the 14 `engagement:write` RPCs, and a user holding only `engagement:write` calls each of the 10 `engagement:read` RPCs, with valid requests
- **THEN** all 24 calls return `PermissionDenied`, no store call is made and no `FavoriteAdded` row exists
- **VERIFIED BY**: team-engagement `internal/rpc` `TestEngagementScopesAreEnforced`

#### Scenario: Engagement SDK: Anonymous caller is unauthenticated on scoped RPCs
- **WHEN** a call without principal metadata reaches each of the 14 `engagement:write` and 10 `engagement:read` RPCs
- **THEN** each returns `Unauthenticated` and no store call is made
- **VERIFIED BY**: team-engagement `internal/rpc` `TestScopedRpcsRejectAnonymous`

#### Scenario: Engagement SDK: Public RPCs serve anonymous callers
- **WHEN** a call without principal metadata and a call by a user holding no engagement scope reach each of `RecordView`, `GetRecentlyViewed`, `GetListingStats`, `ListReviews`, `GetListingRatingSummary`, `GetShopRatingSummary` and `ListQuestionsByListing` with a valid request
- **THEN** all 14 calls return OK, and the anonymous `GetRecentlyViewed` returns an empty list
- **VERIFIED BY**: team-engagement `internal/rpc` `TestPublicRpcsServeAnonymousCallers`

#### Scenario: Engagement SDK: Every RPC has a classified gate
- **WHEN** the methods of the generated `EngagementService` descriptor are compared with the gate table of this requirement
- **THEN** the descriptor has 36 methods, each appears in exactly one row (5 admin, 14 write, 10 read, 7 public), and an unclassified method fails the test
- **VERIFIED BY**: team-engagement `internal/rpc` `TestEveryRpcHasItsGate`

#### Scenario: Engagement SDK: A stranger cannot open or read a dispute
- **WHEN** a user who is neither buyer nor seller nor admin calls `CreateDispute` for the order, then `GetDispute` for an existing dispute of it, then `ListOrderDisputes` for the order
- **THEN** `CreateDispute` returns `NotFound` `order not found` and no dispute row exists, `GetDispute` returns `NotFound` `dispute not found` with the same code and text as for a missing id, and `ListOrderDisputes` returns `PermissionDenied` `not a party to this order`
- **VERIFIED BY**: team-engagement `internal/rpc` `TestDisputePartiesOnly`

#### Scenario: Engagement SDK: A non-owner's shop reply is downgraded to an ordinary answer
- **WHEN** a seller who does not own the listing answers a question on it with `is_shop_reply` set
- **THEN** the call returns OK, the stored answer is not marked as a shop reply and the question has that one ordinary answer
- **VERIFIED BY**: team-engagement `internal/rpc` `TestShopReplyOwnerOnly`

### Requirement: Calls to team-order and team-audit carry a service principal

The order client and the audit connection SHALL be built with `upstream.NewClient` and `upstream.AsService` (the audit one through `auditx.Dial`), both with the principal id `service-team-engagement`, which the SDK sends verbatim and which the unmodified specs `review-moderation`, `dispute-audit` and `report-resolution` require. Every call to team-order (`GetOrder` for reviews, `GetOrder` and `GetReturn` for disputes) SHALL carry `x-principal-id=service-team-engagement`, `x-principal-type=service` and scope `order.read`; calls to team-audit SHALL carry `x-principal-id=service-team-engagement`, `x-principal-type=service` and scope `audit.write`; in both cases the inbound caller's principal SHALL NOT be forwarded. A call to team-order and a call to team-audit SHALL each be bounded by the SDK's fixed 2 s or the inbound deadline, whichever is earlier; no environment variable changes that bound.

#### Scenario: Engagement SDK: Order call carries the order.read service principal
- **WHEN** `CreateReview` is called by a user principal and verifies the order against a recording team-order
- **THEN** the recorded metadata has id `service-team-engagement`, type `service` and scope `order.read`, and the user id appears in no `x-principal-*` value
- **VERIFIED BY**: team-engagement `internal/upstream` `TestOrderCallCarriesServicePrincipal`

#### Scenario: Engagement SDK: Audit call carries the audit.write service principal
- **WHEN** an admin calls `HideReview` against a recording team-audit
- **THEN** the recorded metadata has id `service-team-engagement`, type `service` and scope `audit.write`, and the admin id appears only as the event's `actor_id`
- **VERIFIED BY**: team-engagement `internal/upstream` `TestAuditCallCarriesServicePrincipal`

#### Scenario: Engagement SDK: Return lookup carries the order.read service principal
- **WHEN** a buyer calls `CreateDispute` with a `return_id` against a recording team-order
- **THEN** the recorded `GetOrder` and `GetReturn` calls both carry id `service-team-engagement`, type `service` and scope `order.read`, and the buyer id appears in no `x-principal-*` value
- **VERIFIED BY**: team-engagement `internal/upstream` `TestReturnCallCarriesServicePrincipal`

#### Scenario: Engagement SDK: Slow team-order times out
- **WHEN** team-order does not answer within 2 s
- **THEN** the verifier returns an error within 2 s plus 500 ms and `CreateReview` returns `Unavailable` with 0 rows written
- **VERIFIED BY**: team-engagement `internal/upstream` `TestVerifyPurchaseTimeoutIsError`

### Requirement: Privileged actions run behind the audit gate

`HideReview`, `ResolveDispute` and `ResolveReport` SHALL call `auditx.Gate.Do` after authorization, validation, loading and transition checks, with the event they write today (`review.hide`, `dispute.resolve`, `report.resolve`, same actor, target and metadata keys), and SHALL run the state change only as the gate's action. When the audit write fails or times out, the RPC SHALL return `Unavailable` with the message `audit trail unavailable` and the state SHALL be unchanged. The write is bounded by the gate's fixed 2 s. `UPSTREAM_AUDIT_ADDR` is required in every `ENV`, so the gate always has a writer: no nil writer and no mode without an audit connection exists in the binary. The action's own error SHALL reach the caller unchanged, and a retry SHALL write a new audit event.

#### Scenario: Engagement SDK: Audit is written before the state change
- **WHEN** an admin hides a review, resolves a dispute or closes a report against a recording audit and store
- **THEN** the recorder shows exactly one audit event before the one state change, with the existing action name and metadata keys
- **VERIFIED BY**: team-engagement `internal/engagement` `TestGateWritesAuditBeforeStateChange`

#### Scenario: Engagement SDK: Audit failure blocks hiding a review
- **WHEN** team-audit returns an error during `HideReview`
- **THEN** the call returns `Unavailable` with message `audit trail unavailable` and the review's `hidden_at` is NULL
- **VERIFIED BY**: team-engagement `internal/engagement` `TestHideReviewAuditFailureFailsClosedOnGate`

#### Scenario: Engagement SDK: Audit failure blocks resolving a dispute
- **WHEN** team-audit returns an error during `ResolveDispute` for an investigating dispute
- **THEN** the call returns `Unavailable` with message `audit trail unavailable`, the dispute keeps its status and no `DisputeResolved` row is enqueued
- **VERIFIED BY**: team-engagement `internal/engagement` `TestResolveDisputeAuditFailureFailsClosedOnGate`

#### Scenario: Engagement SDK: Audit failure blocks closing a report
- **WHEN** team-audit returns an error during `ResolveReport` for an open report
- **THEN** the call returns `Unavailable` with message `audit trail unavailable` and the report stays `OPEN`
- **VERIFIED BY**: team-engagement `internal/engagement` `TestResolveReportAuditFailureFailsClosedOnGate`

#### Scenario: Engagement SDK: State change failure after audit is retryable
- **WHEN** the audit write succeeds, the store update fails, and the call is retried with a healthy store
- **THEN** the first call returns `Internal`, the retry returns OK, and two audit events exist for the target
- **VERIFIED BY**: team-engagement `internal/engagement` `TestGateActionErrorIsReturnedAndRetryWritesNewEvent`

#### Scenario: Engagement SDK: Audit write is bounded
- **WHEN** team-audit does not answer
- **THEN** the call returns `Unavailable` within 3 s (the gate's 2 s plus 1 s)
- **VERIFIED BY**: team-engagement `internal/upstream` `TestAuditWriterTimeoutIsBounded`

### Requirement: Engagement facts are written to the SDK outbox as full envelopes

Each fact (`FavoriteAdded`, `FavoriteRemoved`, `SellerFollowed`, `SellerUnfollowed`, `ReviewCreated`, `ReviewHidden`, `DisputeResolved`) SHALL be enqueued with `outbox.Store.EnqueueTx` inside the transaction of the change it announces, into table `engagement_outbox_events`. The row payload SHALL be the marshalled `platform.events.v1.EventEnvelope` with a fresh event id, `type` equal to the payload's full proto name, the caller as principal (absent when none), the request id, and `occurred_at`; the row's aggregate id SHALL be the record key (listing id, seller id, or dispute id) and its aggregate type `Listing`, `Seller` or `Dispute`. The service SHALL NOT write to the legacy `outbox` table. A failed business write SHALL leave no row. A call that changes no state SHALL write no row.

#### Scenario: Engagement SDK: Fact row carries the envelope
- **WHEN** buyer `u-1` favourites listing `L1` with request id `r-9`
- **THEN** one row exists with aggregate type `Listing`, aggregate id `L1`, and a payload that unmarshals to an `EventEnvelope` with type `platform.engagement.v1.FavoriteAdded`, principal `u-1` of type USER, request id `r-9` and a payload holding `u-1` and `L1`
- **VERIFIED BY**: team-engagement `internal/events` `TestFactRowBuildsEnvelope`

#### Scenario: Engagement SDK: A fact without a caller has no principal
- **WHEN** a fact is enqueued from a context that carries no principal (an anonymous caller)
- **THEN** the envelope has no principal and the request id of the context
- **VERIFIED BY**: team-engagement `internal/events` `TestFactRowWithoutPrincipalHasNone`

#### Scenario: Engagement SDK: Follow fact is keyed by seller
- **WHEN** a buyer follows seller `S1`
- **THEN** one `SellerFollowed` row exists with aggregate type `Seller` and aggregate id `S1`
- **VERIFIED BY**: team-engagement `internal/store` `TestFollowEnqueuesSellerKeyedFact`

#### Scenario: Engagement SDK: A rolled-back write leaves no fact
- **WHEN** `CreateReview` fails after the review insert inside its transaction
- **THEN** the reviews table has 0 new rows and `engagement_outbox_events` has 0 new rows
- **VERIFIED BY**: team-engagement `internal/store` `TestFactRolledBackWithBusinessWrite`

#### Scenario: Engagement SDK: A repeated favourite writes no second row
- **WHEN** a buyer adds the same listing to their favourites twice
- **THEN** exactly one `FavoriteAdded` row exists
- **VERIFIED BY**: team-engagement `internal/store` `TestRepeatedFavouriteWritesOneFact`

#### Scenario: Engagement SDK: Resolving a dispute writes one DisputeResolved row
- **WHEN** an admin resolves an investigating dispute `D1` with a ruling and a second admin resolves it at the same moment
- **THEN** one call succeeds, the other returns `FailedPrecondition`, and exactly one `DisputeResolved` row exists with aggregate type `Dispute`, aggregate id `D1`, status, ruling and the resolver id of the winner
- **VERIFIED BY**: team-engagement `internal/store` `TestResolveWithEventEnqueuesOneDisputeResolved`

#### Scenario: Engagement SDK: Hiding a review writes ReviewHidden without moderator data
- **WHEN** an admin hides review `R1` of listing `L1`
- **THEN** one `ReviewHidden` row keyed by `L1` exists whose payload holds `review_id`, `listing_id` and `hidden_at` and no reviewer id or review text, and hiding it again writes none
- **VERIFIED BY**: team-engagement `internal/store` `TestHideReviewEnqueuesReviewHiddenOnce`

#### Scenario: Engagement SDK: The legacy outbox is not written
- **WHEN** any fact is written
- **THEN** the row count of the legacy `outbox` table is unchanged
- **VERIFIED BY**: team-engagement `internal/store` `TestLegacyOutboxIsNotWritten`

### Requirement: The relayer publishes facts per record key and never parks on a transient error

`outbox.Relayer` SHALL publish rows to `ENGAGEMENT_EVENTS_TOPIC` with the record key equal to the aggregate id, in `seq` order for each key, claiming `OUTBOX_BATCH_SIZE` rows every `OUTBOX_POLL_INTERVAL`. A transient publish error (broker down, timeout) SHALL leave the row pending and retried with capped backoff and SHALL NOT park it. Published rows older than 7 days SHALL be deleted by a job that runs at start and every hour; pending rows SHALL never be deleted. With two replicas running, each event id SHALL appear exactly once on the topic. Order is the relayer's `seq` order, which is insert order; two transactions that write the same key concurrently can commit in the opposite order, and the topic then follows `seq`, not commit.

#### Scenario: Engagement SDK: Facts of one key arrive in order
- **WHEN** a buyer favourites and then unfavourites listing `L1`
- **THEN** `FavoriteAdded` then `FavoriteRemoved` for `L1` are read from `engagement.events` in that order with key `L1`
- **VERIFIED BY**: team-engagement `internal/events` `TestRelayPublishesPerKeyInOrder`

#### Scenario: Engagement SDK: Broker outage delays but does not lose a fact
- **WHEN** the broker is stopped, a buyer follows a seller, and the broker is started again
- **THEN** one `SellerFollowed` envelope for that buyer and seller appears on `engagement.events` and the row is not parked
- **VERIFIED BY**: team-engagement `internal/events` `TestRelayTransientErrorsNeverPark`

#### Scenario: Engagement SDK: Retention keeps pending rows
- **WHEN** the retention job runs over a published row aged 8 days, a published row aged 6 days and a pending row aged 30 days
- **THEN** only the 8-day published row is deleted
- **VERIFIED BY**: team-engagement `internal/events` `TestOutboxRetentionPurgesPublishedOnly`

#### Scenario: Engagement SDK: Two relayers publish a row once
- **WHEN** two relayers run over 50 pending rows
- **THEN** each of the 50 event ids appears exactly once on the topic
- **VERIFIED BY**: team-engagement `internal/events` `TestTwoRelayersPublishOnce`

### Requirement: Migration 0014 moves unpublished facts to the SDK outbox

Migration `0014` SHALL create `engagement_outbox_events` with the SDK schema and SHALL copy every row of the legacy `outbox` table whose `published_at` is NULL: event id kept, aggregate type `Seller` for `SellerFollowed` and `SellerUnfollowed` and `Listing` otherwise except `DisputeResolved` which is `Dispute`, aggregate id the legacy key, payload `BuildEnvelope` of the legacy row (event id, payload type and bytes, principal id and type, request id, `occurred_at`; the legacy `principal_type` holds the enum name and maps `PRINCIPAL_TYPE_ANONYMOUS`, `PRINCIPAL_TYPE_USER` and `PRINCIPAL_TYPE_SERVICE` to the same enum values; a row with empty `principal_id` and empty `principal_type` gets no principal), and a claim order equal to the legacy `seq` order. Published legacy rows SHALL NOT be copied, and every copied row SHALL be `pending` with 0 attempts (the legacy relayer had no parked state, so nothing is left behind). After the copy, `0014` SHALL requeue every row of `engagement_outbox_events` that is parked as `failed` (status `pending`, attempts 0, no lease, error cleared), so an upgrade never leaves a parked row that would block its aggregate under the SDK relayer; on a table created by `0014` this changes no row. Running the copy again SHALL change nothing. The legacy table SHALL stay untouched. The down migration SHALL drop only `engagement_outbox_events`; because facts written after cut-over live only there, a rollback of the image after `0014` is not supported (roll forward; see the rollback rule in the design).

#### Scenario: Engagement SDK: Unpublished legacy facts are copied in order
- **WHEN** the legacy outbox holds 3 unpublished rows with seq 5, 6, 7 and 2 published rows, and `0014` is applied
- **THEN** `engagement_outbox_events` holds 3 pending rows with the same event ids, claimed in seq order 5, 6, 7, each status `pending` with 0 attempts and each payload equal to `BuildEnvelope` of its legacy row (principal id and type, request id, `occurred_at`) and carrying the legacy principal type mapped to the enum (a row with no principal gets none), and the legacy table still holds all 5 rows
- **VERIFIED BY**: team-engagement `migrations` `TestMigration0014CopiesUnpublishedFacts`

#### Scenario: Engagement SDK: Parked rows are requeued by the migration
- **WHEN** `engagement_outbox_events` already exists with one row parked as `failed` (attempts 5, an error text) and one `published` row, and `0014` is applied
- **THEN** the parked row is `pending` with attempts 0, no error and no lease, and the published row is unchanged
- **VERIFIED BY**: team-engagement `migrations` `TestMigration0014RequeuesParkedRows`

#### Scenario: Engagement SDK: Copy is idempotent
- **WHEN** the copy statement of `0014` runs a second time
- **THEN** the row count and the event ids of `engagement_outbox_events` are unchanged
- **VERIFIED BY**: team-engagement `migrations` `TestMigration0014CopyIsIdempotent`

#### Scenario: Engagement SDK: Empty legacy outbox
- **WHEN** `0014` is applied on a database whose legacy outbox is empty
- **THEN** it succeeds and `engagement_outbox_events` is empty
- **VERIFIED BY**: team-engagement `migrations` `TestMigration0014EmptyLegacyOutbox`

#### Scenario: Engagement SDK: Down migration keeps the legacy table
- **WHEN** `0014` is applied and then reverted
- **THEN** `engagement_outbox_events` no longer exists and the legacy `outbox` table and its rows remain
- **VERIFIED BY**: team-engagement `migrations` `TestMigration0014DownKeepsLegacyOutbox`

#### Scenario: Engagement SDK: Service role keeps its grants after 0014
- **WHEN** `0014` is applied by the migration owner role
- **THEN** the service role can insert into and select from `engagement_outbox_events` and the legacy table keeps the grants it had
- **VERIFIED BY**: team-engagement `migrations` `TestMigration0014ServiceRoleKeepsGrants`

### Requirement: The follow feed lists published listings of followed sellers

`seller_listings` SHALL be kept in step with `KAFKA_LISTING_TOPIC` by the `ListingEventHandler`, and `ListFollowedListings` SHALL return from it, for the caller only, the listings of the sellers the caller follows. A `ListingChanged` event whose listing is `PUBLISHED` (created or updated) SHALL upsert the row `(seller_id, listing_id)` with the event's `occurred_at` as `created_at`; the first `created_at` and a single owner per listing SHALL be kept on redelivery and on later updates. A `ListingChanged` with change type `DELETED`, or with any status other than `PUBLISHED`, and a `ListingStatusChanged` to a status other than `PUBLISHED` SHALL remove the row; removing an unknown listing SHALL be a no-op. Any other event type SHALL be ignored with a nil result. An event with no listing id, or a published listing with no seller id, SHALL make the handler return an error (the consumer retries it and then parks it). The feed SHALL be ordered newest first (`created_at` descending, listing id descending on ties) with keyset paging that a new head row does not shift, the caller never sees a seller the caller does not follow, and a caller who follows nobody gets an empty feed with total 0. The behaviour and its tests exist on the branch and are moved, not rewritten, to the new packages.

#### Scenario: Engagement SDK: Published listing appears in the follower's feed
- **WHEN** buyer `buyer` follows seller `seller`, a `ListingChanged` for published listing `l1` of `seller` is consumed, and then one for published listing `lx` of a seller `buyer` does not follow
- **THEN** the feed of `buyer` is `[l1]` after the first event and still `[l1]` after the second
- **VERIFIED BY**: team-engagement `internal/events` `TestPublishedListingAppearsInFeed`

#### Scenario: Engagement SDK: Draft listing is not in the feed
- **WHEN** a `ListingChanged` with status DRAFT for a listing of a followed seller is consumed
- **THEN** the feed of the follower is empty
- **VERIFIED BY**: team-engagement `internal/events` `TestDraftListingIsNotInFeed`

#### Scenario: Engagement SDK: An update keeps the creation order
- **WHEN** `l1` is published at T, `l2` at T+1h, and then an UPDATED `ListingChanged` for `l1` arrives at T+2h
- **THEN** the feed is `[l2, l1]`
- **VERIFIED BY**: team-engagement `internal/events` `TestUpdateKeepsCreatedAtOrdering`

#### Scenario: Engagement SDK: Unpublish and delete remove the listing from the feed
- **WHEN** `l1`, `l2`, `l3` are published, `l1` becomes DRAFT through `ListingChanged`, `l2` becomes REJECTED through `ListingStatusChanged`, then `l3` arrives as `DELETED` with a payload that still says published, and a delete of a never-seen `ghost` is consumed
- **THEN** the feed is `[l3]` after the first two, then empty, and the `ghost` delete returns nil
- **VERIFIED BY**: team-engagement `internal/events` `TestUnpublishAndDeleteRemoveFromFeed`

#### Scenario: Engagement SDK: Redelivery is idempotent
- **WHEN** the same published `ListingChanged` is consumed 3 times, and the same `DELETED` event twice
- **THEN** after the 3 deliveries the feed has one row and total 1, and after the deletes it is empty
- **VERIFIED BY**: team-engagement `internal/events` `TestRedeliveryIsIdempotent`

#### Scenario: Engagement SDK: Other event types are ignored and invalid ones fail
- **WHEN** a `ListingPricingChanged` envelope, bytes that are not an envelope, a `ListingChanged` with an empty listing id, and a published `ListingChanged` with an empty seller id are consumed
- **THEN** the first returns nil and leaves `seller_listings` unchanged, and each of the other three returns an error; the undecodable bytes are wrapped as `consume.Permanent`
- **VERIFIED BY**: team-engagement `internal/events` `TestIgnoredAndMalformedEvents`

#### Scenario: Engagement SDK: The feed lists followed sellers' listings newest first
- **WHEN** a user follows `seller_1` and `seller_2`, three of their listings and one of an unfollowed seller are indexed, and a second user who follows nobody lists the feed
- **THEN** the first user gets the three listings newest first with total 3, never the fourth, and the second user gets an empty list with total 0
- **VERIFIED BY**: team-engagement `internal/store` `TestFollows_FollowedListingsFeed`

#### Scenario: Engagement SDK: Feed order is stable across pages
- **WHEN** listings with equal `created_at` are indexed in an order that differs from both time and id order, the feed is read two rows at a time, and a newer listing is indexed between the pages
- **THEN** the pages follow `created_at` descending then listing id descending, no row is repeated or skipped, and a cursor `garbage` returns `ErrInvalidCursor`
- **VERIFIED BY**: team-engagement `internal/store` `TestFollows_FeedNewestFirstAndPaging`

#### Scenario: Engagement SDK: An upsert keeps the first creation time and a single owner
- **WHEN** a listing is upserted at T, again at T+1h, and then upserted for a second seller
- **THEN** its `created_at` stays T and the listing belongs to the second seller only
- **VERIFIED BY**: team-engagement `internal/store` `TestFollows_UpsertKeepsCreatedAtAndSingleOwner`

#### Scenario: Engagement SDK: The feed is self-scoped and needs the read scope
- **WHEN** user `u2`, who follows nobody, calls `ListFollowedListings` with a request that names `u1`'s page, and a user holding only `engagement:write` calls it
- **THEN** `u2` gets an empty list (the feed is read for the principal, never for a request-supplied id) and the write-only caller gets `PermissionDenied`
- **VERIFIED BY**: team-engagement `internal/rpc` `TestListFollowedListingsIsSelfScoped`

#### Scenario: Engagement SDK: A malformed feed cursor is InvalidArgument
- **WHEN** `ListFollowedListings` is called with cursor `garbage`
- **THEN** the call returns `InvalidArgument` with message `invalid cursor`
- **VERIFIED BY**: team-engagement `internal/rpc` `TestListFollowedListingsMalformedCursor`

### Requirement: The listing.events consumer runs on the SDK consumer

The service SHALL consume `KAFKA_LISTING_TOPIC` with group `KAFKA_CONSUMER_GROUP`, a fresh group starting at the earliest offset, through `consume.NewConsumer` with DLQ topic `<topic>.dlq`, handing each record to the `ListingEventHandler` of the requirement above. A record whose bytes cannot be decoded SHALL go to the DLQ after one attempt; any other handler failure SHALL be retried up to 5 attempts and then go to the DLQ, after which the offset advances. When a record can be neither handled nor parked, the consumer SHALL return an error, the offset SHALL NOT advance and the process SHALL exit non-zero.

#### Scenario: Engagement SDK: Undecodable record is dead-lettered after one attempt
- **WHEN** a record with bytes that are not an `EventEnvelope` is consumed
- **THEN** it is produced to `listing.events.dlq` with its original key and value after exactly 1 handler attempt and the offset advances (a record that decodes but fails the handler, for example one with an empty listing id, takes the 5-attempt path of the next scenario)
- **VERIFIED BY**: team-engagement `internal/events` `TestUndecodableRecordGoesStraightToDLQ`

#### Scenario: Engagement SDK: Failing record is parked after five attempts
- **WHEN** the store fails every call for a valid record
- **THEN** the handler is called 5 times, the record is produced to `listing.events.dlq`, and the offset advances
- **VERIFIED BY**: team-engagement `internal/events` `TestFailingRecordParkedAfterFiveAttempts`

#### Scenario: Engagement SDK: Unparkable record stops the service
- **WHEN** the handler fails 5 times and the DLQ produce also fails
- **THEN** the consumer returns an error, the offset does not advance, and `app.Run` returns that error
- **VERIFIED BY**: team-engagement `internal/app` `TestConsumerFailureStopsService`

#### Scenario: Engagement SDK: A new group backfills from the start
- **WHEN** the topic holds 3 published-listing events and a service with a new consumer group starts
- **THEN** `seller_listings` holds the 3 rows
- **VERIFIED BY**: team-engagement `internal/events` `TestNewGroupBackfillsFromEarliest`

### Requirement: Closed reports are swept on a schedule by the lifecycle

The report retention sweep SHALL be a `lifecycle.Every` worker with interval `REPORT_SWEEP_INTERVAL` that runs once at start and then on every tick, with cutoff now minus `REPORT_RETENTION_DAYS`, in batches of 500, and stops when the service shuts down. A failing run SHALL be logged and SHALL NOT stop the worker. Batches committed before a failure SHALL stay committed.

#### Scenario: Engagement SDK: Sweep runs at start and on each tick
- **WHEN** the worker starts with a fake clock and two ticks arrive
- **THEN** the sweep is called 3 times, each with cutoff now minus 730 days, and the worker returns nil when its context ends
- **VERIFIED BY**: team-engagement `internal/app` `TestSweepRunsAtStartAndEveryInterval`

#### Scenario: Engagement SDK: A failing sweep does not stop the worker
- **WHEN** the first sweep run returns an error and a tick follows
- **THEN** the sweep is called again on the tick
- **VERIFIED BY**: team-engagement `internal/app` `TestSweepFailureDoesNotStopWorker`

#### Scenario: Engagement SDK: Batch boundary
- **WHEN** 1200 closed reports older than the cutoff exist and a failure happens in batch 2
- **THEN** 500 reports stay redacted, and the next run redacts the remaining 700
- **VERIFIED BY**: team-engagement `internal/engagement` `TestSweepReportsFailureKeepsEarlierBatchAndResumes`

### Requirement: Paging keeps the cursors and limits clients already hold

List RPCs SHALL clamp page sizes with `pagination.Clamp` to the current defaults and maxima (favourites, follows, followed-listings feed, recently viewed and collection items: default 50, maximum 100; reviews, disputes and reports: default 20, maximum 100). `ListQuestionsByListing` SHALL clamp the same way with default 20 and maximum 100: a page size above 100 now returns 100 items, where it returned 20 before (accepted: the SDK clamp replaces the reset to the default); its `page` handling and total are unchanged. The cursor encodings SHALL stay as before: the feed cursor, the dispute cursor `base64url("<unix-micro>|<id>")`, the report cursor and the review offset cursor. Malformed cursors SHALL behave as the error table requirement lists them, RPC by RPC.

#### Scenario: Engagement SDK: A cursor from before the migration still works
- **WHEN** `ListDisputes` is called with a cursor produced by `base64url("1760000000000000|D7")`
- **THEN** the page starts after that dispute and the call returns OK
- **VERIFIED BY**: team-engagement `internal/store` `TestDisputeCursorFormatUnchanged`

#### Scenario: Engagement SDK: Page size above the maximum is clamped
- **WHEN** `ListFavorites` is called with page size 101 over 150 favourites, then with page size 100
- **THEN** the first call returns at most 100 items and a next cursor, and the second returns 100 items
- **VERIFIED BY**: team-engagement `internal/store` `TestPageSizeClampBoundary`

#### Scenario: Engagement SDK: Default page sizes per list
- **WHEN** `ListFavorites`, `ListFollowedSellers`, `GetRecentlyViewed`, `ListReviews`, `ListDisputes` and `ListReports` are called with page size 0 over 150 rows, and `ListDisputes` and `ListReports` with page size 101
- **THEN** the first three return 50 items, `ListReviews`, `ListDisputes` and `ListReports` return 20 items, and `ListDisputes` and `ListReports` with page size 101 return 100 items
- **VERIFIED BY**: team-engagement `internal/store` `TestDefaultPageSizesPerList`

#### Scenario: Engagement SDK: Question page size is capped at 100
- **WHEN** `ListQuestionsByListing` is called over 150 questions with page size 0, 100 and 101
- **THEN** it returns 20, 100 and 100 items, and the total is 150 each time
- **VERIFIED BY**: team-engagement `internal/rpc` `TestListQuestionsPageSizeClamped`

#### Scenario: Engagement SDK: Malformed review cursor is rejected
- **WHEN** `ListReviews` is called with cursor `abc`
- **THEN** the call returns `InvalidArgument` with message `invalid page cursor`
- **VERIFIED BY**: team-engagement `internal/rpc` `TestListReviewsRejectsMalformedCursor`

#### Scenario: Engagement SDK: Malformed report cursor is rejected
- **WHEN** an admin calls `ListReports` with cursor `abc`, and then with cursor `-1`
- **THEN** both calls return `InvalidArgument` with message `invalid cursor`
- **VERIFIED BY**: team-engagement `internal/rpc` `TestListReportsMalformedCursor`

#### Scenario: Engagement SDK: Malformed dispute cursor stays Internal
- **WHEN** an admin calls `ListDisputes` with cursor `%%%`
- **THEN** the call returns `Internal` with message `internal error` (today's answer, kept)
- **VERIFIED BY**: team-engagement `internal/rpc` `TestListDisputesMalformedCursorStaysInternal`
