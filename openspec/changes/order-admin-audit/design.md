## Context

`isAdminOrUser(principal, ids...)` returns true for a listed user id or for any holder of `order.admin`. The handler cannot tell afterwards which clause matched, and an admin could also be the seller of the order. The audit rule needs exactly that distinction.

## Decisions

- **Helper `isAdminPath(principal)`**: true when `hasScope(principal, order.admin)`, evaluated after `isAdminOrUser` passed. No owner exclusion (Clarification Q1). The scope that defines an admin in team-order is `order.admin` (the generic `admin` marker deliberately does not open orders, per authz-residuals-2); `ForceFailSaga` additionally requires `admin`.
- **Single injection point**: `AuditWriter` interface in the handler package; `WithAuditWriter(w)` option; `NewOrderHandler` signature unchanged. main.go passes the option only when `NewAuditClient` returned non-nil, so a typed-nil pointer is never stored in the interface.
- **Helper `h.auditAdmin(ctx, principal, action, targetType, targetID, meta) error`** returns a gRPC `Unavailable` status ("audit unavailable") when the writer is nil or the call errors; the cause is logged, not returned.
- **Audit before change, after load.** `UpdateOrderStatus`, `UpdateReturnStatus`, `CreateShipment` already load the target and authorize before the change, so the audit call slots in between. `ForceFailSaga` adds `GetOrder` for the NotFound check.
- **Why not audit inside the service layer**: the admin/seller distinction lives in the handler (principal scopes); the service takes an `ActorSeller` already.
- **Status-transition failure after audit** (e.g. `FailedPrecondition`) leaves the audit row, same as the Q2 `HideReview` pattern.
- **Carrier**: `service.CreateShipment` takes any carrier (empty becomes "SPX") and e2e sends "SPX Express", so an allow-list would break current behaviour. Metadata `carrier` is the effective carrier truncated to 64 runes.
- **ForceFailSaga saga_state**: the added `GetOrder` is a read before the existing flow; `sagaStateOf(cancelled.Order)` and the response are unchanged.
- **Per-repo copy of the audit client**: team-order imports go-sdk, but the initiative decided on per-repo copies; `internal/upstream/audit.go` stays standalone and keeps `domain.go`'s principal interceptor untouched.

## Clarifications

### Round 1 — 2026-10-15

- Q1 [permission]: uniform rule. Every UpdateOrderStatus, UpdateReturnStatus and CreateShipment call where the caller holds `order.admin` is audited, including an `order.admin` caller who is the seller. ForceFailSaga is unchanged (`admin` and `order.admin`, always audited). Callers without `order.admin` (pure sellers) are unaudited and unaffected by team-audit availability.
