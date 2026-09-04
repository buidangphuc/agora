# [W0-T1] Foundation: PaymentSettled proto + ADRs + doc-drift

## Role
SA  # structural decisions; also SE for the proto edit

## Objective
The `platform.payment.v1.PaymentSettled` event exists in the contract and
regenerates cleanly; ADR-0007..0010 record the hardening decisions; AGENTS.md doc
drift is fixed. No service code changes here.

## Write-set (EXCLUSIVE)
- platform-core/packages/proto/platform/payment/v1/payment.proto            (edit)
- platform-core/docs/ADR/0007-durable-saga-compensation.md                  (create)
- platform-core/docs/ADR/0008-inventory-reservation-model.md                (create)
- platform-core/docs/ADR/0009-payment-order-event-integration.md            (create)
- platform-core/docs/ADR/0010-service-zero-trust.md                         (create)
- AGENTS.md                                                                 (edit)

## Read-only dependencies
- platform-core/packages/proto/platform/events/v1/events.proto (EventEnvelope)
- Existing ADR 0001-0006 for house style.

## Contracts you implement
Add to payment.proto (additive only — no field renumber):
```proto
message PaymentSettled {
  string payment_id = 1;
  string order_id   = 2;
  string buyer_id   = 3;
  PaymentStatus status = 4;   // reuse existing enum: SETTLED | FAILED
  string occurred_at = 5;     // RFC3339
}
```
Published on Kafka `payment.events`, key=order_id, wrapped in EventEnvelope.

## Acceptance criteria
- [ ] `make -C platform-core lint-proto` passes; `buf breaking` shows NO breaking change (additive).
- [ ] `make -C platform-core proto` regenerates without error.
- [ ] ADR-0007..0010 each follow the Decision/Context/Alternatives/Consequence shape of 0006.
- [ ] AGENTS.md §5 no longer lists a gateway `JWT_SECRET` (superseded by ADR-0006 JWKS_URL); §9 no longer says chat/orders/payment/AI are "not built yet" (they are).
- [ ] `python scripts/repo_doctor.py` still clean.

## Review
contract-boundary-reviewer (proto is the shared contract). Rubric: SA section of references/experts.md.

## Verify
make -C platform-core lint-proto && make -C platform-core proto

## Out of scope
- Do NOT wire any service to the new event (that's W1-T3 / W2-T1).
- Do NOT touch any team-* code or the old plan/.
