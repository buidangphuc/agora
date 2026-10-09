## Context

`platform-recsys` writes nearline signals to Redis; `team-ai` reads them while serving. The spec named neither the key layout
nor the reader, and `team-ai` fed its ranker an `InMemoryNearlineStore()` that nothing wrote to, so the signals never reached
a request. This design fixes the contract both sides code against.

## Nearline Redis contract

Written by the platform-recsys nearline consumer (`recsys/nearline/signals.py`), read by team-ai (`RedisNearlineStore`).
Keys are in the same Redis DB as the serving cache (the stack's DB 0) unless team-ai is given `RECS_NEARLINE_REDIS_URL`.
Prefix `recs:nearline` (team-ai: `RECS_NEARLINE_PREFIX`). `<id>` is a listing id, `<actor>` a user id, or an anonymous/session id
when there is no user.

| Key | Type | Content | Written on |
|---|---|---|---|
| `recs:nearline:ctr:<listing_id>` | HASH | `clicks_ips`, `imprs_ips`: float sums of inverse-propensity weights (`position ** 0.5`, position clamped to >= 1) | `impression`, `view`, `click` events |
| `recs:nearline:user:<actor>:items` | ZSET | member = listing id, score = event timestamp (epoch seconds) | any event with an actor |
| `recs:nearline:user:<actor>:cats` | HASH | category -> weight (view/impression 1, click 2, add_to_cart 5) | events that carry a category |
| `recs:nearline:coview:<listing_id>` | ZSET | member = co-viewed listing id, score = co-view count | views in the same actor session (symmetric) |

Every key carries a TTL of the aggregation window (default 86 400 s), refreshed on each write. Values are plain decimal strings.

**What team-ai reads.** Only `recs:nearline:ctr:<listing_id>`, for the candidates of one request, in a single pipelined `HMGET`.
The debiased CTR is `min(1, clicks_ips / imprs_ips)`. A row is unusable, and the item keeps its prior `historical_ctr`
(`ctr_source = "fallback"`), when a field is missing or malformed, `imprs_ips <= 0`, or `imprs_ips < RECS_NEARLINE_MIN_IMPRESSIONS`
(default 1.0). A usable CTR of exactly 0 is treated as no signal by the ranker (its `> 0` rule), so an item that was shown a lot and
never clicked keeps its prior CTR rather than dropping to 0. Any Redis error or a read slower than `RECS_NEARLINE_TIMEOUT_MS`
(20 ms) leaves the request on prior CTRs; nearline never fails or degrades a request. The other three key families are not read by
team-ai yet.

**For the recsys side to check.** In the current code the Redis branch of `process_interaction` writes `ctr`, `items` and `cats` but
no `coview` keys (co-view linking exists only in the in-memory branch), so the `coview` row above is a contract the consumer still
has to honour. The recsys side owns the consumer; reconcile this table with it.

## Team-ai wiring

`build_recommendation_service` builds `RedisNearlineStore` when `RECS_NEARLINE_REDIS_URL` is set (empty = off) and the service reads
it once per request, for `ranking.model: gbdt` placements, into a snapshot that the ranker consumes. `explain` reports
`nearline_enabled` (a store is configured) and `nearline_hit_count` (candidates with usable data).

## Why the team-ai scenarios are checked at the gateway

The effect is observable at the edge: with two candidates whose model scores tie, the one with the higher nearline CTR ranks first in
`Recommend`. The e2e writes the hash exactly as the consumer would, so it checks the contract, not the consumer.
