## Why

Today an admin can rewrite or delete any seller's listing through `UpdateListing` / `DeleteListing` (team-domain `principalOwner` / `isAdmin`, `service/listing.go` `UpdateWithEvent` / `DeleteWithEvent`) and nothing records who did it. Trust-ops finding #5 requires every admin-scope mutation to leave a team-audit event and to be refused when the audit cannot be written. This slice covers the two team-domain rows of the initiative catalogue.

initiative: q3-audit-log

## What changes

- New `team-domain/internal/upstream/audit.go`: `AuditClient` (mirrors `team-engagement/internal/upstream/audit.go`). It dials `UPSTREAM_AUDIT_ADDR`, replaces the outgoing principal with `x-principal-id=service-team-domain`, `x-principal-type=service`, `x-principal-scopes=audit.write`, and calls `AuditService.WriteAuditEvent` with the admin's user id as `actor_id`. Empty address returns a nil client.
- New config keys `UPSTREAM_AUDIT_ADDR` (default empty) and `UPSTREAM_CALL_TIMEOUT_SECONDS` (default 2) in `internal/config`, `.env.example` and README.
- `ListingService` gains an injected `AuditWriter` interface. In `UpdateWithEvent` / `DeleteWithEvent`, after the existing load (NotFound) and the owner/admin check, and only when the caller holds the `admin` scope (whether or not the caller owns the listing), the service writes the audit event (seller_id from the stored listing), then applies the change. Audit error, timeout or nil writer returns a new sentinel mapped by the handler to `Unavailable`; the listing row and the outbox are untouched.
- Events: `listing.admin_update` (target_type `listing`, target_id listing id, metadata `seller_id`, `status`) and `listing.admin_delete` (metadata `seller_id`). `status` on update is the lowercase stored-form status string the update applies (`draft`, `published`, `rejected`, as team-domain stores it, see `statusPublished` in handler/listing.go); when the request carries `LISTING_STATUS_UNSPECIFIED` it is the listing's current stored status.
- The non-admin owner path (caller id equals the stored `seller_id`, no `admin` scope) makes no audit call and does not depend on team-audit.
- Audit proto copy `team-domain/proto/platform/audit/v1/audit.proto` is added (vendored from platform-core after `q3-audit-log-contract`) and generated locally by `make proto`.
- Wiring in `cmd/server/main.go`: the writer is attached only when the client is non-nil; `docker-compose.services.yaml` `UPSTREAM_AUDIT_ADDR` for team-domain is set by the contract slice, not here.
- E2E: admin update and admin delete are audited, owner update is not, and audit-down refuses the admin path with the listing unchanged.

No gateway change: `UpdateListing` / `DeleteListing` routing and scopes are unchanged.

## Non-functional requirements

- Latency: the admin path adds one gRPC call bounded by `UPSTREAM_CALL_TIMEOUT_SECONDS` (default 2 s), no retry. The non-admin owner path adds no call and no latency.
- Availability: with team-audit down or `UPSTREAM_AUDIT_ADDR` unset, every admin-scope edit/delete returns `Unavailable` (accepted, decision 2026-10-15); non-admin owner edits/deletes, reads and listing creation are unaffected.
- Limits: one audit event per admin RPC call; a retry after a post-audit failure may write a second row (no idempotency key, initiative non-goal).

## Data classification

| field | class | purpose | basis | retention |
|---|---|---|---|---|
| `actor_id` (admin user id) | personal (pseudonymous id) | accountability of privileged actions | legal obligation / legitimate interest (Decree 13/2023 processing records) | team-audit policy, immutable |
| `target_id` (listing id) | non-personal | identify the touched listing | same | same |
| `seller_id` metadata | personal (pseudonymous id) | tell whose listing was touched | same | same |
| `status` metadata | non-personal | record the resulting listing status | same | same |

No titles, descriptions, prices, names or free text are written to the audit event.

## Threat model

- Assets: seller listings; the audit trail's integrity; the `audit.write` service principal.
- Abuse cases: (1) an admin edits or deletes another seller's listing without a record, prevented by audit-first and fail-closed; (2) a non-admin forges an audit event, prevented because team-domain never forwards the caller's principal and `WriteAuditEvent` is unrouted at the gateway; (3) a seller or admin spoofs `actor_id` via the request, prevented because the actor id comes from the verified Principal, never from the request; (4) audit outage used to dodge the trail, prevented by returning `Unavailable` and changing nothing; (5) an admin editing their own listing to avoid a record, prevented because the rule is uniform for every admin-scope call.
- Authz rule: `listing.write` scope required (unchanged); owner (id equals stored `seller_id`) may update/delete, audited only if the caller also holds `admin`; a non-owner is allowed only with the `admin` scope and then audited; anyone else gets `PermissionDenied` with no audit event.
