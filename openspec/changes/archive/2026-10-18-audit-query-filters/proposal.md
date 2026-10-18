## Why

`AuditService.QueryAuditLog` filters only by `actor_id` and `target_type`, so an admin cannot answer "who touched this order, and when". The initiative spec (`plans/q3-audit-log/00-spec.md`, locked 2026-10-15) adds `target_id`, `action` and a time range; `q3-audit-log-contract` owns the proto, this slice implements the filters in team-audit, makes the gateway pass them through, and ships the shared e2e helper `platform-e2e/tests/e2e/support/audit_support.py` that the six producer slices and `frontend-audit-viewer` import to assert audit events.

initiative: q3-audit-log

## What Changes

- **team-audit** (`internal/handler`, `internal/service`, `internal/repository`)
  - Handler `QueryAuditLog` reads `target_id`, `action`, `created_after`, `created_before` from the regenerated request and passes them to the service. `RequireAdmin` stays the first statement; validation runs after it.
  - Service `Query` takes a filter struct instead of positional args. If both timestamps are set and `created_before` < `created_after` it returns `ErrInvalidTimeRange`, which the handler maps to `InvalidArgument` (fixed message "created_before must not be earlier than created_after"). Equal timestamps are valid and match nothing (empty half-open range).
  - Repository `QueryFilter` gains `TargetID`, `Action`, `CreatedAfter`, `CreatedBefore` (zero value = no bound). Postgres and in-memory implementations apply all filters with AND: `created_at >= after`, `created_at < before`, exact-match strings. Empty string = not filtered (unchanged semantics for `actor_id` and `target_type`). `total` counts matches before paging; ordering (`created_at DESC, id DESC`) and the offset cursor are unchanged.
  - **Index decision:** add migration `002_audit_action_created_index` (up and down) creating `idx_audit_events_action_created ON audit_events (action, created_at DESC)`. Reason: the viewer's "filter by action" with no target or actor would otherwise scan the whole table (the existing indexes lead with `actor_id` and `target_type`), which breaks the p95 < 300 ms goal at 100k rows. `target_id` filters keep using `idx_audit_events_target_created` (a `target_id` filter without `target_type` is not index-served; accepted, see Open items). Time-only queries (no other filter) sort on `created_at`; accepted for the first release.
  - README: Contract table (new filters, error), Data (new index), Spec and verification (FEATURES.yaml exists, covered by the e2e feature), and Known gaps (remove the stale `target_id`/time-range bullet and the "No FEATURES.yaml" bullet).
- **team-gateway**: regenerate `generated/` from the vendored proto copy (copy refreshed by the contract slice). No policy change: `QueryAuditLog` stays in `adminProcedures`, `WriteAuditEvent` stays unrouted. The forwarder already passes `req.Msg` through, so no handler change.
- **platform-e2e**
  - New feature `tests/e2e/features/security/audit_query_filters.feature` with bindings `tests/e2e/step_definitions/test_audit_query_filters.py` (admin through the gateway).
  - New shared helper `tests/e2e/support/audit_support.py` (signatures in design.md), written by this slice only. Producer slices and `frontend-audit-viewer` import it and never edit it.
- Not changed: proto (contract slice), `WriteAuditEvent`, the `audit_events` columns, other services, the frontend.

## Impact

- Affected specs: new capability `audit-log-query` (this change). The existing `audit-access-control` capability is untouched.
- Backward compatible: callers that set no new field get the same result as today.
- Deploy: the new index is created by the compose one-shot `team-audit-migrate`; `CREATE INDEX` on a small table is instantaneous. For a large table, the production runbook should use `CREATE INDEX CONCURRENTLY` (human item, no prod data yet).

## NFR

- Latency: filtered `QueryAuditLog` p95 < 300 ms on 100k rows with any single filter or any combination (non-gating; checked once with `EXPLAIN` in task 6, not in CI).
- Availability: unchanged (stateless handler; reads only).
- Limits: page size default 20, max 100 (unchanged); filter strings are exact-match and bounded by column widths (`action` 128, `target_*` 64); no wildcard or substring search, so no user-controlled regex or `LIKE` cost.
- Safety: all filter values are bound parameters (`$n`), never concatenated into SQL.

## Data classification

No new personal data is stored. The new request fields are filter inputs only.
- `target_id`, `action`, `created_after`, `created_before` (request): purpose = let an authorised admin locate audit rows; consent basis = legal obligation / legitimate interest (accountability of privileged actions, Decree 13/2023 processing records); retention = not stored (not logged either; the handler never logs filter values).
- Returned rows are unchanged: admin user id, target ids, catalogue metadata. Retention follows the team-audit policy (immutable; erasure out of scope per initiative non-goals).
- Helper test data: the e2e helper seeds synthetic ids only (`e2e-aud-*`); no real user data.

## Threat model

- Assets: the audit trail (integrity and confidentiality of who-did-what).
- Abuse cases:
  - A non-admin or anonymous caller reads the trail through the new filters (enumeration of actors and targets). Mitigation: `RequireAdmin` first, unchanged; the gateway keeps `QueryAuditLog` in `adminProcedures`. Scenarios assert 401/403 with filters set.
  - SQL injection through `target_id` or `action`. Mitigation: bound parameters; a scenario passes a quote-laden value and gets an empty result, not an error.
  - Resource exhaustion through broad time ranges or deep offsets. Mitigation: page size cap 100 (unchanged), action index; exact-match only.
  - Forging the trail through the new helper or seeding. The helper writes only to the local e2e stack (direct SQL into `audit_db` through the postgres container); production write stays service-principal only.
- Authz rule: `QueryAuditLog` requires an authenticated principal with scope `admin` (a service principal also needs `admin`); every filter combination has the same rule.

## Open items

- A `target_id` filter without `target_type` is not index-served; the viewer is expected to send both. Add `(target_id, created_at DESC)` only if the viewer starts sending `target_id` alone in volume.
- Time-only queries have no dedicated index.
