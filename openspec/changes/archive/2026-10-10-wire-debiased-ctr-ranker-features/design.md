## Reconciliation (2026-10-09)

- **The nearline source is now injected.** `build_recommendation_service` builds `RedisNearlineStore` from `RECS_NEARLINE_REDIS_URL`
  (contract: `add-recsys-nearline-signals/design.md`, "Nearline Redis contract"). Before, the factory passed an in-memory store
  nothing wrote to, so the `> 0` override in the ranker never fired in production. The service reads the store once per request into
  a snapshot (the ranker is synchronous) and passes it to the ranker.
- **`ctr_source` reaches the response.** `GBDTRankerAdapter.extract_features` returns a `FeatureVector` (values plus `ctr_source`);
  each ranked item carries `ctr_source` (kept through the online-feature re-rank), and `explain["ctr_sources"]` counts returned items
  per source. `explain` also reports `nearline_enabled` (a store is configured) and `nearline_hit_count` (candidates with usable data).
  Neither is on the gateway wire (`RecommendResponse` has no explain), which is why the provenance scenarios are unit-verified.
- **What "fallback" is in production.** The prior CTR is `ctr_7d` from the `item_popularity` online row (the ranker now reads the
  registry's names; see `wire-serving-gbdt-featurestore/design.md`, "Feature contract"). Without a row it is `0.0`.
- **A usable debiased CTR of exactly 0 is "fallback"** (the ranker overrides only when the nearline CTR is `> 0`, the rule the existing
  tests pin). An item shown a lot and never clicked therefore keeps its prior CTR.
- **Out of scope here, still open: platform-recsys.** `extract_candidate_features` taking a nearline source and recording the CTR source per
  training row (tasks 2.x) is the recsys agent's half; this design covers team-ai only.

## Finding for the recsys side

The consumer adds the same weight (`position ** 0.5`) to both `clicks_ips` and `imprs_ips`. When all of an item's events are at one
position the weight cancels in `clicks_ips / imprs_ips`, so an item whose impressions were *consistently* at worse positions gets the
same CTR as at position 1, not a higher one. Position debiasing only appears when clicks and impressions occur at different positions.
The scenario "Equal raw CTR, worse positions, higher debiased CTR" is therefore only true of the consumer if impressions are not
weighted by position (the usual IPS estimator weights clicks by `1 / propensity` and counts impressions unweighted).
