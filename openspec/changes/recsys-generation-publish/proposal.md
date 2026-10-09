## Why

This is AI-first change 6 of 7. A promoted model is published by overwriting the live data in place.
`load_cache` rewrites `recs:v1:user:*`, `recs:v1:item:*` and `recs:v1:popular`, and `load_vectors` upserts into
`item_als_vectors` and then deletes points from other versions. So:
- While a publish is running, team-ai serves a mix of the old and new generations.
- Any crash in the middle leaves that mix in place.
- There is no way back to the previous model short of retraining it.

The promotion gate compares holdout metrics only. A degenerate model can therefore pass and go live, for example one
that recommends the same handful of items to everyone, or one that covers almost no users.

On 2026-10-08 the user decided (D2) to extend the v1 registry with generation-scoped keys and a pair of
champion/previous pointers, rather than add a `recs:v2` namespace.

## What Changes

- **platform-recsys:**
  - **Publish by generation.** A promoted model writes its own keys `recs:v1:gen:<model_version>:{user:<k>,item:<id>,popular}`
    and its own Qdrant collection `item_als_vectors__<model_version>`.
  - **Switch only when complete.** Only after everything is written does it move the Qdrant alias `item_als_vectors`
    and the pointers: `recs:v1:serving` becomes the new generation, and `recs:v1:previous` the one it replaced.
  - **Compatibility.** `recs:v1:model_version` keeps mirroring `serving`.
  - **Retention.** Generations other than the serving and previous ones are deleted (their keys and collection).
  - **Structural gate.** It runs before the metric gate and rejects a candidate when:
    - user coverage is below `GATE_MIN_USER_COVERAGE` (default 0.5 of the dataset's users);
    - catalogue coverage of its top-N lists is below `GATE_MIN_ITEM_COVERAGE` (default 0.05);
    - the mean pairwise overlap of users' top-N lists is above `GATE_MAX_LIST_OVERLAP` (default 0.9);
    - any factor is NaN or infinite.

    A rejection is recorded with its reason, and serving is unchanged.
  - **Rollback.** `python -m recsys rollback` swaps `serving` and `previous` (pointers, alias and registry champion) and
    refuses when there is no previous generation.
- **team-ai:**
  - The recommendation cache reads `recs:v1:serving` and then the generation-scoped keys.
  - If `recs:v1:serving` is absent, it falls back to the unscoped v1 keys, so there is no break before the first new
    publish.
  - Qdrant reads keep using the `item_als_vectors` name, which is now an alias.
- **platform-e2e:** scenarios run the real recsys image on dataset fixtures and read recommendations through the gateway.

Repos touched: platform-recsys, team-ai, platform-e2e. **No proto change.**

## Capabilities

### New Capabilities
- `recsys-generations`: how a promoted model becomes the serving generation atomically, how the previous one is kept
  and restored, which candidates the structural gate rejects, and how serving reads the generation.

### Modified Capabilities
- None. The existing `recommendations` promotion-gate requirements still hold; this change adds the generation and
  structural requirements in a new capability.

## Non-goals

- **Two-tower and GBDT publishing.** They keep their current collections.
- **Serving safeguards.** Never-error fallback, eligibility filtering, the memory-backend refusal and `placement_id`
  belong to `recs-serving-safeguards` (change 7).
- **A/B serving of two generations at once.**

## Impact

- **New recsys settings:** `GATE_MIN_USER_COVERAGE`, `GATE_MIN_ITEM_COVERAGE` and `GATE_MAX_LIST_OVERLAP`.
- **Redis and Qdrant:**
  - At most two generations are kept: keys and one collection each.
  - The first publish after this change creates the alias. If a plain `item_als_vectors` collection exists, it is
    renamed by copying it into a generation collection, and the alias then takes the name.
- **team-ai:** one extra Redis GET per request for the pointer, cached in process for 5 s.
