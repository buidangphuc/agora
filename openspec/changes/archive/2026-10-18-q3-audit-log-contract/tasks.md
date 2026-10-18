Paths relative to the repo root. E2E track: none (pure contract; behaviour is e2e-tested in audit-query-filters and the producer slices). FEATURES.yaml entries for platform-core are `not-testable` with reasons, as in q2-review-integrity-contract.

## 1. Code track — proto
Write-set: `platform-core/packages/proto/platform/audit/v1/audit.proto`
- [x] 1.1 Add `target_id = 4`, `action = 5` (both `optional string`), `created_after = 6`, `created_before = 7` (Timestamp) to `QueryAuditLogRequest` (covers: New filter fields exist with fixed numbers, Wire-breaking edit is rejected, Lint passes).
- [x] 1.2 Write comments: filter semantics on the RPC and the four fields; `<entity>.<verb>` rule, the uniform admin-path wording (admin-scope callers audited even when they own the resource) and the 13-action catalogue on `WriteAuditEventRequest` (covers: Filter semantics are documented, Naming rule and catalogue are present).
- [x] 1.3 Leave `actor_id`, `target_type`, `page` and all write/event messages untouched (covers: Write wire is unchanged, Old client request still parses).
- [x] 1.4 Run `make -C platform-core lint-proto breaking` (covers: Change is backward compatible, Lint passes).

## 2. Generation, verification and copies
Write-set: `platform-core/packages/proto/platform/audit/v1/auditquery_roundtrip_test.go`, `{team-audit,team-gateway,team-analytics,team-engagement,team-frontend,team-verification,team-domain,team-order,team-payment,team-promotion}/proto/platform/audit/v1/audit.proto`
- [x] 2.1 Write the Go test first (red), mirroring `hidereview_roundtrip_test.go` (`buf build` + dynamic descriptors): field numbers/types of `QueryAuditLogRequest`, boundary round-trips, old-bytes parse, `WriteAuditEventRequest`/`AuditEvent` field numbers, comment assertions (filter semantics, `<entity>.<verb>`, 13 actions); then go green via 1.x (covers: New filter fields exist with fixed numbers, Round-trip of equal time bounds, Round-trip of empty action and long target id, Untouched request leaves new fields unset, Old client request still parses, Filter semantics are documented, Naming rule and catalogue are present, Action missing from the catalogue is caught, Uniform admin-path wording and non-admin exclusion, Non-admin access is documented as denied, Write wire is unchanged).
- [x] 2.2 Run `buf generate`; confirm Go, Python, JVM and TS outputs expose the four fields (covers: Filters in every generated language).
- [x] 2.3 Vendor `audit.proto` byte-identical into the ten trees: update the four existing copies (team-audit, team-gateway, team-analytics, team-engagement) and create `proto/platform/audit/v1/audit.proto` in team-verification, team-domain, team-order, team-payment, team-promotion and team-frontend (producer slices only regenerate; each already has `platform/common/v1/common.proto`). Run `cmp` against the source and `buf lint` in each proto dir that has a `buf.yaml` (team-payment has none: `cmp` only) (covers: Copies match the source, Drifted or missing copy is rejected, Consumer copies lint where a buf module exists).

## 3. Workspace wiring
Write-set: `docker-compose.services.yaml`, `AGENTS.md`, `.claude/agents/auth-scope-reviewer.md`
- [x] 3.1 Add `UPSTREAM_AUDIT_ADDR=team-audit-svc:50066` to team-verification, team-domain, team-order, team-payment, team-promotion; check the rendered root `docker-compose.services.yaml` only, and `docker compose config -q` (covers: Six producers have the audit address, Wrong or missing address is detected, Compose file stays valid).
- [x] 3.2 Add the "admin mutations audit first, fail closed" rule to AGENTS.md section 3, worded uniformly: every call by a caller holding `admin` (team-order: `order.admin`) is audited, including an admin who owns the resource (covers: AGENTS.md carries the rule).
- [x] 3.3 Add the BLOCKING check, fail-closed wording, exemption for owners lacking the admin scope and CLEAN-output requirement to `auth-scope-reviewer.md` (covers: Reviewer flags an unaudited admin mutation, Owner path is not flagged).

## 4. E2E track
- [x] 4.1 None: no user-facing behaviour. Add `platform-core/FEATURES.yaml` entries (`status: not-testable`, reason: pure contract, verified by the unit/cmp/grep checks above) for every scenario (done by the e2e-author).
