## Why

Retrieval stages (ALS matrix factorization, vector ANN search) generate candidate items based on single semantic or latent factor similarity. Sorting candidates purely by raw cosine similarity (`sorted(by=cosine)`) ignores critical conversion signals such as item price, category affinities, recency, historical click-through rates, and seller rating.

Following **P3-T1**, this change introduces a GBDT re-ranking stage in `platform-recsys/recsys/ranker/` and integrates it into `team-ai`'s ranking pipeline, achieving superior ranking metrics ($NDCG@10 > baseline$).

## What Changes

- **platform-recsys** (`recsys/ranker/`):
  - `FeatureExtractor`: Transforms candidate metadata and user signals into numerical feature vectors.
  - `GBDTRanker`: GBDT / Tree-ensemble ranking model that scores candidate feature vectors.
  - Evaluation comparing GBDT ranking vs. baseline cosine ranking showing NDCG@10 gain.
  - Unit tests in `platform-recsys/tests/test_ranker.py`.
- **team-ai** (`app/modules/business/recommend/ranking.py`):
  - Integrate feature weighting and ranker scoring into `rank_and_filter`.

## Non-goals

- No heavy C++ compilation dependencies in pure offline unit tests — tree evaluation is written with fast, portable vector math.
