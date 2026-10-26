# order-dispute-refund Specification

## Purpose
Audited refund of a return when a dispute is ruled in the buyer's favour.

## Requirements

### Requirement: A buyer-favour dispute ruling refunds the return

The team-order dispute consumer SHALL, on a `DisputeResolved` event with `status=RESOLVED`, `ruling=BUYER_FAVOUR` and a non-empty `return_id`, move the return to APPROVED (from PENDING or REJECTED; an APPROVED return is left as it is) and, when the return's order has `paid_at` set, move it on from APPROVED to REFUNDED with the existing compare-and-set that writes one `ReturnRefunded` outbox row. For an order without `paid_at` (cash on delivery) the consumer SHALL stop at APPROVED. The refund amount SHALL be the return's stored `refund_amount`; no amount is read from the event.

#### Scenario: Dispute refund: Rejected return of an online-paid order is refunded
- **WHEN** a `DisputeResolved` event (RESOLVED, BUYER_FAVOUR) names a REJECTED return of a Paid-online order whose remaining returnable amount covers the return
- **THEN** the return ends REFUNDED and exactly one `ReturnRefunded` outbox row exists for it carrying the return's stored refund amount
- **VERIFIED BY**: team-order `internal/consumer` `TestDisputeBuyerFavourRejectedReturnRefunded`

#### Scenario: Dispute refund: Pending escalated return of an online-paid order is refunded
- **WHEN** a BUYER_FAVOUR event names a PENDING return of a Paid-online order
- **THEN** the return ends REFUNDED and exactly one `ReturnRefunded` outbox row exists for it
- **VERIFIED BY**: team-order `internal/consumer` `TestDisputeBuyerFavourPendingReturnRefunded`

#### Scenario: Dispute refund: Already approved return is refunded without a second approval
- **WHEN** a BUYER_FAVOUR event names an APPROVED return of a Paid-online order
- **THEN** the return ends REFUNDED with one `ReturnRefunded` outbox row and no PENDING or REJECTED transition is attempted
- **VERIFIED BY**: team-order `internal/consumer` `TestDisputeBuyerFavourApprovedReturnRefunded`

#### Scenario: Dispute refund: Cash-on-delivery return stops at approved
- **WHEN** a BUYER_FAVOUR event names a REJECTED return of an order with no `paid_at`
- **THEN** the return ends APPROVED and zero `ReturnRefunded` outbox rows exist for it
- **VERIFIED BY**: team-order `internal/consumer` `TestDisputeBuyerFavourCODStopsAtApproved`

#### Scenario: Dispute refund: Return already refunded is left unchanged
- **WHEN** a BUYER_FAVOUR event names a return that is already REFUNDED
- **THEN** the event is acknowledged, the return stays REFUNDED, the outbox still holds exactly one `ReturnRefunded` row and zero audit calls are made
- **VERIFIED BY**: team-order `internal/consumer` `TestDisputeBuyerFavourAlreadyRefundedNoop`

#### Scenario: Dispute refund: Ruling reaches the refund end to end
- **WHEN** an admin resolves a dispute linked to a REJECTED return of a Paid-online order with ruling BUYER_FAVOUR through the gateway
- **THEN** within 15 s the buyer's `GetReturnRequest` shows REFUNDED and the order's payment shows one refund of the return amount
- **VERIFIED BY**: platform-e2e `features/order/dispute_refund.feature` "Dispute refund: Ruling reaches the refund end to end"

### Requirement: The dispute refund is audited before any change

For every `DisputeResolved` event that would change a return, the consumer SHALL write one team-audit event before the first change, with `action=return.dispute_refund`, `target_type=return_request`, `target_id` equal to the return id, `actor_id` equal to the event's `resolver_id`, and `metadata` keys `order_id`, `dispute_id`, `ruling` and `from_status` (the return status name read before the change). If the audit write errors, times out or no audit client is configured, the consumer SHALL return a transient error and SHALL NOT change the return or emit any event. The consume loop SHALL retry a transient error for the same record until it succeeds, with a per-attempt delay capped at `MaxBackoff` (default 5 s), SHALL NOT commit the record's offset meanwhile and SHALL NOT route it to the DLQ; only an error wrapping `ErrPermanent` goes to the DLQ.

#### Scenario: Dispute refund: Audit event names the resolving admin
- **WHEN** a BUYER_FAVOUR event with `resolver_id`="admin-1", `dispute_id`="d-1" names a REJECTED return "r-1" of order "o-1"
- **THEN** exactly one audit call is made before the first return change, with action `return.dispute_refund`, target `return_request`/"r-1", actor "admin-1" and metadata `order_id`="o-1", `dispute_id`="d-1", `ruling`="DISPUTE_RULING_BUYER_FAVOUR", `from_status`="RETURN_STATUS_REJECTED"
- **VERIFIED BY**: team-order `internal/consumer` `TestDisputeRefundWritesAuditBeforeChange`

#### Scenario: Dispute refund: Audit unavailable leaves the return unchanged
- **WHEN** a BUYER_FAVOUR event arrives while the audit writer returns an error
- **THEN** the handler returns a non-permanent error, the return keeps its status, no `ReturnRefunded` outbox row is written and the event id is not marked processed
- **VERIFIED BY**: team-order `internal/consumer` `TestDisputeRefundAuditDownNoChangeRetried`

#### Scenario: Dispute refund: Missing audit client refuses the change
- **WHEN** the consumer runs without an audit writer and receives a BUYER_FAVOUR event
- **THEN** the handler returns a non-permanent error and the return keeps its status
- **VERIFIED BY**: team-order `internal/consumer` `TestDisputeRefundNilAuditWriterNoChange`

#### Scenario: Dispute refund: Recovery after audit comes back
- **WHEN** team-audit is stopped, an admin resolves a BUYER_FAVOUR dispute on a REJECTED return of a Paid-online order, 20 s later the return is read as still REJECTED, then team-audit is started
- **THEN** within 30 s of team-audit starting the return is REFUNDED, exactly one `return.dispute_refund` event exists for it with the admin's id as actor, and the `engagement.events.order.dlq` topic holds no record for the dispute
- **VERIFIED BY**: platform-e2e `features/order/dispute_refund.feature` "Dispute refund: Recovery after audit comes back" (tag `@destructive`)

#### Scenario: Dispute refund: Audit outage longer than the retry window is not dead-lettered
- **WHEN** `Run` handles a BUYER_FAVOUR record and the audit writer returns an error on the first 20 attempts, then succeeds
- **THEN** zero records are produced to the DLQ, the offset is not committed before the successful attempt and is committed exactly once after it, every retry delay is at most `MaxBackoff` (5 s by default), and the return ends REFUNDED
- **VERIFIED BY**: team-order `internal/consumer` `TestDisputeRunRetriesTransientWithoutDLQ`

#### Scenario: Dispute refund: Event without resolver is a permanent error
- **WHEN** a BUYER_FAVOUR event arrives with an empty `resolver_id`
- **THEN** the handler returns an error wrapping `ErrPermanent`, zero audit calls are made and the return is unchanged
- **VERIFIED BY**: team-order `internal/consumer` `TestDisputeRefundMissingResolverPermanent`

### Requirement: Re-approving a rejected return re-checks the returnable remainder

`ReapproveReturn` SHALL move a REJECTED return to APPROVED in one transaction that locks the order row, sums the `refund_amount` of the order's returns whose status is not REJECTED, and succeeds only when that sum plus the return's `refund_amount` does not exceed the order total. When it does not fit, the return SHALL stay REJECTED and the consumer SHALL fail the event with `ErrPermanent`, log return id, dispute id and reason, and route the record to the DLQ.

#### Scenario: Dispute refund: Rejected return that still fits is re-approved
- **WHEN** `ReapproveReturn` runs for a REJECTED return of 400000 VND on an order of 1000000 VND whose other non-rejected returns total 600000 VND
- **THEN** the return becomes APPROVED
- **VERIFIED BY**: team-order `internal/repository` `TestReapproveReturnFitsBoundary`

#### Scenario: Dispute refund: Rejected return that no longer fits stays rejected
- **WHEN** `ReapproveReturn` runs for a REJECTED return of 400000 VND on an order of 1000000 VND whose other non-rejected returns total 700000 VND
- **THEN** the call returns `ErrReturnExceedsRemainder` and the return stays REJECTED
- **VERIFIED BY**: team-order `internal/repository` `TestReapproveReturnExceedsRemainderStaysRejected`

#### Scenario: Dispute refund: Non-fitting return is dead-lettered for CS
- **WHEN** a BUYER_FAVOUR event names a REJECTED return that no longer fits the order's remainder
- **THEN** the handler returns an error wrapping `ErrPermanent`, the return stays REJECTED, no `ReturnRefunded` row is written, and `Run` produces the record to the DLQ topic once
- **VERIFIED BY**: team-order `internal/consumer` `TestDisputeRefundNoRemainderDeadLettered`

#### Scenario: Dispute refund: Re-approval is not reachable from the status RPC
- **WHEN** a seller or an `order.admin` caller calls `UpdateReturnStatus` to APPROVED on a REJECTED return
- **THEN** the call returns `FailedPrecondition` and the return stays REJECTED
- **VERIFIED BY**: team-order `internal/handler` `TestUpdateReturnStatusRejectedStaysTerminal`

#### Scenario: Dispute refund: Two returns cannot both be re-approved past the total
- **WHEN** two `ReapproveReturn` calls for two different REJECTED returns of the same order run concurrently and only one fits the remainder
- **THEN** exactly one returns success and the other returns `ErrReturnExceedsRemainder`
- **VERIFIED BY**: team-order `internal/repository` `TestReapproveReturnConcurrentSerialised` (Postgres)

### Requirement: Rulings that do not favour the buyer move nothing

The consumer SHALL acknowledge without an audit call, a return change or an outbox row any `DisputeResolved` event whose `status` is REJECTED, whose `ruling` is not BUYER_FAVOUR, whose `return_id` is empty, or whose type is not `platform.engagement.v1.DisputeResolved`.

#### Scenario: Dispute refund: Seller-favour ruling moves no return and no money
- **WHEN** a `DisputeResolved` event (RESOLVED, SELLER_FAVOUR) names a REJECTED return
- **THEN** the event is acknowledged, the return stays REJECTED, zero audit calls and zero outbox rows are written
- **VERIFIED BY**: team-order `internal/consumer` `TestDisputeSellerFavourNoop`

#### Scenario: Dispute refund: Rejected dispute moves no return and no money
- **WHEN** a `DisputeResolved` event with `status=REJECTED` and a BUYER_FAVOUR ruling field names a return
- **THEN** the event is acknowledged and the return, the audit calls and the outbox are unchanged
- **VERIFIED BY**: team-order `internal/consumer` `TestDisputeRejectedStatusNoop`

#### Scenario: Dispute refund: Dispute without a return is ignored
- **WHEN** a RESOLVED, BUYER_FAVOUR event arrives with an empty `return_id`
- **THEN** the event is acknowledged and nothing is audited or changed
- **VERIFIED BY**: team-order `internal/consumer` `TestDisputeNoReturnIdNoop`

#### Scenario: Dispute refund: Other event types are ignored
- **WHEN** an envelope of type `platform.engagement.v1.ReviewCreated` reaches the consumer
- **THEN** it returns nil and writes no dedupe row
- **VERIFIED BY**: team-order `internal/consumer` `TestDisputeConsumerIgnoresOtherTypes`

#### Scenario: Dispute refund: Seller-favour ruling leaves the return untouched end to end
- **WHEN** an admin resolves a dispute linked to a REJECTED return with ruling SELLER_FAVOUR through the gateway
- **THEN** after 10 s the return is still REJECTED and the audit log holds no `return.dispute_refund` event for it
- **VERIFIED BY**: platform-e2e `features/order/dispute_refund.feature` "Dispute refund: Seller-favour ruling leaves the return untouched end to end"

### Requirement: The dispute refund is idempotent and race-safe

The consumer SHALL record each processed `event_id` in `processed_events` under consumer name `team-order.dispute` only after its effect is applied, SHALL treat a redelivered event id as a no-op, and SHALL apply every return transition as a compare-and-set so that one return is refunded at most once however many events, retries or seller actions reach it.

#### Scenario: Dispute refund: Redelivered event refunds once
- **WHEN** the same BUYER_FAVOUR envelope (same event id) is delivered twice
- **THEN** the return is REFUNDED, exactly one `ReturnRefunded` outbox row and exactly one audit call exist
- **VERIFIED BY**: team-order `internal/consumer` `TestDisputeRedeliveryRefundsOnce`

#### Scenario: Dispute refund: Second ruling event on a refunded return is acknowledged
- **WHEN** two BUYER_FAVOUR events with different event ids name the same return
- **THEN** the return has exactly one `ReturnRefunded` outbox row and the second event is acknowledged with no audit call
- **VERIFIED BY**: team-order `internal/consumer` `TestDisputeSecondEventSameReturnRefundsOnce`

#### Scenario: Dispute refund: Auto-approve winning the race does not fail the event
- **WHEN** a BUYER_FAVOUR event names a PENDING return and the PENDING to APPROVED compare-and-set loses to a concurrent approval
- **THEN** the consumer re-reads the return, continues from APPROVED, and the return ends REFUNDED with one `ReturnRefunded` row
- **VERIFIED BY**: team-order `internal/consumer` `TestDisputePendingCASLostContinues`

#### Scenario: Dispute refund: Seller refund winning the race is acknowledged
- **WHEN** a seller moves an APPROVED return to REFUNDED between the consumer's read and its APPROVED to REFUNDED write
- **THEN** the consumer acknowledges the event, and exactly one `ReturnRefunded` outbox row exists for the return
- **VERIFIED BY**: team-order `internal/consumer` `TestDisputeRefundCASLostAcknowledged`

#### Scenario: Dispute refund: Unknown return is a permanent error
- **WHEN** a BUYER_FAVOUR event names a return id that does not exist
- **THEN** the handler returns an error wrapping `ErrPermanent`, zero audit calls are made and the record goes to the DLQ
- **VERIFIED BY**: team-order `internal/consumer` `TestDisputeUnknownReturnPermanent`

#### Scenario: Dispute refund: Malformed envelope is dead-lettered
- **WHEN** a record that is not a valid `EventEnvelope`, or whose payload is not a `DisputeResolved`, reaches `HandleRaw`
- **THEN** the handler returns an error wrapping `ErrPermanent` and nothing is audited or changed
- **VERIFIED BY**: team-order `internal/consumer` `TestDisputeMalformedRecordPermanent`

### Requirement: The consumer is configured and wired like the payment consumer

team-order SHALL read `ENGAGEMENT_EVENTS_TOPIC` (default `engagement.events`), `ORDER_DISPUTE_CONSUMER_GROUP` (default `team-order.dispute`) and `ENGAGEMENT_EVENTS_DLQ_TOPIC` (default `engagement.events.order.dlq`), and SHALL start the dispute consumer only when Kafka is enabled and an audit client exists, with offsets committed only after a record is applied or dead-lettered.

#### Scenario: Dispute refund: Config exposes the consumer settings
- **WHEN** team-order loads settings with none of the three variables set, then with all three set
- **THEN** the defaults are `engagement.events`, `team-order.dispute` and `engagement.events.order.dlq`, and the set values replace them
- **VERIFIED BY**: team-order `internal/config` `TestKafkaEngagementConsumerSettings`

#### Scenario: Dispute refund: Consumer does not start without an audit client
- **WHEN** team-order starts with Kafka enabled and `UPSTREAM_AUDIT_ADDR` empty
- **THEN** the dispute consumer is not started and the startup log names the reason
- **VERIFIED BY**: team-order `internal/bootstrap` `TestDisputeConsumerSkippedWithoutAudit`

#### Scenario: Dispute refund: Offset is not committed when the DLQ write fails
- **WHEN** a record fails permanently and the DLQ producer returns an error
- **THEN** `Run` does not commit the record's offset
- **VERIFIED BY**: team-order `internal/consumer` `TestDisputeRunNoCommitWhenDLQFails`

#### Scenario: Dispute refund: Return linked to another order is refused
- **WHEN** a BUYER_FAVOUR event carries an `order_id` that differs from the stored return's order id
- **THEN** the handler returns an error wrapping `ErrPermanent`, zero audit calls are made and the return is unchanged
- **VERIFIED BY**: team-order `internal/consumer` `TestDisputeOrderMismatchPermanent`
