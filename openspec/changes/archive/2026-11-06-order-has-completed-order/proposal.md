## Why

`promotion-new-user-vouchers` (initiative `q7-q37-referral-welcome`, D6) must refuse a `new_user_only` voucher to a
buyer who already has a completed order. team-promotion may not read the order database (AGENTS.md §3.3), so
team-order has to answer "has this buyer completed an order" through its own gRPC surface.

initiative: q7-q37-referral-welcome

## What changes

- `team-order` implements `OrderService.HasCompletedOrder(HasCompletedOrderRequest{buyer_id}) returns
  (HasCompletedOrderResponse{has_completed})`, declared by the `q7-referral-welcome-contract` slice. `has_completed`
  is true iff the buyer has at least one order with status `ORDER_STATUS_COMPLETED` (4).
- Internal only: the caller must be a SERVICE principal holding the `order.read` scope (the same rule team-order
  already applies in `canViewOrder` and `GetReturnRequest`). Users, sellers and admins are refused; unlike `canViewOrder`, `order.admin` does not grant access here.
- The RPC is not routed at team-gateway: `OrderForwarder` embeds `UnimplementedOrderServiceHandler`, so the edge
  answers 501 unimplemented and never contacts team-order. No gateway change; a gateway test pins it.
- A read-only store query (`EXISTS` over `orders` by `buyer_id` and status, served by `orders_buyer_idx`). No
  migration, no event, no write.

## Capabilities

### New Capabilities
- `order-completion-lookup`: internal answer to "has this buyer completed an order".

### Modified Capabilities
none

## Impact

- `team-order/internal/rpc/order.go`, `team-order/internal/store/order.go` (+ fake store), tests, `team-order/FEATURES.yaml` (e2e-author).
- `team-gateway` edge test only (unrouted guard); no production change there.
- Depends on regenerated stubs from `q7-referral-welcome-contract`.

## Non-goals

Counting orders, listing them, filtering by date, or any user-facing route; completed-by-seller vs buyer distinction;
caching. Other statuses (CANCELLED, PAID, SHIPPED, PENDING) never count.

## Non-functional requirements

- Latency: indicative target p99 under 20 ms on the local stack (single indexed `EXISTS`), not a gated number; called only for `new_user_only` vouchers.
- Availability: stateless read; failure returns a gRPC error, never a guessed `false`.
- Limits: one buyer id per call, no batching.

## Data classification

| Field | Class | Purpose | Basis | Retention |
|---|---|---|---|---|
| `buyer_id` (request) | personal identifier | voucher eligibility lookup | contract (providing the purchase benefit) | not stored by this RPC |
| `has_completed` (response) | derived order-history fact | eligibility decision | contract | not stored by this RPC |

Logs carry no buyer id in any form: no raw id and no fingerprint of it.

## Threat model

- Assets: whether a given account has purchase history.
- Abuse cases: a buyer or seller probes another account's purchase history through the gateway; a user token calls
  the RPC directly; a service without the scope probes.
- Authz rule: only SERVICE principals with `order.read`; not reachable from the edge; empty `buyer_id` is
  `INVALID_ARGUMENT` so the lookup cannot be used to scan with a blank key.
