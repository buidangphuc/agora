## Context

`NearlineSignalAggregator` (`platform-recsys/recsys/nearline/signals.py`) existed as a library with no event source.
This change adds the source (a Kafka consumer process) and fixes the aggregator's Redis path, which wrote recents,
categories and click-through counters but never co-views, and never bounded the recents list.

Reconciled with the archived AI-first changes (`recsys-generation-publish`, `serving-switch-atomicity`):

- Generation keys are `recs:v1:gen:<model_version>:*` plus the pointers `recs:v1:{serving,previous,model_version}`. Nearline
  state is not a model output: it is not stamped with a `model_version`, is not kept for rollback and is not pruned by
  `redis_cache.prune_generations` (which scans `recs:v1:gen:*` only). The two key spaces never overlap.
- The recsys job still trains only on the governed dataset (`DATASET_DIR`). The nearline consumer is not a training path:
  it reads `analytics.events` for a 24 hour online window and never feeds ALS.

## Decisions

### D1. A separate long-running process, not part of the batch job
`python -m recsys.nearline` (`recsys/nearline/__main__.py`). The image entrypoint stays `python -m recsys`; the compose
service overrides it. Consumer group `platform-recsys-nearline` (`NEARLINE_CONSUMER_GROUP`) is distinct from
team-analytics' `team-analytics` group, so both receive every event.

### D2. Wire decoding without generated code
Messages are a `platform.events.v1.EventEnvelope` wrapping a `platform.analytics.v1.TrackingEvent`. `recsys/nearline/wire.py`
reads the protobuf wire format directly (fields: envelope 1,2,3,4,7; principal 1,2; tracking 1,2,3,4,7,19) rather than
vendoring the contract and a protobuf runtime into the Spark image. `tests/test_nearline_wire.py` pins the field numbers
with bytes produced by the real protobuf runtime. Field numbers are additive-only in platform-core.

### D3. Delivery semantics
Manual offset commits every 100 messages and when idle; at-least-once. Redis errors propagate and the process exits 1
with the batch uncommitted. Replays are idempotent through `recs:nearline:seen:<event_id>` (`SET NX EX 900`). Events older
than `NEARLINE_TTL_SECONDS` are ignored, so a replay can never revive an expired window.
`NEARLINE_IDLE_EXIT_SECONDS>0` exits 0 after that many idle seconds (drain mode, used by e2e). `NEARLINE_START_OFFSET`
(`latest` default) is where a group with no committed offset starts.

### D4. Co-views are per session
A listing viewed, clicked or carted is co-viewed with the up to 3 distinct listings that session touched just before
(`session_id`, else the actor). Both directions are incremented. Different sessions are never linked.

### D5. Impressions do not become "recent items"
Impressions (search result lists) feed only the click-through counters. Recents, categories and co-views come from
view/click/add_to_cart.

## The Redis contract (what team-ai reads)

All keys carry TTL `NEARLINE_TTL_SECONDS` (86400 s), refreshed on every write. All are in the same Redis DB as the
`recs:v1:*` keys (`REDIS_DATABASE`, 0). `actor` is the warehouse `user_key`: the principal's id for a signed-in user,
else `anon:<anonymous_id>`; with neither, the session id. Anonymous visitors are not stitched to their later login here
(the featurestore does that offline).

| Key | Type | Content | Read with |
|---|---|---|---|
| `recs:nearline:user:{actor}:items` | ZSET | member `listing_id`, score = event time (epoch seconds, float); the 50 most recent | `ZREVRANGE key 0 n-1` (newest first) |
| `recs:nearline:user:{actor}:cats` | HASH | `item_category` -> float affinity; view +1, click +2, add_to_cart +5 | `HGETALL` |
| `recs:nearline:coview:{listing_id}` | ZSET | member other `listing_id`, score = co-view count; the top 50 | `ZREVRANGE key 0 n-1 WITHSCORES` |
| `recs:nearline:ctr:{listing_id}` | HASH | `clicks_ips`, `imprs_ips` (floats, weight sqrt(position)); CTR = `clicks_ips / imprs_ips` capped at 1 | `HGETALL` |

Internal, not part of the contract: `recs:nearline:session:{session}:items` (ZSET) and `recs:nearline:seen:{event_id}`.
A missing key means "no signal in the window". Values are strings from a `decode_responses=True` client; a reader
without it receives bytes. Readers must tolerate absent keys and treat Redis errors as "no signal" (the signals only
re-rank; they are never required to serve).

## Compose proposal (not applied; compose is another change's file)

```yaml
  platform-recsys-nearline:
    image: platform-recsys:local
    entrypoint: ["python", "-m", "recsys.nearline"]
    restart: unless-stopped
    environment:
      - KAFKA_BROKERS=redpanda:9092
      - KAFKA_ANALYTICS_TOPIC=analytics.events
      - NEARLINE_CONSUMER_GROUP=platform-recsys-nearline
      - REDIS_HOST=redis
      - REDIS_PORT=6379
    depends_on:
      - redis
      - redpanda
```

The service is a plain long-running container (no `jobs` profile: nothing schedules it). The image must be rebuilt: it
gains `confluent-kafka` in `requirements.txt`.

## Risks

- A consumer that is down loses nothing: the group resumes from its committed offset, and events older than 24 hours are
  ignored anyway. A brand-new group starts at `latest`, so the first 24 hours after a first deploy are partial.
- Redis memory: per active actor about 50 items + a category hash; per listing up to 50 co-views; one `seen` key per event
  for 15 minutes.
