# [W1-T3] team-payment: emit PaymentSettled via outbox (no sync order call)

## Role
SE  # backend service

## Objective
On settle, team-payment persists a `PaymentSettled` event through a transactional
outbox and STOPS calling `order.UpdateOrderStatus` synchronously. Payment state
and the emitted event are written atomically.

## Write-set (EXCLUSIVE)
- team-payment/internal/service/payment.go        (edit — remove sync order call, write outbox)
- team-payment/internal/repository/**             (edit/create — outbox table + writer)
- team-payment/internal/events/**                 (create — relayer/publisher to payment.events)
- team-payment/migrations/**                       (create — outbox table)
- team-payment/internal/**/*_test.go              (create/edit)

## Read-only dependencies
- W0: `platform.payment.v1.PaymentSettled` (run `buf generate` in this repo to get it — codegen, never hand-edit generated/).
- team-domain outbox/relayer as the REFERENCE implementation to mirror.

## Reference implementation
Mirror team-domain's outbox: write (payment row + outbox row) in ONE tx; a relayer
publishes at-least-once with `event_id` = outbox row id; key = order_id; wrap in
EventEnvelope on topic `payment.events`.

## Contracts
- AD4: settle path = `tx { set payment=SETTLED; insert outbox(PaymentSettled) }`
  then relayer publishes. NO direct `order.UpdateOrderStatus` call remains.

## Acceptance criteria
- [ ] Test: settling a payment writes payment=SETTLED AND an outbox PaymentSettled row in one tx (rollback leaves neither). (reproduces H3 dual-write)
- [ ] Grep: no synchronous `UpdateOrderStatus` call from payment.go.
- [ ] `buf generate` produced PaymentSettled types; `go test ./...` green in Docker.

## Review
contract-boundary-reviewer (new broker producer + removes cross-service sync call). Rubric: SE section.

## Verify
docker compose -f docker-compose.services.yaml run --rm team-payment go test ./...

## Out of scope
- Do not implement the consumer (that's W2-T1 in team-order).
- No real payment logic (stays mock). Only team-payment/.
