## Why

Initiative `trust-legal` (Q24) lets a user delete their account. Before deletion is scheduled, identity must know
whether the user still has open commerce obligations (open orders, open returns, wallet money, pending payouts),
and after `UserErased` the commerce services must stop holding the user's contact and bank details. Today
team-order keeps the recipient name, phone and address in `orders.shipping_address`, team-payment keeps
`payout_requests.account_number` and `account_name`, and none of team-order, team-payment, team-promotion or
team-referral reads `identity.events/UserErased` (the event does not exist yet).

initiative: trust-legal

## What Changes

- team-order implements `OrderService.GetUserObligations` (internal, `order.internal.read`).
- team-payment implements `PaymentService.GetPayoutObligations` (internal, `payment.internal.read`).
- Both RPCs stay unrouted at the gateway (501); the gateway edge test for each is part of this slice.
- Four new Kafka consumers on `identity.events`, one per service (team-order, team-payment, team-promotion,
  team-referral), each with its own consumer group and a DLQ `identity.events.<svc>.dlq`, idempotent by user id:
  - team-order: masks the buyer's recipient name, phone and address on all their orders and the free-text reason on
    their returns with "Đã xóa"; deletes their cart; keeps ids, amounts, items and shipments.
  - team-payment: masks `account_number` and `account_name` on the seller's payout requests with "Đã xóa"; keeps
    ids, `bank_code`, amounts, the ledger and payment transactions.
  - team-promotion: deactivates vouchers owned by the user and deletes their new-user voucher state; keeps
    reservations/redemptions (ids and amounts).
  - team-referral: deletes the user's referral code, account row and first-order row; keeps attribution rows and
    the reward ledger (ids and amounts).
- Compose: DLQ topics `identity.events.order.dlq`, `identity.events.payment.dlq`, `identity.events.referral.dlq`
  added to the topic init (`identity.events.promotion.dlq` exists); consumer env for the four services.
- No schema migration: every erasure is an UPDATE or DELETE over existing columns.

## Capabilities

### New Capabilities
- `order-user-obligations`: `GetUserObligations`.
- `payment-payout-obligations`: `GetPayoutObligations`.
- `order-user-erasure`: team-order handling of `UserErased`.
- `payment-user-erasure`: team-payment handling of `UserErased`.
- `promotion-user-erasure`: team-promotion handling of `UserErased`.
- `referral-user-erasure`: team-referral handling of `UserErased`.

### Modified Capabilities
none

## Impact

- `team-order`: `internal/rpc`, `internal/store`, `internal/events`, `internal/config`, `internal/app`, regenerated proto.
- `team-payment`: `internal/rpc`, `internal/store`, `internal/events` (new consumer), `internal/config`, `internal/app`.
- `team-promotion`: `internal/consumer`, `internal/store`, `internal/app` (new consumer group).
- `team-referral`: `internal/consumer`, `internal/store`, `internal/config`, `internal/app`.
- `docker-compose.services.yaml` (topic init, consumer env): hot file shared with the other erasure slices.
- `team-gateway`: tests only, asserting the two new RPCs answer 501 (not listed under the slice's repos; see OPEN_ISSUES).
- Depends on `trust-legal-contract` (RPC and `UserErased` messages in `platform-core/packages/proto`); no proto
  edit here.

## Non-goals

Deciding whether deletion is allowed (identity does, from these counts); erasing engagement, chat, notification,
domain, analytics, ML, verification or audit data (other slices); the accounting retention length (legal,
NEEDS_HUMAN); re-checking obligations when the sweeper runs (identity-account-deletion).

## Non-functional requirements

- `GetUserObligations` and `GetPayoutObligations`: p95 at most 100 ms locally, so identity's synchronous fail-closed
  check stays well inside the initiative's 2 s class of budgets; both are read-only and single queries per RPC.
- Erasure of one user completes in each of the four services within 1 h of `UserErased` being published, with
  the service's DLQ empty (initiative NFR).
- Each erasure runs in one database transaction per service: it either fully applies or not at all.
- Consumers retry a store error until it succeeds; only a malformed event goes to the DLQ.
  events go to the DLQ.
- Boot fails without Kafka or Postgres; no `*_ENABLED` flag, no in-memory fallback.

## Data classification

| Field | Where | Class | Purpose | Basis | Retention |
|---|---|---|---|---|---|
| recipient name, phone, street, ward, district, city | `orders.shipping_address` (team-order) | personal | delivering the order | contract performance | masked with "Đã xóa" at erasure |
| return `reason` free text | `order_returns.reason` | personal (free text) | resolving the return | contract performance | masked at erasure |
| cart contents | `cart_items` | personal (behavioural) | checkout convenience | contract / consent | deleted at erasure |
| order ids, buyer/seller ids, items, amounts, status | team-order | pseudonymous business record | accounting | legal obligation | kept for the accounting period set by legal (NEEDS_HUMAN) |
| payout `account_number`, `account_name` | `payout_requests` (team-payment) | financial, personal | paying the seller | contract performance | masked at erasure, never kept |
| payout `bank_code`, amounts, ledger, payment transactions | team-payment | pseudonymous business record | accounting | legal obligation | accounting period (NEEDS_HUMAN) |
| voucher ownership, new-user voucher state | team-promotion | personal (user id keyed) | issuing welcome vouchers | consent / contract | vouchers deactivated, state deleted at erasure |
| voucher reservations/redemptions | team-promotion | pseudonymous business record | accounting | legal obligation | kept, ids and amounts |
| referral code, account row, first-order row | team-referral | personal (user id keyed) | invite programme | contract | deleted at erasure |
| referral attributions, reward ledger | team-referral | pseudonymous business record | accounting | legal obligation | kept, ids and amounts |
| obligation counts and balances (RPC payloads) | in transit | business data tied to a user id | block deletion while open | legal and contract | not stored by the callee |

After erasure the user id in kept rows is no longer linked to a person: identity anonymises the account. Logs carry
counts and hashed ids only.

## Threat model

- Assets: recipient and bank details of buyers and sellers; wallet balances; the integrity of orders and the ledger.
- Abuse cases:
  - A user or seller calls the obligations RPCs to learn another user's balance or open orders. Mitigation: only a
    SERVICE principal with the scope is accepted; USER principals get `PERMISSION_DENIED`; the gateway answers 501.
  - A forged `UserErased` erases a victim's data. Mitigation: the event is read only from `identity.events`, which
    only team-identity writes through its outbox; consumers act on `user_id` only, never on caller-supplied roles,
    and ignore every other event type.
  - A malformed or replayed event corrupts data. Mitigation: empty `user_id` goes to the DLQ without side effects;
    replays are no-ops.
  - Erasure masks data of an order still being shipped. Mitigation: identity blocks deletion while obligations are
    open; re-check at sweep time is open for identity-account-deletion (OPEN_ISSUES).
  - Over-broad delete touching another user. Mitigation: every statement filters on the event's `user_id` only, and a
    scenario proves another user's rows are unchanged.
- Authz rule: `GetUserObligations` requires SERVICE principal with `order.internal.read`; `GetPayoutObligations`
  requires SERVICE principal with `payment.internal.read`; anonymous is `UNAUTHENTICATED`, everything else
  `PERMISSION_DENIED`.
