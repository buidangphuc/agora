## Why

SDK v1.0.0 cannot be consumed and main's services hand-roll what it lacks.

- The module path `github.com/buidangphuc/platform-core/packages/go-sdk` is not fetchable (no such repo), so the spec forces every service to vendor a copy into `third_party/go-sdk` with `sync-go-sdk.sh` and a `replace`.
- Since 2026-10-13 main's services grew helpers the SDK does not have: an audit-first fail-closed client (domain, engagement, identity, order, payment, promotion, verification), Kafka key fingerprint logging (domain, engagement, identity), a strict-ENV guard for mock data that ran after DB validation (payment, fixed in #48), and a unique-violation mapping (promotion returns `Internal` for a duplicate `CreateVoucher`).
- The owner decided (2026-10-22) that a service which imports the database or Kafka always uses it, so the SDK must say it has no enable flags.

Source of the requirements: `plans/go-sdk-migration/PLAN.md`, Step 0 (gaps G1 to G6), "Kept flags" and "Shared lint/make files".

initiative: adhoc

## What Changes

- **G1** Module path becomes `github.com/buidangphuc/agora/platform-core/packages/go-sdk` (`go.mod` and every import). Releases are git tags `platform-core/packages/go-sdk/vX.Y.Z`. Services `require` a tag: no `replace`, no `third_party`. **BREAKING** (tooling): `platform-core/tools/sync-go-sdk.sh`, its test, `check-go-sdk-vendor.sh` and its test are removed; requirement "Services vendor a pinned SDK copy" is REMOVED. Services reference the shared `platform-core/make/go-service.mk` and `platform-core/packages/go-sdk/.golangci.yml` by relative path (no copies); a service lint exclusion becomes a path-scoped rule in the shared file; goimports `local-prefixes` becomes `github.com/buidangphuc`.
- **G2** The SDK keeps no dependency enable flags. Stated as a requirement. Service-side flag removal is NOT in this change (later per-service changes).
- **G3** New `pkg/mockdata` and group `config.MockData` (`MOCK_DATA`): a service declares its mockable adapters; `Load` refuses a non-empty `MOCK_DATA` when `Runtime.IsStrict()`, before any group validation; unknown names are refused; active adapters are logged once at boot.
- **G4** New `pkg/auditx`: `config.Audit` (`UPSTREAM_AUDIT_ADDR`, required when strict), `Dial` with a service principal, and a fail-closed `Gate` that refuses the privileged action with `Unavailable` "audit trail unavailable" when the audit write fails.
- **G5** `consume` and `kafkax` log a keyed fingerprint (`key_fp`, HMAC-SHA256 under `LOG_KEY_FP_SECRET`, group `config.LogKeyFP`) instead of the raw key; raw keys are scrubbed from logged and DLQ error text.
- **G6** `grpcx.Status` maps a Postgres unique violation (SQLSTATE 23505) to `AlreadyExists`.
- Version: README and CHANGELOG say v1.0.1. The tag is cut by the user after merge.

## Non-goals

- Removing `DATABASE_ENABLED`, `KAFKA_ENABLED`, `OUTBOX_ENABLED`, `FEATURE_FLAGS_ENABLED` and the other service flags (per-service changes, Step 1 and 2 of the plan).
- Kept flags, not touched: `OTEL_ENABLED`, `GRPC_REFLECTION_ENABLED`, `SERVER_REFLECTION_ENABLED`, `EDGE_REFLECTION_ENABLED`, `SEED_ADMIN_ENABLED`.
- Migrating any service to the SDK, and moving the mock switches of Step 3 (route fulfils, `tei-fake`).
- Adding generated protobuf code to the SDK (see design.md D2).

## Impact

`platform-core/packages/go-sdk` (go.mod, pkg/config, pkg/mockdata, pkg/auditx, pkg/consume, pkg/kafkax, pkg/grpcx, README, CHANGELOG, `.golangci.yml`), `platform-core/make/go-service.mk`, `platform-core/templates/_service-go`, `platform-core/tools` (two scripts and their tests deleted), `platform-core/FEATURES.yaml`, `platform-core/docs/ADR` (new ADR superseding the distribution part of ADR-0016), `.github/workflows/asdlc-gates.yml` and `scripts/asdlc/ci_local.sh` (drop the vendor check).

## Non-functional requirements

- Latency: the fingerprint costs one HMAC per logged record (under 10 microseconds for a 200-byte key); the audit gate adds one unary call bounded by the earlier of the caller deadline and `upstream.DefaultTimeout` (2s) before the privileged action; `Load` adds no I/O.
- Availability: the audit gate is fail-closed by design, so an audit outage makes privileged actions unavailable; read paths and non-audited actions are unaffected.
- Limits: `MOCK_DATA` holds at most 16 adapter names; a fingerprint is exactly 12 lowercase hex characters.
- Compatibility: all changes except G1 are additive; no new third-party dependency (`pgconn` already ships with pgx, `crypto/hmac` is stdlib).
- Release: consumers need the tag `platform-core/packages/go-sdk/v1.0.1` to exist before any service can adopt it.

## Data classification

| Field | Class | Purpose | Consent / legal basis | Retention |
|---|---|---|---|---|
| Kafka record key (a user id) | personal data | read only to derive `key_fp`; no longer logged | n/a (not stored by this change) | not stored |
| `key_fp` (12 hex of HMAC-SHA256 of the key) | pseudonymous personal data | correlate log lines of one record without exposing the id | legitimate interest (security logging), same basis as archived `domain-consumer-key-redaction` | platform log retention; no new store |
| `LOG_KEY_FP_SECRET` | secret | HMAC key; makes `key_fp` unguessable from a known user-id list | n/a | Vault in staging and prod; rotation changes all fingerprints |
| Audit event `actor_id`, `target_id`, metadata | personal data | passed through `auditx` to team-audit as today | existing basis of the audited admin action | owned by team-audit; unchanged |
| `MOCK_DATA`, `UPSTREAM_AUDIT_ADDR` | internal config | boot configuration | n/a | n/a |

Consumer error text may embed user ids (for example `fmt.Errorf("user %s: %w", id, err)`); the SDK scrubs the record key from logged and DLQ-header error text but cannot know other ids a handler puts in its error. Services must not format ids into errors (stated in the README).

## Threat model

| Asset | Abuse case (STRIDE) | Mitigation |
|---|---|---|
| Real payment, DB and events | Tampering: a mock adapter reaches a shared environment | `MOCK_DATA` refused by `Load` in strict ENV before any other validation; unknown adapter names refused; active adapters logged at boot |
| Audit trail integrity | Repudiation: a privileged action runs although its audit write failed | `auditx.Gate` writes first and does not run the action on any failure, a nil writer included |
| Privileged action availability | Denial of service via audit outage | accepted: fail-closed is the requirement; 2s bound per write |
| Audit write authority | Spoofing: any caller writes audit events | `Dial` attaches a service principal (`upstream.AsService`); team-audit already enforces the authz rule: `WriteAuditEvent` calls `interceptor.RequireService(ctx, ScopeAuditWrite)` (`team-audit/internal/handler/audit.go:49`); only a service principal holding the audit write scope may write |
| User identity in logs | Information disclosure: raw Kafka key or user id in logs and DLQ headers | keyed HMAC fingerprint; secret required when strict; key scrubbed from error text |
| Log fingerprints | Information disclosure: a known user-id list is hashed to re-identify `key_fp` | HMAC under a per-environment secret, not a bare SHA-256 |
| Database error text | Information disclosure: constraint name or row detail reaches the client | 23505 maps to a fixed `already exists` message; detail is logged only |
| Supply chain | Tampering: a service builds against an unreviewed copy | tagged module fetched through go.sum; no vendored copy to edit |
