# [W1-T4] team-gateway: rate limiter eviction (bounded memory)

## Role
SE  # backend service

## Objective
The gateway rate limiter no longer grows unbounded and no longer silently
multiplies its effective limit across instances is documented. Per-key entries
expire; memory is bounded under many distinct keys.

## Write-set (EXCLUSIVE)
- team-gateway/internal/edge/interceptors.go       (edit)
- team-gateway/internal/edge/*_test.go             (create/edit)

## Read-only dependencies
- ADR-0010 (zero-trust / limiter note, from W0) for rationale only.

## Contracts
- Add TTL/LRU eviction to the per-key limiter map (idle keys removed). Keep the
  same limiter semantics for active keys. If a shared/Redis limiter is out of
  scope for now, add a `// TODO(ADR-0010): shared limiter when gateway scales out`
  and a code comment stating the per-instance limitation — do NOT add a new dep.

## Acceptance criteria
- [ ] Test: after inserting M distinct keys then idling past TTL, the map size is bounded (old keys evicted), not M-and-growing. (reproduces the unbounded-map finding)
- [ ] Test: an active key is still limited correctly (no regression).
- [ ] `go test ./...` green in Docker.

## Review
auth-scope-reviewer (edge interceptor path). Rubric: SE section.

## Verify
docker compose -f docker-compose.services.yaml run --rm team-gateway go test ./...

## Out of scope
- No new dependency (no Redis client). No auth/JWT changes. Only team-gateway/.
