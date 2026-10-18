## Why

Four team-order RPCs let an admin change an order's life without leaving a trace: `ForceFailSaga` (cancels any order), `UpdateOrderStatus`, `UpdateReturnStatus` and `CreateShipment`. In `team-order/internal/handler/order.go` the last three accept the seller or any caller holding `order.admin` (`isAdminOrUser`), so an operator can ship, complete, refund or reject on a seller's behalf and nobody can later answer "who touched this order". The locked initiative (`plans/q3-audit-log/00-spec.md`, 2026-10-15) requires a team-audit event, written before the change, for every state change that succeeds only because of the admin scope, failing closed when team-audit is unavailable. This slice covers the four team-order rows of the catalogue. The audit contract (QueryAuditLog filters, proto) belongs to `q3-audit-log-contract`; the query helper for e2e belongs to `audit-query-filters`.

initiative: q3-audit-log

## What Changes

- **team-order**
  - New `internal/upstream/audit.go`, mirroring `team-engagement/internal/upstream/audit.go` in a per-repo copy and following the style of `internal/upstream/domain.go`:
    - `AuditClient` with `NewAuditClient(addr, timeout)` returning `(nil, nil)` for an empty address, and `Close()`.
    - Its own client interceptor replaces the whole outgoing principal with `x-principal-id=service-team-order`, `x-principal-type=service`, `x-principal-scopes=audit.write` (exactly one scope). The caller's principal is never forwarded. This is separate from the `domain.go` interceptor, which keeps its `listing.write` / `promotion.reserve` scopes.
    - One method `WriteAdminEvent(ctx, actorID, action, targetType, targetID string, metadata map[string]string) error` calling `AuditService.WriteAuditEvent`, bounded by the call timeout, no retry.
  - Config: `UPSTREAM_AUDIT_ADDR` (default empty) and `UPSTREAM_CALL_TIMEOUT_SECONDS` (default 2) in `internal/config/config.go` `Upstream`; wired in `cmd/server/main.go` only when the client is non-nil.
  - Handler (`internal/handler/order.go`): a small `AuditWriter` interface (`WriteAdminEvent`) injected with a new option `WithAuditWriter`, so unit tests use a fake. A nil writer means admin-path calls return `Unavailable`.
  - "Admin path" for `UpdateOrderStatus`, `UpdateReturnStatus` and `CreateShipment` = the caller holds `order.admin` (decided 2026-10-15; the catalogue's "admin" means `order.admin` in team-order). There is no owner exclusion: an `order.admin` caller who is also the seller is audited too. `ForceFailSaga` is always admin path (it already requires both `admin` and `order.admin`). The e2e seeded admin token carries both `admin` and `order.admin` (see ar2_order_admin.feature).
  - Order of steps on an admin path: authenticate, authorize, validate, load the target (NotFound), write the audit event, apply the change. Audit error, timeout or nil writer returns `Unavailable` with no change. A change failure after a successful audit returns the change's own error and the audit row stays.
  - `ForceFailSaga` today does not load the order before `CancelOrder`; it gains a `GetOrder` load so an unknown order id returns `NotFound` with no audit event.
  - Events (all `actor_id` = the caller's user id from the verified Principal, never from the request):

    | RPC | action | target_type / target_id | metadata |
    |---|---|---|---|
    | ForceFailSaga | `order.force_fail` | `order` / order id | `fail_step` ("payment", "shipping" or "unspecified" when empty) |
    | UpdateOrderStatus | `order.admin_status_update` | `order` / order id | `seller_id`, `to_status` |
    | UpdateReturnStatus | `return.admin_status_update` | `return_request` / return id | `order_id`, `to_status` |
    | CreateShipment | `shipment.admin_create` | `order` / order id | `carrier` (free text today: truncated to 64 runes; empty recorded as the service default "SPX") |

    `to_status` is the requested target status enum name (for example `ORDER_STATUS_SHIPPED`, `RETURN_STATUS_APPROVED`). `tracking_number`, tracking codes and addresses are NOT put in metadata.
  - Seller path (caller holds no `order.admin`) is not audited, makes no audit call and is independent of team-audit availability. Buyer and stranger calls are refused by the existing authz before any audit call.
  - Proto: `proto/platform/audit/v1/audit.proto` copied from `platform-core/packages/proto` (the contract slice's version) and Go stubs regenerated; consumers never edit the copy.
- **platform-e2e** (API level, owned by this slice): new feature `features/order/admin_audit.feature` with step bindings, using `support/audit_support.py` from `audit-query-filters` (functions `assert_one_event(world, target_type=, target_id=, action=, actor_id=, metadata=)`, `assert_no_event`, `stop_audit(world)`, `start_audit(world)`). Setup: Paid orders via `oic_order_support.put_in_status`; a Requested return via the existing RMA steps (`prm_rma_steps.py`); the seeded admin via `login_admin`.
- Not changed: team-identity, team-gateway policy (the four RPCs are already routed), `docker-compose.services.yaml` (`UPSTREAM_AUDIT_ADDR=team-audit-svc:50066` for team-order is added by the contract slice), buyer-side `CancelOrder`, read RPCs, `ProcessMockPayment`.

## NFR

- Added latency on the four admin paths is at most one `WriteAuditEvent` call bounded by `UPSTREAM_CALL_TIMEOUT_SECONDS` (default 2 s), no retry.
- Seller and other non-admin paths add no call and no latency.
- Availability: team-audit down, slow or unconfigured blocks only these four admin actions with `Unavailable`; checkout, seller operations, buyer cancel and all reads are unaffected.
- Limits: metadata values are enum names and ids; `carrier` is the only free-text value and is truncated to 64 runes (an allow-list was rejected: existing e2e and journeys send "SPX Express" and `service.CreateShipment` accepts any carrier).

## Data classification

| Field | Class | Purpose | Basis | Retention |
|---|---|---|---|---|
| audit `actor_id` (admin user id) | personal (staff identifier) | accountability of privileged order actions | legal obligation / legitimate interest (Decree 13/2023 processing records) | team-audit retention policy (immutable) |
| audit metadata `seller_id` | personal-light (user id) | identify whose order was touched | same | same |
| audit metadata `order_id`, `to_status`, `carrier`, `fail_step` | operational, non-personal | describe the action | same | same |
| `target_id` (order / return id) | operational | name the target | same | same |

Not recorded: buyer id, addresses, tracking numbers or codes, amounts, other free text.

## Threat model

- **Assets:** order and return state (money and stock follow from them), the audit trail, the `service-team-order` audit credential.
- **Abuse cases:**
  - Operator moves an order or return on a seller's behalf unnoticed: audit written first on the admin path; no event, no change.
  - Operator drives team-audit down or unsets the address to act silently: fail closed, `Unavailable`, order unchanged.
  - Forged audit entries: the user's principal is never forwarded; only the service principal with `audit.write` writes, and `actor_id` comes from the verified Principal.
  - Admin acting through their own seller account: audited too (no owner exclusion), so there is no way to use `order.admin` unaudited.
  - Denied or invalid calls polluting the trail: authz, validation and load errors return before the audit write, so they write no event.
  - Audit row without a change (change fails after audit): accepted, the row records an attempt; the response carries the change's own error.
  - Seller path audited by accident: unit and e2e cases assert zero events and zero audit calls.
- **Authz rule (unchanged):** `UpdateOrderStatus`, `UpdateReturnStatus`, `CreateShipment` need the order's seller or `order.admin`; `ForceFailSaga` needs `admin` AND `order.admin`. Audit applies whenever the caller holds `order.admin` (for the three seller-or-admin RPCs), seller or not. Pure sellers are unaudited.
- **Fail-closed coverage:** `Unavailable` for force-fail, return and shipment is verified at handler level only (deliberate); end to end, only the status-update path stops team-audit.
