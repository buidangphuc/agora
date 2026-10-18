## Why

`EngagementService.ResolveDispute` (`team-engagement/internal/handler/engagement.go`) is admin-only (`interceptor.RequireAdmin`) and rules on a buyer/seller dispute, yet it leaves no record of which admin ruled, when, or on what. The initiative `q3-audit-log` (locked 2026-10-15) requires every admin-only state change to write one team-audit event first and to fail closed. This slice covers the `dispute.resolve` row of the catalogue. It reuses the team-audit client and `UPSTREAM_AUDIT_ADDR` wiring that `engagement-review-integrity` (Q2) added for `review.hide`.

initiative: q3-audit-log

## What Changes

- **team-engagement upstream client** (`internal/upstream/audit.go`): add a generic `WriteEvent(ctx, AuditEvent{ActorID, Action, TargetType, TargetID, Metadata})` on `AuditClient`. `WriteHideEvent` stays and delegates to it, so `review.hide` behaviour is unchanged. Same service principal (`service-team-engagement`, scope `audit.write`), same bounded timeout, nil client returns an error.
- **Service** (`internal/service/dispute.go`): `DisputeService` gains `WithAudit(AuditWriter)` and `ResolveDispute` gains an `actorID` argument. The `AuditWriter` interface in `internal/service/review.go` gains `WriteEvent` (alongside `WriteHideEvent`) so one writer serves both services. Order of operations: validate, load dispute (not found), closed check, **audit write**, then `UpdateDispute`. Audit error or nil writer returns the new sentinel `ErrAuditUnavailable` (already defined in the service package), mapped to `Unavailable`.
- **Handler**: `ResolveDispute` passes the actor id taken from the verified Principal (never from the request) and maps `ErrAuditUnavailable` to `codes.Unavailable`. The existing `RequireAdmin` check stays first.
- **Wiring** (`cmd/server/main.go`): `disputeSvc.WithAudit(auditClient)` only when the client is non-nil, next to the existing `reviewSvc.WithAudit`. No new env var: `UPSTREAM_AUDIT_ADDR` is already read and already set in `docker-compose.services.yaml`.
- **E2E** (`platform-e2e`): new feature `features/engagement/dispute_audit.feature` using `support/audit_support.py` from `audit-query-filters`.
- No proto change, no migration, no gateway change (`ResolveDispute` is already an admin procedure there).

Audit event contract:

| Field | Value |
|---|---|
| `actor_id` | admin user id from the verified Principal |
| `action` | `dispute.resolve` |
| `target_type` / `target_id` | `dispute` / dispute id |
| `metadata.resolution` | the trimmed `resolution` text of the request (existing note field), possibly empty |
| `metadata.order_id` | the dispute's `order_id` |
| `metadata.status` | refinement of the catalogue: the requested status (`resolved`, `rejected` or `investigating`), so the trail shows the outcome even when the note is empty |

`ResolveDispute` is "always admin", so every call that reaches the audit step is audited; there is no owner path.

## NFR

- Latency: the added team-audit call is bounded by `UPSTREAM_CALL_TIMEOUT_SECONDS` (default <= 2 s), no retry.
- Availability: team-audit down, timing out, or `UPSTREAM_AUDIT_ADDR` unset blocks `ResolveDispute` only (`Unavailable`, dispute unchanged). CreateDispute, GetDispute and all other RPCs are unaffected.
- Limits: `resolution` is stored and audited as already allowed by the existing handler (no new limit). team-audit imposes no metadata size limit; `resolution` is audited at its existing length.
- At-least-once: if the audit write succeeds and `UpdateDispute` fails, the call returns `Internal`; a retry writes a second event (accepted, as in Q2).

## Data classification

| Field | Class | Purpose | Basis | Retention |
|---|---|---|---|---|
| audit `actor_id` (admin user id) | personal (staff identifier) | accountability of who ruled | legal obligation / legitimate interest (Decree 13/2023 processing records) | team-audit policy (immutable) |
| audit `target_id` (dispute id), `metadata.order_id` | pseudonymous identifiers | locate the dispute and order | same | same |
| audit `metadata.resolution` (admin-typed note, existing field) | personal-light (may mention parties) | record the ruling as given | same | same |
| audit `metadata.status` | metadata | outcome | same | same |

No claimant or defendant id, dispute reason or evidence URL is copied into the audit event. The resolution note is not written to logs.

## Threat model

- **Assets:** dispute rulings (affect refunds and seller standing), the audit trail, the service principal credential for team-audit.
- **Abuse cases:**
  - Ruling on a dispute without a trail: audit is written before the update; audit failure means no update.
  - Forged actor: `actor_id` comes from the verified Principal, never from the request; the user's own principal is not forwarded to team-audit, only the service principal with `audit.write`.
  - Forcing team-audit down to silence a ruling: fail closed, `Unavailable`, dispute unchanged.
  - Buyer or seller ruling on their own dispute: `RequireAdmin` first; denied callers write no event.
  - Probing dispute existence via audit: not-found and closed disputes return before the audit call, so they write no event.
- **Authz rule:** authenticated caller with the `admin` scope (unchanged). Audit is written only after authz, validation, load and closed check have passed.
