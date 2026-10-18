## Why

q6-trust-enforcement (locked 2026-10-16, `plans/q6-trust-enforcement/00-spec.md`, story 7) lets an admin rule on a return-linked dispute in team-engagement. A buyer-favour ruling must refund the return, but team-order owns returns and money and team-engagement must not call it (AGENTS.md rule 5: a state change crosses services as a Kafka event). Today nothing in team-order consumes `engagement.events`, and a REJECTED return is terminal (`UpdateReturnStatus` refuses every move out of it), so a ruling has no path to money. This slice adds the team-order consumer for `DisputeResolved`. A buyer-favour ruling moves the return to APPROVED and, for an order paid online, to REFUNDED through the existing compare-and-set that writes the existing `ReturnRefunded` outbox row, so team-payment refunds once. The refund is a system money action: `return.dispute_refund` is written to team-audit before any return change, failing closed.

Builds only after `q10-order-lifecycle` has merged to main (return model: `can_escalate`, return window, return auto-approve actor `system`, migration numbering). Depends on `engagement-dispute-escalation` for the `DisputeResolved` proto and producer.

initiative: q6-trust-enforcement

## What Changes

- **team-order**
  - New consumer `internal/consumer/dispute.go` (`DisputeConsumer`), mirroring `consumer/payment.go`: `HandleRaw` / `HandleEnvelope`, dedupe on `EventEnvelope.event_id` in `processed_events` under consumer name `team-order.dispute`, `ErrPermanent` for poison records, the same `Run` loop and DLQ (`ENGAGEMENT_EVENTS_DLQ_TOPIC`, default `engagement.events.order.dlq`), with one difference: a transient error is retried until it succeeds (capped backoff, `MaxBackoff` default 5 s) and never dead-lettered; only `ErrPermanent` goes to the DLQ. It handles only type `platform.engagement.v1.DisputeResolved`; other types are ignored.
  - Decision table per event (after dedupe):

    | event | effect |
    |---|---|
    | `status=REJECTED`, or `ruling` != BUYER_FAVOUR, or `return_id` empty | acknowledged; no audit, no return change |
    | BUYER_FAVOUR on a return already REFUNDED | acknowledged; no audit, no change |
    | BUYER_FAVOUR, return APPROVED / PENDING / REJECTED | audit `return.dispute_refund`, then step below |

    - Steps after the audit: REJECTED returns are re-approved REJECTED to APPROVED; PENDING returns are approved PENDING to APPROVED; an APPROVED return is left. Then, only if the order has `paid_at` set (paid online), APPROVED to REFUNDED through `TransitionReturn`, which writes `ReturnRefunded` in the same transaction (event id deterministic per return, so the refund is requested once). A COD order stops at APPROVED, as the seller path does today (`ErrNotPaidOnline`).
  - Audit event: `action=return.dispute_refund`, `target_type=return_request`, `target_id` = return id, `actor_id` = `resolver_id` of the event (the admin who ruled; never a service id), `metadata`: `order_id`, `dispute_id`, `ruling` (`DISPUTE_RULING_BUYER_FAVOUR`), `from_status` (return status read before the change, e.g. `RETURN_STATUS_REJECTED`). Written through the existing `AuditClient` (principal `service-team-order`, scope `audit.write`) via the same `AuditWriter` seam as the handler. The consumer is built only when an audit client exists; without one it is not started (a buyer-favour event would otherwise never be consumable), and startup logs it.
  - Audit down, slow or write error: the handler returns a transient error (not `ErrPermanent`) before any change; the `Run` loop retries the same record with a delay capped at 5 s until the audit write succeeds (00-spec story 7: "the event is redelivered, no change"). The offset is not committed meanwhile, so the partition waits; no change is made and nothing is parked in the DLQ. Once team-audit is back, the next attempt (at most 5 s later) applies the refund.
  - Re-approval of a REJECTED return: new repository method `ReapproveReturn(ctx, id, orderTotal)` on `ReturnRepository` (Postgres and in-memory), one transaction: lock the order row `FOR UPDATE` (as `CreateReturnCapped`), sum `refund_amount` of the order's returns whose status is not REJECTED, and compare-and-set REJECTED to APPROVED only if `taken + refund_amount <= order total`; otherwise `ErrReturnExceedsRemainder`. The return keeps its `refund_amount`. This is the first transition out of REJECTED; it exists only on the dispute path (not in `UpdateReturnStatus`, so sellers and admins cannot use it).
  - Non-fitting remainder, return or order not found, missing `resolver_id` or `order_id`: `ErrPermanent`, so the record goes to the DLQ at once and an error log line carries return id, dispute id and reason, for CS to handle by hand. The return stays as it is. (If the audit event was already written for this attempt, the audit row stays: it records the attempt.)
  - Concurrency: every transition is a compare-and-set. A seller or `system` auto-approve that wins the PENDING to APPROVED race is not an error: the consumer re-reads and continues. A concurrent REFUNDED is acknowledged. A return refunded once is never refunded twice.
  - Config (`internal/config/config.go` `Kafka`): `EngagementTopic` (`ENGAGEMENT_EVENTS_TOPIC`, default `engagement.events`), `EngagementConsumerGroup` (`ORDER_DISPUTE_CONSUMER_GROUP`, default `team-order.dispute`), `EngagementDLQ` (`ENGAGEMENT_EVENTS_DLQ_TOPIC`, default `engagement.events.order.dlq`). Wiring in `cmd/server/main.go` and `internal/bootstrap/kafka.go` (a second franz-go reader and DLQ producer in the shape of `PaymentKafka`), only when `KAFKA_ENABLED`. README env table updated.
  - Proto: `proto/platform/engagement/v1/events.proto` (the `DisputeResolved` message) and `platform/audit/v1/audit.proto` copied from `platform-core/packages/proto` (never hand-edited), Go stubs regenerated.
  - No migration: dedupe uses the existing `processed_events` table, refund-once is the existing CAS plus deterministic outbox id. (If q10 leaves a change to `order_returns` that needs one, numbering follows q10's last migration.)
- **platform-e2e** (owned by this slice): `docker-compose.services.yaml` team-order env (`ENGAGEMENT_EVENTS_TOPIC`, `ENGAGEMENT_EVENTS_DLQ_TOPIC`) and the `engagement.events.order.dlq` topic in the redpanda init command; feature `features/order/dispute_refund.feature` with steps, driving the ruling through the gateway (`ResolveDispute` by the seeded admin) and reading the return through `GetReturnRequest`, the audit log through `support/audit_support.py` and the refund through the payment status.
- Deploy follow-up (not built here): a CS runbook for replaying `engagement.events.order.dlq` and a DLQ-depth alert.
- Not changed: `UpdateReturnStatus` and its transition table (seller path unchanged, still audited only for `order.admin`), `CreateReturnRequest`, team-payment (it already refunds on `ReturnRefunded`), team-engagement (producer and `GetReturnRequest` principal are `engagement-dispute-escalation`), gateway, frontend.

## NFR

- A buyer-favour ruling reaches REFUNDED (online-paid) once the event is published and the consumer reads it (transient errors are retried until applied, retry delay capped at 5 s); with the stack healthy, within 15 s of the ruling in e2e (outbox relay, consumer and team-payment included).
- Refund once per return id: CAS plus deterministic `ReturnRefunded` event id; redelivery of the same event id is a no-op, a second ruling event on the same return finds REFUNDED and acknowledges.
- Availability: team-audit down stops dispute refunds only (the dispute consumer waits on the record, retrying at most every 5 s, and resumes within 5 s of team-audit returning); seller refunds, checkout and every other order path are unaffected.
- Limits: one audit call per processed event; no in-process retry of the audit call itself (the loop retries the record).

## Data classification

| Field | Class | Purpose | Basis | Retention |
|---|---|---|---|---|
| audit `actor_id` (admin user id from `resolver_id`) | personal (staff identifier) | accountability for a money action | legal obligation / legitimate interest (Decree 13/2023 processing records) | team-audit policy (immutable) |
| audit metadata `order_id`, `dispute_id`, `from_status`, `ruling` | operational, non-personal | describe the action | same | same |
| `DisputeResolved` payload (ids, status, ruling) | operational | drive the refund | contract performance | consumed, not stored beyond `processed_events` event ids |

Not recorded: buyer or seller ids, dispute reason, evidence, resolution note, amounts, addresses. No new tables or columns.

## Threat model

- **Assets:** return status and the refund it triggers (money out of the platform), the audit trail, `service-team-order` audit credential, the `engagement.events` topic.
- **Abuse cases:**
  - Dispute refund with no accountable actor: audit written first, `actor_id` = resolver; no event, no change. Event without `resolver_id`: permanent error to DLQ, no change.
  - Operator stops team-audit to rule silently: fail closed; the refund waits (record retried, offset not committed) and applies, audited, only once team-audit is back.
  - Double refund (redelivery, two rulings, seller refund racing a ruling): dedupe ledger, CAS on every transition, deterministic `ReturnRefunded` id; the loser acknowledges.
  - Refunding beyond the order total (re-approving a REJECTED return after another return took the remainder): `ReapproveReturn` re-checks the remainder under the order row lock; on failure nothing changes and CS is alerted by the DLQ.
  - Forged `DisputeResolved` from a non-engagement producer: topic access is broker-level (same trust as `payment.events`); the consumer never takes an amount from the event, only ids; the amount is the stored `refund_amount`. A forged event for a return not on a dispute cannot be told apart at team-order (accepted; broker ACLs are out of scope; flagged in OPEN_ISSUES).
  - Seller or buyer using the re-approval path: it is not an RPC; `UpdateReturnStatus` still refuses every move out of REJECTED.
  - Replay of a stale event after the return was refunded by the seller: REFUNDED is acknowledged, no second refund, no audit row.
- **Authz rule:** no caller-facing RPC is added. The only trigger is an event with a verified envelope type; the audit actor is the resolving admin carried by the event, written with the service principal holding exactly `audit.write`.
