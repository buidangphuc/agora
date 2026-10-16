## Context

Depends on `q3-audit-log-contract`: the proto copies in team-audit and team-gateway must already hold `QueryAuditLogRequest.target_id` (4), `action` (5), `created_after` (6), `created_before` (7), and `make proto` must have been run in both repos (`generated/` is gitignored).

## team-audit design

- `repository.QueryFilter{ActorID, TargetType, TargetID, Action string; CreatedAfter, CreatedBefore time.Time; Limit, Offset int}`. Zero `time.Time` = no bound.
- Postgres: append `AND target_id = $n`, `AND action = $n`, `AND created_at >= $n`, `AND created_at < $n` to the same `where` string used by the COUNT and page queries (placeholders numbered from `len(args)`), so total and page can never disagree.
- In-memory: same predicates on `ev.CreatedAt.AsTime()`; an event without `CreatedAt` is never stored (Append sets it).
- Service: `Query(ctx, QueryParams{...})` validates the range (`ErrInvalidTimeRange`), clamps paging as today. The handler converts `*timestamppb.Timestamp` with `.AsTime()` only when non-nil (`GetCreatedAfter() != nil`).
- `mapAuditErr`: `ErrInvalidTimeRange` -> `InvalidArgument`.
- Migration `002_audit_action_created_index.{up,down}.sql`: `CREATE INDEX IF NOT EXISTS idx_audit_events_action_created ON audit_events (action, created_at DESC);` / `DROP INDEX IF EXISTS ...`.
- Equal bounds are legal and empty (half-open range); only strictly inverted is an error.

## Shared helper: `platform-e2e/tests/e2e/support/audit_support.py`

Written by this slice only; other slices import, never edit. Black box through the gateway as the seeded admin; seeding by direct SQL into `audit_db` (no host port for team-audit, and no gRPC client in the e2e venv; SQL is the only seeding path and also lets tests choose `created_at`). Container and credentials follow the existing support modules: `AUDIT_CONTAINER` (default `agora-team-audit-svc`), `AUDIT_POSTGRES_CONTAINER` (default `agora-postgres-1`), `docker exec <postgres> psql -U postgres -d audit_db`. Producer-written events (the real thing under test) always arrive through the producer RPC, never through `seed_events`.

```python
AUDIT = "/platform.audit.v1.AuditService"

class AuditUnavailable(AssertionError): ...        # gateway 5xx / transport error on query

def fresh_target(prefix: str = "e2e-aud") -> tuple[str, str]:
    """(target_type, target_id), both unique per call, for scenarios that need an unshared target."""

def admin_token(world) -> str:
    """Login as the seeded admin (same lookup as edge_steps._admin_token)."""

def query_events(
    token: str, *, target_type: str | None = None, target_id: str | None = None,
    actor_id: str | None = None, action: str | None = None,
    created_after: str | None = None, created_before: str | None = None,   # RFC 3339
    page_size: int = 100, cursor: str | None = None,
) -> httpx.Response:
    """Raw QueryAuditLog through the gateway (Connect JSON, camelCase keys). No assertions."""

def events_for(
    world, target_type: str, target_id: str | None = None, *, action: str | None = None,
    actor_id: str | None = None, token: str | None = None,
) -> list[dict]:
    """All matching events newest first (follows nextCursor). Asserts HTTP 200; raises AuditUnavailable on 5xx.
    token defaults to the seeded admin's."""

def assert_one_event(
    world, *, target_type: str, target_id: str, action: str, actor_id: str,
    metadata: dict[str, str] | None = None,
) -> dict:
    """Exactly one event with that action for the target; actor_id equal; every key in `metadata` equal
    (extra keys allowed). Returns the event."""

def assert_no_event(world, *, target_type: str, target_id: str, action: str | None = None) -> None:
    """No event (of `action`, when given) for the target."""

def seed_events(rows: list[dict]) -> list[str]:
    """Insert rows directly into audit_db.audit_events; returns the ids. Each row:
    {actor_id='e2e-actor', action, target_type, target_id, metadata={}, created_at=None}
    where created_at is an RFC 3339 string (None = now). ids are `e2e-aud-<8 hex>`."""

def stop_audit(world) -> None:
    """docker stop team-audit; registers a world cleanup that starts it and waits until an admin
    QueryAuditLog returns 200 (re-uses the recovery loop of edge_steps.stop_audit). Idempotent."""

def start_audit(world) -> None:
    """Start it early (the cleanup is then a no-op) and wait until QueryAuditLog answers 200."""
```

Producer-slice usage rule: a success scenario calls `assert_one_event(...)` after the RPC; a refused/owner-path scenario calls `assert_no_event(...)`; a fail-closed scenario calls `stop_audit(world)`, performs the admin RPC, asserts `Unavailable` and the unchanged target, then `start_audit(world)`. The scenario must be tagged `@destructive` (existing convention for the lane that may stop containers).

## E2E feature design

`security/audit_query_filters.feature`, all scenarios as the seeded admin through the gateway; each seeds a fresh target with `fresh_target()` so scenarios cannot see each other's rows. Time-boundary scenarios seed events at fixed instants (e.g. `2026-01-01T10:00:00Z`, `11:00:00Z`, `12:00:00Z`) and bound with the same instants.

## Gateway design

Regeneration only (`make proto`); add one forwarder test with a fake `AuditServiceClient` recording the request. No policy change.

## Risks

- Postgres `created_at` precision is microseconds; protobuf Timestamp is nanoseconds. Bounds are compared as sent; seeded events use whole seconds, so boundary scenarios are exact.
- Offset paging can shift under concurrent writes (existing gap, unchanged).

### Round 2 — 2026-10-15

- Q1 [naming] scenario "In-memory repository matches Postgres" duplicates a title owned by engagement-review-integrity (team-engagement FEATURES.yaml); rename to "Audit query: In-memory repository matches Postgres"? → **approved** (human, converge)
