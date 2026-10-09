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

## Finding (not fixed here)

The GBDT feature vector reads `category_match`, `popularity_score`, `price`, `freshness_score`, `historical_ctr` and
`conversion_rate` from the item features. The `item_popularity` view that `platform-featurestore` materialises provides
`views_7d`, `clicks_7d`, `add_to_cart_7d`, `favorites_current`, `review_count`, `avg_rating` and `ctr_7d`. Of those, only `ctr_7d`
and `favorites_current` influence ranking, through `apply_online_features`; every other GBDT input falls back to its default in
production. Closing that needs either a feature view with the ranker's inputs or a ranker retrained on the view's columns, which
is a modelling decision.
