## Why

A buyer whose return was rejected, or ignored by the seller, has no recourse today: `EngagementService.CreateDispute` only knows an order, nothing links a dispute to a return, an admin ruling moves no return and no money, `ResolveDispute` lets a dispute jump `OPEN` to `RESOLVED`, and there is no list an admin can work from. The initiative `q6-trust-enforcement` (Locked 2026-10-16, stories 6 and 7) moves the return-dispute half of Q10 here. This slice is the team-engagement side: escalate a return to a dispute, tighten the dispute state machine, record a ruling, list disputes, and emit `DisputeResolved` for team-order (slice `order-dispute-refund`) to refund the return.

initiative: q6-trust-enforcement

This slice builds only after `q10-order-lifecycle` has merged to main. It consumes `OrderReturn.can_escalate` (REJECTED, or PENDING with no seller answer past the q10 threshold, inside the q10 return window), unchanged, and needs q10 `order-receipt-completion` for the return window. It also follows `engagement-listing-reports` (same handler, service, migration, relayer and gateway-forwarder files).

## What Changes

- **Migration** (number follows `engagement-listing-reports`): `disputes` gains `return_id` (nullable text) and `ruling` (text, empty default); partial unique index on `return_id` where status is `OPEN` or `INVESTIGATING` (one open dispute per return); index on `(status, created_at)` for the admin list. Reversible down file.
- **team-engagement upstream** (`internal/upstream/order.go`): new `GetReturn(ctx, returnID)` on `OrderClient` calling `OrderService.GetReturnRequest` with the existing service principal (`service-team-engagement`, scope `order.read`), bounded by `UPSTREAM_CALL_TIMEOUT_SECONDS`. No new env var (`UPSTREAM_ORDER_ADDR` already exists).
- **CreateDispute**: optional `return_id`. After the existing scope, validation and order-party checks, the handler reads the return, requires it to belong to the order and to have `can_escalate` true, then creates the dispute linked to it. Errors: `INVALID_ARGUMENT` (a `defendant_id` other than the order's seller, checked before the return read), `FAILED_PRECONDITION` (not escalatable), `ALREADY_EXISTS` (open dispute on the return), `NOT_FOUND` (unknown return or return of another order), `UNAVAILABLE` (team-order down, fail closed). Without `return_id` the RPC behaves as today.
- **ResolveDispute**: state machine `OPEN -> INVESTIGATING -> {RESOLVED, REJECTED}`, `OPEN -> REJECTED` (spam) and `INVESTIGATING -> INVESTIGATING` allowed; new `ruling` (`DISPUTE_RULING_BUYER_FAVOUR` / `SELLER_FAVOUR`), required for RESOLVED on a return-linked dispute; a ruling with any other status is `INVALID_ARGUMENT`. Status change is a compare-and-set on the previous status. The existing `dispute.resolve` audit event (written first, fail closed) gains metadata `ruling` and `return_id`. When the dispute reaches RESOLVED or REJECTED, one `DisputeResolved` fact is written to the engagement outbox in the same transaction as the status update (relayed to `engagement.events`, key = dispute id).
- **New RPCs**: `ListOrderDisputes` (order buyer, order seller or admin; newest first) and `ListDisputes` (admin; status filter; oldest first; paginated; not audited).
- **team-order task** (inside this slice because q10 does not provide it): `OrderService.GetReturnRequest` also accepts a service principal holding `order.read` (read only; `UpdateReturnStatus` is unchanged and still refuses it). New capability `return-read-access`.
- **team-gateway tasks**: forward `ListOrderDisputes` (signed-in users) and `ListDisputes` (listed in `adminProcedures`), pass `return_id` / `ruling` through the existing `CreateDispute` / `ResolveDispute` forwarders, update `policy_edge_test.go` pinned set and `unrouted_test.go`.
- **Docs**: remove the "Dispute state machine is loose" Known-gaps line from `team-engagement/README.md`; document `ListOrderDisputes`, `ListDisputes` and the return link in its RPC table.
- **Behaviour change for existing callers**: `OPEN -> RESOLVED` is now refused (`FAILED_PRECONDITION`); `OPEN -> REJECTED` stays allowed. Existing unit tests, `engagement/dispute_audit.feature` and the dispute steps in `disputes.feature` that resolve an `OPEN` dispute directly must go through `INVESTIGATING` first.
- Proto, `DisputeResolved` and the audit catalogue come from `q6-trust-contract`; this slice adds none. No new dependency.

## NFR

- Latency: `CreateDispute` with `return_id` adds one team-order call, bounded by `UPSTREAM_CALL_TIMEOUT_SECONDS` (default <= 2 s), no retry. `ListDisputes` page <= 200 ms at 10k disputes (index above). `ListOrderDisputes` one indexed query (`order_id`) plus one team-order call for non-admins.
- Availability: team-order down blocks only `CreateDispute` with `return_id` and `ListOrderDisputes` for non-admins (`UNAVAILABLE`). team-audit down blocks only `ResolveDispute` (existing behaviour). Kafka down delays `DisputeResolved` (outbox keeps it; relayed on recovery); the ruling itself is not blocked.
- Limits: `reason` and `resolution` limits unchanged; `ListDisputes` page size defaults to 20 and is clamped to 100.
- Delivery: `DisputeResolved` is at-least-once and keyed by dispute id; consumers dedupe on envelope event id. Relay lag <= 2 s at default poll interval.
- Concurrency: two simultaneous resolutions of one dispute produce one status change, one event; one open dispute per return holds under concurrent creates.

## Data classification

| Field | Class | Purpose | Basis | Retention |
|---|---|---|---|---|
| `disputes.return_id` | pseudonymous identifier | link the dispute to the return it escalates | contract performance (order dispute handling) | with the dispute row (existing) |
| `disputes.ruling` | decision metadata | record the admin's decision | same | with the dispute row |
| audit `metadata.ruling`, `metadata.return_id`, `actor_id` (admin id) | pseudonymous ids / staff identifier | accountability of who ruled | legal obligation / legitimate interest (Decree 13/2023 processing records) | team-audit policy (immutable) |
| `DisputeResolved` payload | ids, status, ruling, resolver id, timestamp | trigger the refund in team-order | same | outbox rows deleted 7 days after publish (existing); Kafka topic retention |

No new free text. Existing `reason`, `evidence_urls`, `resolution` are unchanged, are not copied into events, and the resolution note stays out of logs. The claimant and defendant ids are not in `DisputeResolved`.

## Threat model

- **Assets:** buyer refunds (money), seller standing, dispute rulings, the audit trail, the service principal for team-order.
- **Abuse cases:**
  - Escalating a return that is not escalatable (still inside the seller's answer time, approved, window closed): `can_escalate` is read from team-order on every create, never trusted from the request; `FAILED_PRECONDITION`.
  - Escalating another buyer's return or order: order-party check first (`NOT_FOUND`, as today); the return must belong to the order.
  - Flooding disputes on one return: one open dispute per return enforced by a partial unique index, not only by a pre-check (`ALREADY_EXISTS`).
  - A buyer or seller ruling in their own favour: `ResolveDispute` stays admin only; the ruling is an admin field; the actor in the audit event and in `DisputeResolved.resolver_id` comes from the verified Principal.
  - Forcing a refund without a trail: audit is written first and fails closed; the refund itself is team-order's audited consumer. A ruling cannot be applied twice (compare-and-set, one event).
  - Skipping investigation: `OPEN -> RESOLVED` refused (only spam may be rejected straight from `OPEN`, and that moves no money).
  - Reading other users' disputes: `ListOrderDisputes` is party or admin; `ListDisputes` admin only (edge and service both check). Non-parties and unknown orders both get `PERMISSION_DENIED`, so order ids cannot be probed.
  - Service principal misuse: team-order accepts `order.read` for reads only; `UpdateReturnStatus` still refuses it.
- **Authz rule:** `CreateDispute` needs scope `engagement:write` and being the order's buyer; `ResolveDispute` and `ListDisputes` need `admin`; `ListOrderDisputes` needs `engagement:read` and being the order's buyer or seller, or `admin`; team-order `GetReturnRequest` needs the return's buyer or seller, an admin, or a service principal with `order.read`.
