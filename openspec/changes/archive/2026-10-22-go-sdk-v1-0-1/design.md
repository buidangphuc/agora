## Context

Behaviour comes from main's hand-rolled versions: audit-first calls (`team-promotion/internal/service/audit.go`, `team-order/internal/handler/order.go`), `key_fp` consumer logging (archived changes `domain-consumer-key-redaction`, `engagement-consumer-key-redaction`), and the `MOCK_PAYMENTS` strict guard (`team-payment/internal/config/config.go`, `RequireNoMockPayments`, which must run before DB validation). The SDK `config.Load` already runs `ResolveFor(Runtime)` on groups before `Validate`, and `Runtime.IsStrict()` exists.

## D1. Delta shape

The baseline already holds the archived `add-go-sdk-messaging`, `go-sdk-v0-2-1`, `go-sdk-v0-2-2` and `go-sdk-v1`. This change ADDS one requirement per gap (G1 to G6), MODIFIES the service-template requirement (it mentions sync) and REMOVES every baseline requirement about vendoring or `sync-go-sdk.sh`: "Services vendor a pinned SDK copy", "Sync prepares a buildable, correctly linted service", "Sync keeps service-local lint overrides", "Vendored SDK copies match their tag", "Sync keeps the go-sdk requirement". No ADDED header duplicates a baseline header.

## D2. auditx does not import generated protobuf

The SDK has no generated protobuf code: `platform-core/packages/proto` is its own Go module with `buf.gen.yaml` and `.proto` files, and the SDK has no `platform/audit/v1` Go package to import. Making the SDK depend on the proto module would couple every SDK release to proto releases (the v1 change rejected the same for an EventEnvelope helper). Decision: `auditx` defines `Event` and the `Writer` interface, plus `Dial` (a service-principal `*grpc.ClientConn` built on `upstream.NewClient` and `upstream.AsService`) and the fail-closed `Gate`. The service supplies a ten-line `Writer` adapter around the generated `auditv1.NewAuditServiceClient(conn).WriteAuditEvent`. Alternative rejected for now: a dependency on the proto module so `auditx` owns the call; revisit when the SDK publishes its own generated audit client.

The request fields map one to one: `Event.ActorID/Action/TargetType/TargetID/Metadata` to `WriteAuditEventRequest.actor_id/action/target_type/target_id/metadata`.

## D3. Fail-closed message and logging

The gate returns `Unavailable` with the fixed text `audit trail unavailable` (exported as `auditx.Message`). team-order currently appends `; action not performed`; team-promotion uses the short text. The short text is chosen because it is the message the plan names; per-service migrations must align their tests. A nil writer is treated as an outage (promotion returns "audit trail not configured" today; the SDK does not distinguish). The gate logs the failure at error level with only the action, the target type and the writer error's gRPC status code; it never logs the target id or the error text, and never returns the cause.

## D4. MockData in config, not only in mockdata

The refusal must run before group validation, and only `config.Load` controls that order, so the strict check is `config.MockData.ResolveFor(Runtime)` (resolve runs before validate). The adapter name check needs the service's declared names, which `Load` cannot know, so it lives in `mockdata.New`, which the service calls right after `Load`. Mock adapters are an outer-adapter swap: `mockdata.Select(set, name, real, fixture)`. `LogActive` uses a `sync.Once` per `Set`. A parse error of another variable (for example a missing `GRPC_PORT`) is still reported first by `env.Parse`, as for every group; the guarantee is that `MOCK_DATA` is not hidden behind `Validate` errors, which was the payment bug.

## D5. Key fingerprint construction

HMAC-SHA256 keyed by `LOG_KEY_FP_SECRET`, truncated to 12 lowercase hex characters (48 bits: enough to correlate log lines of one record, too short to be a lookup key). The archived services used an unkeyed SHA-256 prefix; with an unkeyed hash a list of known user ids re-identifies every `key_fp`, which is why the plan requires a per-environment secret. Rotating the secret changes every fingerprint; correlation across a rotation is lost and accepted.

Local default: `config.LogKeyFP` fills a fixed documented value for non-strict ENV so local logs are stable. When `consume.Options.KeyFP` is nil the consumer uses a random per-process secret: never unkeyed, never configured by accident. Both consumers (single and batch) share one `redact` helper.

Error scrubbing: handler error text is replaced by a string replace of the raw key text with `[key_fp:<fp>]`. It cannot catch other user ids a handler formats into its error; the README tells services not to. The `dlq-error` header is scrubbed too, because it is persisted and read by operators; the DLQ record key and value are unchanged so replays still work.

`pkg/outbox` logs `event_id`, not keys, and is not changed.

## D6. Where the 23505 mapping lives

`grpcx.Status` is the single place where driver errors turn into client errors ("Unknown errors do not leak"). It gets a built-in step after the table lookup and before `Internal`: `errors.As(err, &*pgconn.PgError)` with code `23505` returns `AlreadyExists` with the fixed text `already exists`. Table rules come first, so a service that maps its own sentinel (for example a `ErrDuplicateCode` with its own message) keeps working. `pgconn` is already in the module through pgx; `grpcx` gains that import only. Alternatives rejected: a `postgres` sentinel needing every repository to wrap errors; a `postgres.IsUniqueViolation` helper alone (services would still each map it).

## D7. Release mechanics

`go.mod` module path changes; the tag `platform-core/packages/go-sdk/v1.0.1` follows Go's rule for a module in a subdirectory (`<subdir>/vX.Y.Z`). v1.0.0 was never tagged under this path, so no major-version suffix is needed. The tag is created by the user after merge; nothing here pushes a tag. The agora repository is public, so services and their Docker builds fetch the module through the default Go proxy with no credentials and no `GOPRIVATE`. The only delay is tag to proxy availability; human task 3.2 verifies a clean Docker build after tagging.

## D8. Shared lint and make files

golangci-lint has no config inheritance, so there is one file with path-scoped rules. `go-service.mk` and `.golangci.yml` stay where they are; the template `Makefile` includes them by relative path. They run at dev and CI time only, so the per-service Docker build context is unaffected.

## Clarifications

### Round 1 — 2026-10-22
- Archive order: `add-go-sdk-messaging`, `go-sdk-v0-2-1`, `go-sdk-v0-2-2` and `go-sdk-v1` are archived first (PR #50); this change then REMOVES their vendoring requirements. (human)
- D2 is final: `auditx.Writer` is an interface the service implements over its generated audit client; the SDK takes no proto dependency. (human)
- D3 is final: the fail-closed message is `audit trail unavailable` everywhere. (human)
- `key_fp` needs no DPO sign-off: basis is legitimate interest (security logging), retention is the platform log retention. (human)
