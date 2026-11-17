## Why

`Recommend` has no experiment assignment and its `model_version` is the generation id only. The ladder floor keeps the
real model version, and `fallback` reaches only the `recs.served` log line, so analytics cannot tell which tier, ranker or
arm produced an impression (Q16, ai#4). Reports in `analytics-experiment-reports` group by `model_version`; they need a
string that names generation, serving tier, ranker and arm, and an assignment that is stable per visitor.

## What changes

initiative: docs-ai-data

- `placements.yaml` gains an optional `experiment: {flag: <key>}` per placement. `home_feed` names `recs-home-feed-ranker`.
- team-ai evaluates the named Flipt variant flag over Flipt's REST evaluate API (`POST /evaluate/v1/variant`) with the
  `httpx` it already ships. The entity id is the SHA-256 of the user id (or anonymous id) plus a salt, so Flipt never sees a
  raw id. Evaluation starts at the top of `recommend` and is awaited before the first ranking; timeout 50 ms.
- A variant attachment (JSON) may override only `ranking.model`. Any other attachment key is ignored and logged by key name.
- `RecommendResponse.model_version` becomes `<generation>/<tier>/<ranker>/<arm>`. No proto change (opaque string).
  - generation: the serving generation, or `serving-fallback` when it cannot be read.
  - tier: the ladder strategy that produced the items (`user_precomputed`, `seed_vector_similarity`, `category_popular`,
    `cart_cross_similarity`, `global_popular`), `popular` when the floor served, `serving-fallback` after an unexpected
    failure.
  - ranker: `gbdt` (trained model ranked), `fixed_weights` (gbdt placement without a trained model), `similarity`.
  - arm: `<flag>:<variant>`, `<flag>:unassigned` when Flipt failed or timed out, `none` otherwise.
- The `recs.served` log line gains `tier`, `ranker`, `arm`.
- A script `team-ai/scripts/seed_flipt_flags.py` creates the flag `recs-home-feed-ranker` (variants `control`, `similarity`)
  idempotently, switched off. No live experiment is started.
- Modified requirements: `recs-serving-safeguards` (degrade stamp) and `recsys-generations` (reported version).

Out of scope: candidate-ladder or generation overrides (U4), frontend exposure beacon, reports, interleaving.

## NFR

- Flipt evaluation adds at most 20 ms p95 to `Recommend` when Flipt is healthy; hard timeout 50 ms (`RECS_FLIPT_TIMEOUT_MS`, default 50).
- `Recommend` never fails or returns a different status because of Flipt (U3); availability follows the existing ladder.
- Assignment is deterministic: the same entity id and flag state give the same variant on every call and every replica.
- With the flag disabled or no flag declared the added cost is zero network calls.

## Data classification

| field | class | purpose | basis | retention |
|---|---|---|---|---|
| `user_id` / `anonymous_id` (input of the entity hash) | personal (pseudonymous id) | stable arm assignment | first-party service operation, same basis as existing personalised serving; DPO sign-off on experiments is the human item U6 of the initiative | not stored by team-ai; sent to Flipt only as salted SHA-256 |
| entity hash | pseudonymous | Flipt evaluation key | as above | Flipt keeps no per-entity state for evaluation |
| `model_version` string | none | provenance on responses, logs, events | n/a | log retention unchanged |

The salt is `RECS_EXPERIMENT_SALT`; it is never logged. Raw ids and hashes are never written to team-ai logs.

## Threat model

- Assets: assignment integrity, raw visitor ids, recommendation availability.
- Abuse: a caller forces an arm by sending another `user_id` (already neutralised by `_bound_identity`: users are served as
  themselves, anonymous callers keep only their device id); a slow or hostile Flipt stalls `Recommend` (50 ms timeout, default
  ranker); id leakage to Flipt (hash only); arm spoofing through attachments (only `ranking.model`, validated against the
  implemented models; an unknown value is ignored).
- Authz: unchanged, `Recommend` needs `listing.read`; the seed script needs Flipt access and is not exposed on any RPC.
