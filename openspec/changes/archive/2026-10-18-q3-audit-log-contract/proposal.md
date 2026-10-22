## Why
The Q3 audit initiative needs admins to answer "who touched this order/listing/user, and when". `QueryAuditLogRequest` can only filter by `actor_id` and `target_type`, and the `<entity>.<verb>` action naming used by six producer services is written nowhere. This slice adds the wire contract (additive, buf-breaking safe), the proto copies every producer and consumer needs, the compose wiring of `UPSTREAM_AUDIT_ADDR`, and the workspace rule that future admin RPCs must audit first. Behaviour lives in the later slices.

initiative: q3-audit-log

## What changes
In `platform-core/packages/proto/platform/audit/v1/audit.proto`:
- `QueryAuditLogRequest` gains `optional string target_id = 4` (exact match), `optional string action = 5` (exact match, e.g. `kyc.review`), `google.protobuf.Timestamp created_after = 6` (inclusive; unset = no lower bound) and `google.protobuf.Timestamp created_before = 7` (exclusive; unset = no upper bound). All filters AND together. `created_before` earlier than `created_after` is `InvalidArgument` (enforced in team-audit, documented in the RPC comment). Existing fields 1-3 keep their number, name and type; paging (offset cursor, default 20, max 100, newest first) is unchanged.
- `WriteAuditEventRequest` and `WriteAuditEvent` get comments only: the naming rule `<entity>.<verb>` and the catalogue of the 12 Q3 actions plus the shipped `review.hide`.
- The proto is copied byte-identical into `proto/platform/audit/v1/` of team-audit, team-gateway, team-analytics, team-engagement (existing copies, updated) and team-frontend, team-verification, team-domain, team-order, team-payment, team-promotion (new copies). Consumers regenerate in their own slices and never edit the copy.
- A committed Go test `platform-core/packages/proto/platform/audit/v1/auditquery_roundtrip_test.go` (same approach as Q2 `hidereview_roundtrip_test.go`).
- Root `docker-compose.services.yaml`: `UPSTREAM_AUDIT_ADDR=team-audit-svc:50066` added to team-verification, team-domain, team-order, team-payment, team-promotion (team-engagement already has it).
- Root `AGENTS.md` rule 6 and a matching BLOCKING check in `.claude/agents/auth-scope-reviewer.md`: an RPC whose mutation depends on the `admin` or `order.admin` scope writes a team-audit event first and fails closed (`Unavailable`).

Out of scope: team-audit filtering behaviour (audit-query-filters), producer audit writes, the viewer page, gateway policy (QueryAuditLog is already admin-gated, WriteAuditEvent stays unrouted), per-environment gitops values.

## NFR
- Additive only: `buf breaking` against HEAD passes; clients that ignore the new fields keep working; no field number or type changes.
- Contract-level target for implementers: filtered QueryAuditLog p95 < 300 ms on 100k rows (non-gating, initiative NFR).
- The compose change adds no new service and no new port; producers with the variable unset must still start (default unset, fail-closed only on admin paths, enforced in producer slices).
- No new dependencies.

## Data classification
| field | class | purpose | basis | retention |
|---|---|---|---|---|
| `QueryAuditLogRequest.target_id` | non-personal identifier (may name a user id for `kyc_submission` lookups) | filter | legitimate interest | transient on the wire |
| `QueryAuditLogRequest.action`, `created_after`, `created_before` | non-personal | filter | contract | transient |
| `QueryAuditLogRequest.actor_id` (existing) | personal (staff id) | accountability queries | legal obligation / legitimate interest (Decree 13/2023 processing records) | transient on the wire; audit rows follow team-audit policy |

No new personal data stored. Comment-only changes add no data. The catalogue comment names metadata keys but never free text.

## Threat model
- Assets: the audit trail's integrity and confidentiality; the guarantee that admin mutations are always recorded.
- Abuse: a non-admin reads the audit log (QueryAuditLog stays admin-only at the edge, authoritative check in team-audit); a client forges `actor_id` on write (WriteAuditEvent is unrouted at the gateway and needs a service principal with `audit.write`); a future admin RPC ships without an audit write (mitigated by the AGENTS.md rule and the auth-scope-reviewer check); compose leaves a producer without `UPSTREAM_AUDIT_ADDR` so admin actions are blocked (fail closed, visible as `Unavailable`).
- Authz rule: no new RPC and no authz change in this slice. `QueryAuditLog` requires `admin`; `WriteAuditEvent` requires service principal + `audit.write`. The new fields cannot widen access because they only narrow the result set.
