# Design

## Decisions

- Audit before every change, once per processed event, only when a change is about to happen. No-op acknowledgements (REJECTED dispute, SELLER_FAVOUR, already REFUNDED, no `return_id`) write no audit row: nothing was done. A retry after a failure past the audit writes a second row for the same dispute; both rows carry `dispute_id`, which makes the repeat readable. Accepted over a dedupe table for the audit call (no new storage).
- COD orders are audited and moved to APPROVED even though no money moves: the return state changes on an admin's authority, and the audit rule for system money paths is applied uniformly to the whole consumer.
- `ReapproveReturn` is a new repository method, not a new transition in `UpdateReturnStatus`: REJECTED stays terminal for sellers and admins; only a dispute ruling reopens it. The remainder check reuses `returnRemainder` and the order-row lock of `CreateReturnCapped`.
- PENDING returns are handled too (escalation is allowed on PENDING past the q10 threshold, see story 6): approving them avoids a second, racing seller or system auto-approve.
- Retry policy: the shared `Run` loop with `RetryTransientUntilApplied` set. A transient error (audit down, slow or erroring, database error) retries the same record with a delay that grows per attempt and is capped at `MaxBackoff` (default 5 s); the offset is not committed and the record is never dead-lettered, so a refund is never parked silently and resumes within 5 s of team-audit returning (00-spec story 7: "the event is redelivered, no change"). Only `ErrPermanent` (non-fitting remainder, unknown return, order mismatch, missing resolver, malformed record) goes to the DLQ. Cost accepted: a long audit outage holds the `engagement.events` partition for team-order's group only; other consumer groups are unaffected. `PaymentConsumer` keeps its bounded retries (flag off by default).
- Dispute states (A17): `DisputeResolved` is emitted for RESOLVED and REJECTED (including OPEN to REJECTED spam closes). Only RESOLVED carries a ruling; the consumer still checks `status` first, so a REJECTED event is a no-op whatever its `ruling` field holds.
- Re-escalation (A18): a return whose dispute closed without a refund can be escalated again while `can_escalate` is true, so several `DisputeResolved` events may name one return. Each is handled on its own event id; the first BUYER_FAVOUR refunds, later ones find REFUNDED and acknowledge.

## Open interface assumptions

- `DisputeResolved` is `platform.engagement.v1.DisputeResolved` on topic `engagement.events`, key = dispute id, defined by `q6-trust-contract` and produced by `engagement-dispute-escalation`.
- q10 provides `OrderReturn` read fields and the return auto-approve; this slice reads only return status, order id, refund amount and the order's `paid_at`.

## Clarifications

### Round 1 — 2026-10-16

- A17 [trust] Dispute refund: Rejected dispute moves no return and no money: which dispute statuses reach team-order and which carry a ruling? → **OPEN→INVESTIGATING, OPEN→REJECTED (spam), INVESTIGATING→{RESOLVED, REJECTED}; a ruling with any status other than RESOLVED is INVALID_ARGUMENT in team-engagement, so a REJECTED event is a no-op here** (human)
- A18 [trust] Dispute refund: Second ruling event on a refunded return is acknowledged: can one return be disputed again after its dispute closed? → **Yes, while can_escalate is true; each ruling event is handled on its own event id and a return is refunded at most once** (human)
- M2 [money] Dispute refund: Cash-on-delivery return stops at approved: is a COD return refunded by this slice? → **No; COD refunds are a follow-up, the return stops at APPROVED and does not block auto-complete** (human)
- M3 [money] Dispute refund: Already approved return is refunded without a second approval: is an APPROVED return the seller never refunds auto-refunded? → **No auto-refund (follow-up); only a buyer-favour dispute ruling refunds it here** (human)
- NEEDS_HUMAN [privacy] Dispute refund: Audit event names the resolving admin: lawful basis for the dispute audit metadata (`actor_id`, `order_id`, `dispute_id`, `ruling`, `from_status`) → **DPO sign-off required before release (recorded, not blocking the build)** (human)
