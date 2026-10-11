## Why

`placements.yaml` declares `ranking.model: "gbdt"` and `use_featurestore: true`, but neither
value reaches any behaviour. `grep use_featurestore` outside the loader returns nothing, and
`service.py` contains no reference to the GBDT ranker — the serving path still ends at
`rank_and_filter`, which sorts by the retrieval cosine score.

Both flags are **dead config**: they were introduced by `add-gbdt-ranker` (P3-T1) and
`add-platform-featurestore` (P3-T2), whose unit tests pass because they exercise the modules in
isolation. A module test cannot observe that nothing calls the module. This change closes the
serving-path wire and makes the dead-flag class of defect structurally impossible.

## What Changes

- **team-ai** (`app/modules/business/recommend/`):
  - `RecommendationService` takes a feature-store port and a ranker port (both injectable,
    in-memory adapter for offline tests).
  - `ranking.model == "gbdt"` → candidates are scored by `GBDTRanker.rank_candidates`, not by
    cosine sort. Any other value keeps the existing cosine ordering.
  - `use_featurestore == true` → candidates are enriched with item features before ranking;
    a per-request hit count is recorded.
  - `explain` payload gains `ranking_model`, `featurestore_hit_count`, `ranking_source`.
  - **Startup validation**: a placement naming a ranking model or capability with no bound
    implementation fails at application start, not at request time.
- **Ranker failure is degradation, not error**: ranker or feature-store failure falls back to
  cosine ordering and marks the response `degraded`.

## Non-goals

- No model retraining; the ranker artifact is consumed as-is.
- No remote feature-store transport — the port may be backed by the in-memory adapter.
- No change to retrieval, ladder or cache semantics.
