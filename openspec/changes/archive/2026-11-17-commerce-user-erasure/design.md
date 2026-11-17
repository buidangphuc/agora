## Context

`trust-legal` locks event-driven erasure: identity writes `UserErased` through its outbox and each consuming service
erases or pseudonymises idempotently, with no cross-DB calls. Commerce data is mostly accounting data (kept with
ids and amounts) plus a few personal fields (recipient, phone, address, bank details). The obligation RPCs let
identity refuse deletion while money or goods are in flight.

## Decisions

- Mask, do not delete, order and payout rows: the accounting retention duty (period set by legal) needs ids and
  amounts; only direct identifiers become "Đã xóa". The constant is the same literal for every field and service.
- `shipping_address` is JSONB: the six personal keys are overwritten with `jsonb_set`; `id`, `user_id` and
  `is_default` stay. Idempotency follows from the update being value-based (masked rows stay masked).
- `order_returns.reason` is masked too. It is buyer free text that can carry names or addresses, and the locked
  decision lists recipient, phone and address only; this is a conservative extension (OPEN_ISSUES).
- Payout masking applies to every status including PENDING. Identity blocks deletion while payouts are pending, so
  a pending row at erasure means a payout was requested inside the 48 h cancel window (OPEN_ISSUES).
- `available_balance` is the ledger balance (which already nets pending payout debits) minus the held amount,
  clamped at 0, computed with the existing hold-back split; `held_balance` is that split's held amount. Identity
  treats any non-zero value of the two, or `pending_payouts` > 0, as a blocker.
- Open order statuses are PENDING, PAID and SHIPPED; COMPLETED orders are not open even inside the return window
  (an open return is counted through `open_returns`). Disputes live in team-engagement and are not part of this RPC.
- Consumers: one per service, own group, DLQ `identity.events.<svc>.dlq`, filtering on the envelope type
  `platform.identity.v1.UserErased`, mirroring `team-domain/internal/events/identity.go` (malformed -> `consume.Permanent`,
  store errors returned for retry). team-promotion already owns `identity.events.promotion.dlq` for its
  `UserRegistered` consumer; the erasure consumer has its own group and reuses that DLQ topic so no new topic is needed.
- team-referral and team-promotion existing identity consumers already filter by type; a scenario guards that
  `UserErased` does not break them.
- The user id comes only from the event payload. `roles` are not trusted and only logged as a count for the seller case;
  all handlers erase by user id irrespective of role.
- Obligation RPCs reuse each service's existing service-principal helper (`isServiceWithScope` in team-order);
  team-payment adds the equivalent for `payment.internal.read`. Gateway: nothing is added, the embedded
  `Unimplemented*Handler` already answers 501; a test pins it.

## Risks

- Between the sweeper reading obligations (request time) and erasure, the user could place an order or request a
  payout; the erasure then masks data of an in-flight record. Mitigation belongs to identity (re-check at sweep
  time) and is raised as an open issue.
- Shared hot file `docker-compose.services.yaml` is edited by every erasure slice; the build serialises them.
