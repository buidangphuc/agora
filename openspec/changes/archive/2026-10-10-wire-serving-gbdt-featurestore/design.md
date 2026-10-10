## Reconciliation with the archived serving changes (2026-10-09)

This change predates `recs-serving-safeguards`, `serving-switch-atomicity`, `featurestore-materialization` and
`recsys-generation-publish`. What changed under it:

- **Feature store is no longer in-memory only.** The proposal non-goal "no remote feature-store transport" is superseded:
  `RedisFeatureStore` reads `fs:item_popularity:current` then `fs:item_popularity:v<n>:<listing_id>` (the layout
  `featurestore-materialization` writes), and `_build_feature_store` selects it when `RECS_FEATURESTORE_REDIS_URL` is set.
  Task 1.2 ("inject both via factory.py") is therefore half true: the feature store is injected; the ranker is not
  configurable (the factory builds `GBDTRankerAdapter()` with its built-in weights, consistent with the non-goal "consume the
  ranker artifact as-is"), and the nearline store is injected by `add-recsys-nearline-signals`.
- **Serving reads the generation pointer.** Candidates, popular lists and the Qdrant collection come from the serving
  generation (`recs:v1:serving`); this change's ranking step sits after candidate retrieval and is unaffected.
- **`explain` and `status` are not on the wire.** `RecommendResponse` has no explain or status, so the scenarios in this change
  are service-level and are verified by unit tests (each carries a `VERIFIED BY` line). The feature store's effect on order is
  covered end to end by `recommendations/serving_safeguards.feature`.
- **The verification gap named in tasks 2.2 is closed** by `tests/unit/modules/recommend/test_factory_serving_wiring.py`, which
  goes through `build_recommendation_service`; dropping `feature_store=` from the factory makes it fail.

## Feature contract: serving reads the registry's names

The ranker used to read `category_match`, `popularity_score`, `price`, `freshness_score`, `historical_ctr` and `conversion_rate`
from the online row. No view produces them (`item_popularity@v1` has `views_7d`, `clicks_7d`, `add_to_cart_7d`,
`favorites_current`, `review_count`, `avg_rating`, `ctr_7d`), so every one took its default in production. Serving now reads the
registry names, from `fs:item_popularity:v<N>:<listing_id>` with `N` from `fs:item_popularity:current`.

One explicit list, `recommend/features.py::ITEM_POPULARITY_FEATURES` (name -> default 0.0), mirrors the registry entry;
`test_item_feature_contract.py` fails when the two drift (it reads `platform-featurestore/registry/features.yaml` when that repo is
checked out). `user_activity@v2` (`views_7d`, `clicks_7d`, `add_to_cart_7d`, `favorites_current`, `follows_current`,
`paid_orders_30d`, entity = user key) is not read by serving: the ranker has no user-side input yet.

GBDT input vector, in the weight order `[0.30, 0.25, 0.15, 0.05, 0.05, 0.10, 0.10]`:

| # | input | source |
|---|---|---|
| 0 | similarity | candidate retrieval score |
| 1 | category_match | `candidate.category_id == query.category_id` (not a feature-store value) |
| 2 | popularity | `log1p(1*views_7d + 2*clicks_7d + 5*add_to_cart_7d + 3*favorites_current) / log1p(500)`, capped at 1 |
| 3 | price | no registry source: 0 (inert) |
| 4 | freshness | no registry source: 0 (inert) |
| 5 | ctr | nearline debiased CTR when usable, else `ctr_7d` clamped to [0, 1] |
| 6 | cvr | no registry source: 0 (inert) |

A feature missing from a row, null (`avg_rating` for an item without reviews) or not a finite number takes its default and is
counted: `explain["feature_defaults"]` is the number of defaulted registry features over the candidates that had a row. A row written
with the old names therefore reports 7 per candidate and ranks like an empty row. It never raises.

`ctr_7d` is also used by the bounded tie-break boost from `recs-serving-safeguards`, so it acts twice (weight 0.10 in the vector,
at most +0.05 as a boost); the boost is kept because its tie-break behaviour is specified and has an e2e scenario.

**Trainer (platform-recsys, not edited).** `platform-recsys/recsys/ranker/features.py::CandidateFeatures` trains on a different space:
`similarity_score, category_match, popularity_score, price, freshness_days, historical_ctr, conversion_rate, cart_to_order_ratio,
user_cvr, price_ratio`. Serving uses 7 of those slots with the sources above. To train and serve on the same features the trainer
must (a) build `popularity` and `ctr` from the registry columns exactly as the table says, from the same `item_popularity@v<N>`
snapshot, and (b) either drop `price`, `freshness`, `cvr` or have a view provide them, then update `ITEM_POPULARITY_FEATURES` and
the weight vector together. The `eGMVRankerAdapter` (no placement uses it) still reads the old names and is unchanged.
