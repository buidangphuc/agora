# Design

## Context

team-engagement already has disputes (`internal/service/dispute.go`, `internal/repository/dispute.go`), an audit-first `ResolveDispute`, an outbox with relayer (`internal/events`), and an `OrderClient` using the service principal `service-team-engagement` with `order.read`. team-order's `GetReturnRequest` currently allows only the return's buyer or seller and admins (`isAdminOrUser`), unlike `GetOrder`, which also allows a service principal (`canViewOrder`).

## Decisions

- **Check order in CreateDispute**: scope, field validation, order parties (existing, `NOT_FOUND` for foreign orders), then return (`GetReturn`), then insert. A foreign order never reaches team-order's return read, so return ids of other buyers cannot be probed.
- **Return must belong to the order**: compare the returned `order_id` with the request; a mismatch is `NOT_FOUND` (same as unknown), not `INVALID_ARGUMENT`, to avoid a probing oracle.
- **One open dispute per return**: partial unique index is the guard; the service maps the unique violation to `ALREADY_EXISTS`. A pre-check alone races.
- **Compare-and-set on resolve**: `UPDATE disputes SET status=$new ... WHERE id=$1 AND status=$prev`; zero rows means someone else moved it, mapped to `FAILED_PRECONDITION`. Status update and `DisputeResolved` outbox row share one transaction (`enqueueFact`), so the event exists iff the status moved.
- **Order of operations in ResolveDispute**: authz, validation (status, ruling rules), load, state-machine check, audit write (fail closed), compare-and-set update plus outbox insert. Refusals before the audit step write no event, as in `engagement-dispute-audit`.
- **State machine** (A17): `OPEN -> INVESTIGATING`, `OPEN -> REJECTED` (spam), `INVESTIGATING -> INVESTIGATING` (note update), `INVESTIGATING -> {RESOLVED, REJECTED}`; everything else `FAILED_PRECONDITION`. `OPEN -> REJECTED` writes the audit event and one `DisputeResolved` (REJECTED, no ruling).
- **Defendant on escalation**: with `return_id`, a `defendant_id` other than the order's seller is `INVALID_ARGUMENT` (contract rule), checked after the order-party read and before the return read; the no-return path keeps today's `PERMISSION_DENIED`.
- **Ruling rules**: ruling is meaningful only with status RESOLVED. Return-linked + RESOLVED requires a ruling. A ruling on any other status is `INVALID_ARGUMENT`. A non-return dispute may carry a ruling; it only changes the event and record.
- **ListOrderDisputes authorization** uses the order's parties from team-order (`GetOrderParties`) instead of "caller appears in some dispute", so the answer does not reveal whether a dispute exists. Unknown order and foreign order are both `PERMISSION_DENIED`. Admin skips the upstream call.
- **No team-order dependence on dispute creation**: team-order only learns of a dispute through `DisputeResolved` (slice `order-dispute-refund`).
- **team-order read access**: extend `GetReturnRequest` with the same service-principal rule `canViewOrder` uses (type service + scope `order.read`). `UpdateReturnStatus` keeps `isAdminOrUser` on the seller only.

## Risks

- Existing callers and tests that resolve an `OPEN` dispute directly break by design; the tasks list them.
- If q10 changes the semantics of `can_escalate` or the field number, this slice follows the merged q10 `order.proto`; `engagement.proto` field numbers are owned by `q6-trust-contract` alone (q10 does not touch it).

## Clarifications

### Round 1 — 2026-10-16

- A17 [state machine] Dispute escalation: Open dispute can be rejected as spam: which transitions does ResolveDispute allow, and is a ruling on a status other than RESOLVED rejected or ignored? → **OPEN→INVESTIGATING, OPEN→REJECTED (spam), INVESTIGATING→{RESOLVED, REJECTED} (INVESTIGATING→INVESTIGATING note update kept); a ruling with any status other than RESOLVED is INVALID_ARGUMENT; the q6-trust-contract comment changes to match** (human)
- A18 [authz] Dispute escalation: Buyer and seller list the order's disputes newest first: may a return be re-escalated after its dispute closed, and who may run ListOrderDisputes? → **Re-escalation allowed while can_escalate is true; ListOrderDisputes is allowed to the order's buyer and seller and to admin (the contract and 00-spec change to match)** (human)
- X1 [ownership] Dispute edge: ListOrderDisputes carries the caller's principal: which slice owns the gateway forwarders for ListOrderDisputes and ListDisputes? → **engagement-dispute-escalation (task 8)** (human)
- X2 [contract] Dispute escalation: Buyer escalates a rejected return: does this slice coordinate engagement.proto field numbers with q10? → **No; q10 no longer touches engagement.proto and q6-trust-contract owns the numbers** (human)
