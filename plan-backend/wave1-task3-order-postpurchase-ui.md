# [W1-T3] Order post-purchase UI: tracking timeline + returns/refunds (team-frontend)

## Role
SE

## Objective
Surface the already-built order post-purchase backend in the UI: on the buyer order detail page, show a shipment/status timeline and a "request return/refund" flow with status. Thin UI, no polish. Typecheck + colocated tests green.

## Write-set (EXCLUSIVE)
- team-frontend/src/lib/gateway/orders.ts — add wrappers: getShipmentTracking, getSagaState (timeline), createReturnRequest, getReturnRequest, updateReturnStatus (edit; append only, do not change existing signatures)
- team-frontend/src/lib/gateway/payment.ts — add wrapper: refundPayment (edit; append only)
- team-frontend/src/app/account/orders/** — mount timeline + returns section on the [id] detail page (edit)
- team-frontend/src/features/order/** — new components (OrderTimeline, ReturnRequestForm/Status) + colocated tests (create)

## Read-only dependencies
- Generated stubs: src/generated/platform/order/*, payment/* (regenerated Wave 0)
- Reference: src/features/review/ReviewSection.tsx + src/lib/gateway/reviews.ts (server-action + gateway-call shape); existing orders.ts functions (getOrder etc.).

## Contracts you consume (already in gateway + backend)
- order/v1: `GetShipmentTracking` (Shipment{status, checkpoints[]}), `GetSagaState` (SagaStep[]), `CreateReturnRequest`/`GetReturnRequest`/`UpdateReturnStatus` (OrderReturn{status, refund_amount, reason})
- payment/v1: `RefundPayment` (mock)

## Acceptance criteria
- [ ] Order detail page renders a timeline from GetShipmentTracking checkpoints (and/or GetSagaState steps) — loading + empty handled simply
- [ ] Buyer can submit a return request (reason + amount) and see its status; refund is mock (no real money)
- [ ] gateway wrappers are server-only, build per-request client from session cookie (mirror reviews.ts)
- [ ] `npm run typecheck` clean; `npm run test -- src/features/order` green (≥3 component/action tests)

## Review (different agent)
SE rubric. Touches gateway modules (service boundary via edge) → contract-boundary-reviewer; require CLEAN.

## Verify
cd team-frontend && npm run typecheck && npm run test -- src/features/order

## Out of scope
- Do NOT touch src/lib/gateway/engagement.ts, listing routes, or layout.tsx (other tasks / integration).
- Do NOT add real payment logic. Do NOT redesign the order page — minimal sections only.
- Do NOT edit team-gateway or any backend repo.
