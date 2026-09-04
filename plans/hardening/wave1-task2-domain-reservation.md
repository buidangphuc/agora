# [W1-T2] team-domain: idempotent reserve + TTL sweeper + drop legacy publisher

## Role
SE  # backend service — 3-test minimum, error paths covered

## Objective
Stock reservation is idempotent on `reservation_id`, expired reservations are
released automatically, and the non-idempotent legacy inline publisher is gone.

## Write-set (EXCLUSIVE)
- team-domain/internal/service/**                 (edit — reserve logic)
- team-domain/internal/repository/**              (edit — reservation persistence + TTL query)
- team-domain/internal/events/publisher.go        (edit/delete legacy inline path)
- team-domain/migrations/**                        (create — reservations table + expires_at if absent)
- team-domain/internal/**/*_test.go               (create/edit)

## Read-only dependencies
- ReserveStockRequest.reservation_id (existing proto field — now honored).
- team-domain outbox/relayer (keep as the ONLY publish path).

## Contracts
- AD5: reserve is a no-op if `reservation_id` already applied (return the prior
  result). A reservation row: `(reservation_id PK, listing_id, qty, expires_at)`.
- AD3 (domain side): a sweeper releases reservations past `expires_at` (default TTL
  e.g. 15m) and restores stock. Idempotent, re-runnable.
- AD6: delete `KafkaPublisher.produceEnvelope`'s random-`event_id` path; all
  publishing goes through the outbox + `BuildListingChangedEnvelope`.

## Acceptance criteria
- [ ] Test: reserve twice with the same `reservation_id` decrements stock ONCE. (reproduces M6)
- [ ] Test: a reservation past its TTL is released by the sweeper and stock restored.
- [ ] Test/grep: no code path mints a random event_id anymore.
- [ ] `go test ./...` green in Docker.

## Review
contract-boundary-reviewer (event publish path + cross-service reserve contract). Rubric: SE section.

## Verify
docker compose -f docker-compose.services.yaml run --rm team-domain go test ./...

## Out of scope
- Do not change team-order's caller (that's W2-T1, which SETS reservation_id).
- No listing/feature changes. Only team-domain/.
