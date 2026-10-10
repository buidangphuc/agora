# Tasks

## 1. Code — platform-recsys & team-ai
- [x] Implement `recsys/ranker/features.py` (feature extraction from candidates and user context).
- [x] Implement `recsys/ranker/model.py` (`GBDTRanker` scoring model).
- [x] Implement `recsys/ranker/__init__.py`.
- [x] Update `team-ai/app/modules/business/recommend/ranking.py` to use multi-feature ranking scoring.
- [x] Write unit tests in `platform-recsys/tests/test_ranker.py` demonstrating NDCG@10 gain over raw cosine.

## 2. Verification
- [x] Validate OpenSpec change (`openspec validate add-gbdt-ranker --strict`).
- [x] Run `pytest -v tests/test_ranker.py` in `platform-recsys/`.

## Evidence (2026-10-10)

- Code and unit tests: each repo's `make check` / test suite was green at merge (see the commit bodies).
- e2e after rebuilding team-ai, team-search (server and indexer), gateway, frontend and the recsys image, with
  platform-recsys-nearline and the modelserve overlay (fake TEI + router) running:
  - ML scenarios: 23/23, twice;
  - modelserve, hybrid and taxonomy: 27/27, three times;
  - placement and serve-trained scenarios: green three times.
- Scenarios that cannot be produced end to end carry a VERIFIED BY line in the spec and a not-testable FEATURES
  entry.
- spec_sync --strict reports e2e-ready.
