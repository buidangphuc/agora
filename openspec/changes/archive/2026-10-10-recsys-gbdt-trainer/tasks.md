# Tasks

## 1. Code - platform-featurestore
- [x] Dataset builder takes per-dataset columns; `rank_training@v1` (SQL, registry, lock). verify: tests for labels, window, AS_OF, duplicates, columns/manifest; ALS lock hash unchanged.
- [x] README. verify: `pytest`, `ruff check .`.

## 2. Code - platform-recsys
- [x] `ranker/contract.py` + `tests/test_ranker_contract.py` (registry and team-ai parity). verify: fails when a list drifts.
- [x] `ranker/gbdt.py` (binned trees, LambdaRank, artifact, pure-python scorer) + NDCG. verify: learns a planted signal; scorer equals trainer; deterministic.
- [x] `ranker/training.py` (dataset resolution, point-in-time snapshot join, debiased CTR with `ctr_source`, temporal split, rows file). verify: leakage and window tests.
- [x] `ranker/stage.py`, settings/`.env.example`, `ModelRegistry` champion key, `publish`/`redis_cache` ranker key + retention, `pipeline.py` wiring. verify: stage tests with fake Redis; env drift gate.

## 3. Code - team-ai (feature list file only)
- [x] `RANKING_FEATURES` in `recommend/features.py`. verify: recsys parity test reads it.

## 3b. Code - team-ai (loader)
- [x] `recommend/ranker_artifact.py` (parse `agora-gbdt/1`, feature-list gate, pure-Python scorer, per-generation loader), `PrecomputedCache.ranker_key`, `RedisFeatureStore(view=)`, service/factory wiring, `explain` fields. verify: `tests/unit/modules/recommend/test_trained_ranker.py` (parse, hand-computed tree, wrong features -> fixed, generation switch, artifact reorders vs fixed); `make check`, `make test`.

## 4. E2E - platform-e2e (real job images; FEATURES `planned`, note `needs rebuild`)
- [x] `featurestore/ranking_dataset.feature`, `recommendations/gbdt_ranker.feature`; FEATURES.yaml entries in platform-featurestore and platform-recsys.
- [x] `recommendations/gbl_ranker_serving.feature` (published artifact reorders home_feed through the gateway); FEATURES.yaml entry in team-ai (`planned`, `needs rebuild`).

## Evidence (2026-10-10)

- Final gate on HEAD 6b714354: parallel lane (`e2e.sh -q -n 4 -m "not destructive"`) 825/825 passed, run twice; destructive lane 103/104. The one failure, `test_c1_notification_faults::test_name_lookup_failure_still_notifies`, belongs to change C1 (not this change) and passed on isolated rerun (flaky). Stack READY.
