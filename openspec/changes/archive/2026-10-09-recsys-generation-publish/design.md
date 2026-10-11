## Context

See proposal.md for the motivation. Current code (2026-10-09):

- **platform-recsys:**
  - `pipeline.run` trains ALS on the governed dataset and evaluates on a temporal holdout.
  - It then calls `ModelRegistry.evaluate_and_promote`. Only when the model is promoted does it call
    `qdrant_load.load_vectors` (upsert into `item_als_vectors`/`user_als_vectors`, then `_prune_stale` of other
    versions) and `redis_cache.load_cache`.
  - `load_cache` overwrites `recs:v1:user:*`, `recs:v1:item:*` and `recs:v1:popular`, and sets `recs:v1:model_version`
    last.
  - The registry champion pointer is `recs:model:champion`.
- **team-ai:**
  - `app/modules/business/recommend/cache.py` reads `{prefix}:{schema}:user:<id>`, `:popular` and `:model_version`.
  - The Qdrant backend reads `RECS_QDRANT_COLLECTION` (`item_als_vectors`).

## Goals / Non-Goals

**Goals:**
- An atomic switch per generation.
- Keep exactly one previous generation.
- Rollback.
- Reject structurally degenerate candidates.
- Serving that follows the pointer.

**Non-Goals:** two-tower and GBDT collections, and the serving safeguards (change 7).

## Decisions

### D1. Generation keys and pointer
The generation is the `model_version`.

**Keys** are `recs:v1:gen:<gen>:user:<k>`, `recs:v1:gen:<gen>:item:<id>` and `recs:v1:gen:<gen>:popular`, each with the
existing TTL.

**Pointers:**
- `recs:v1:serving` has no TTL.
- `recs:v1:previous` has no TTL.
- `recs:v1:model_version` mirrors `serving`, for older readers.

**Switch.** A Lua script (`EVAL`) sets `previous=old serving` and `serving=new` and mirrors `model_version`. The three
writes therefore happen atomically.

**TTL refresh.** Pointers have no TTL, but generation keys do. The job refreshes the TTL of the serving and previous
generations on every run, using a scan over the gen prefix to `EXPIRE` them. Rollback therefore never points at expired
keys.

### D2. Qdrant generation collections and alias
**Collections.** Vectors go to `item_als_vectors__<gen>` and `user_als_vectors__<gen>`, each created fresh.

**Alias.** After the vectors are written, `update_collection_aliases` atomically moves the alias `item_als_vectors` (and
`user_als_vectors`) to the new collections.

**First run.** When a real collection named `item_als_vectors` exists and is not an alias, it is copied into
`item_als_vectors__legacy` (scroll and upsert), the original is deleted, and the alias is created. This replaces
`_prune_stale`.

**Deleting older generations** happens after the switch. Any `__<gen>` collection and gen keys that are neither
serving nor previous are removed.

### D3. Structural gate
The function is `structural_check(user_recs, item_recs, factors, dataset_users, dataset_items, settings)`. It returns
`(ok, reason)`.

| Check | Rule |
|---|---|
| Coverage | `len(user_recs) / dataset_users` |
| Catalogue coverage | `len(union of top-N) / dataset_items` |
| Overlap | Mean Jaccard of top-N over a deterministic sample of up to 500 user pairs |
| Factors | `numpy.isfinite(...).all()` on both factor matrices |

It runs before `evaluate_and_promote`. On failure the job registers the model with status `rejected` and
`metrics.gate_reason`, writes the summary decision `rejected` with the reason, and publishes nothing.

### D4. Rollback command
- Command: `python -m recsys rollback`, in `recsys/__main__.py`, dispatched by `argv[1]`.
- It reads `serving` and `previous`. If `previous` is missing, or its keys are gone, it exits 2.
- Otherwise it runs the same Lua swap in reverse, moves the Qdrant aliases to the previous collections, and sets the
  registry champion to the previous model, whose status becomes champion. The demoted model's status becomes
  `archived`.

### D5. team-ai reads the pointer
- `RecommendationCache` gains a pointer read: `GET recs:v1:serving`, memoised for 5 s per process.
- When the pointer is set, keys become `{prefix}:{schema}:gen:<gen>:...`. When it is absent, the old unscoped keys are
  used.
- `get_model_version` returns the pointer when it is set.
- No change to the Qdrant name, because the alias keeps it.

## Risks / Trade-offs

- **Two generations double the Redis and Qdrant footprint.** Mitigation: the bound is two, and local scale is small.
- **The legacy-collection copy on the first publish is O(collection).** Mitigation: it runs once, and local scale is
  small.
- **Serving reads the pointer a few seconds stale.** Mitigation: a 5 s memo, and the spec allows 10 s.

## Migration Plan

- Deploy recsys first. Its first publish creates the gen keys, the aliases and the pointer, and team-ai's fallback
  covers the gap.
- Then deploy team-ai.
- Rollback of the code: recsys keeps writing the unscoped v1 keys for one release as a compatibility shim
  (`RECS_WRITE_LEGACY_KEYS`, default true), so reverting team-ai keeps serving the latest generation. A follow-up
  removes the shim.
