# [W1-T1] team-search: exactly-once consumer + version guard

## Role
DE  # Kafka consumer / read-model — inherits idempotent-re-run + no-loss criteria

## Objective
The search consumer never loses an event on a sink error and never resurrects a
tombstoned listing. Offsets advance only after successful indexing; out-of-order
events are rejected by a version guard.

## Write-set (EXCLUSIVE)
- team-search/internal/consumer/consumer.go        (edit)
- team-search/internal/consumer/listing.go         (edit)
- team-search/internal/index/opensearch.go         (edit)
- team-search/internal/consumer/*_test.go          (create/edit)
- team-search/internal/index/*_test.go             (create/edit)

## Read-only dependencies
- platform/events/v1 EventEnvelope (event_id, occurred_at / outbox seq = version).
- team-domain relayer park-after-max-attempts pattern (mirror it; do not import).

## Contracts
- AD1: commit offset ONLY after handler success; on error → bounded exp-backoff
  retry (max N), then produce record to `listing.events.dlq`, then advance.
- AD2: OpenSearch doc carries `version` (int64 from outbox seq / occurred_at).
  Apply update only if `incoming.version >= stored.version`. Partial updates must
  NOT `doc_as_upsert` a tombstoned id.

## Acceptance criteria
- [ ] Test: handler error on record R → offset does NOT advance past R; after max retries R lands in `.dlq` and only then offset advances. (reproduces C1: today it advances on error)
- [ ] Test: a stale/reordered partial-update event for a deleted listing does NOT recreate the doc, and an older-version event does NOT overwrite newer state. (reproduces H5)
- [ ] `go test ./...` green in Docker.

## Review
contract-boundary-reviewer (CQRS read-model / broker boundary). Rubric: DE section.

## Verify
docker compose -f docker-compose.services.yaml run --rm team-search go test ./...

## Out of scope
- No search ranking/relevance changes. No proto changes. Only team-search/.
- Do not change the producer (team-domain) — assume at-least-once with stable event_id.
