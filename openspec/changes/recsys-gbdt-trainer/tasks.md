# Tasks

## 1. Code - platform-featurestore
- [ ] Dataset builder takes per-dataset columns; `rank_training@v1` (SQL, registry, lock). verify: tests for labels, window, AS_OF, duplicates, columns/manifest; ALS lock hash unchanged.
- [ ] README. verify: `pytest`, `ruff check .`.

## 2. Code - platform-recsys
- [ ] `ranker/contract.py` + `tests/test_ranker_contract.py` (registry and team-ai parity). verify: fails when a list drifts.
- [ ] `ranker/gbdt.py` (binned trees, LambdaRank, artifact, pure-python scorer) + NDCG. verify: learns a planted signal; scorer equals trainer; deterministic.
- [ ] `ranker/training.py` (dataset resolution, point-in-time snapshot join, debiased CTR with `ctr_source`, temporal split, rows file). verify: leakage and window tests.
- [ ] `ranker/stage.py`, settings/`.env.example`, `ModelRegistry` champion key, `publish`/`redis_cache` ranker key + retention, `pipeline.py` wiring. verify: stage tests with fake Redis; env drift gate.

## 3. Code - team-ai (feature list file only)
- [ ] `RANKING_FEATURES` in `recommend/features.py`. verify: recsys parity test reads it.

## 4. E2E - platform-e2e (real job images; FEATURES `planned`, note `needs rebuild`)
- [ ] `featurestore/ranking_dataset.feature`, `recommendations/gbdt_ranker.feature`; FEATURES.yaml entries in platform-featurestore and platform-recsys.
