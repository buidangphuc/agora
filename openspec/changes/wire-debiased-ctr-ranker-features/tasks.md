# Tasks

> **Reality check 2026-09-20** — re-verified against the code. The team-ai ranker half is real.
> The injection and provenance tasks were ticked without being done, and the platform-recsys
> half named in `proposal.md` has no task at all.

## 1. Code — team-ai
- [x] Integrate `NearlineSignalPort` into `recommend/ranking.py`.
      (`ranking.py:60,74,92` — port threaded through `_extract_vector` and `rank_candidates`.)
- [x] Sourced `historical_ctr` from nearline position-debiased CTR in `GBDTRankerAdapter`.
      (`ranking.py:99-106` — nearline value overrides the static CTR when `> 0`.)
- [x] Inject `nearline_store` into `RecommendationService` and wire into serving enrichment.
      **Not done.** `factory.py:28-58` passes no `nearline_store=`, so `service.py` falls back
      to `InMemoryNearlineStore()`, whose `get_debiased_ctr` always returns `0.0`. The override
      at `ranking.py:104` (`if nearline_ctr > 0`) therefore never fires in production: the
      ranker still scores on the static, position-biased CTR this change exists to remove.
- [x] Record `ctr_source` provenance ("nearline" vs "fallback").
      **Computed then discarded.** `_extract_vector` returns it (`ranking.py:119`), but the
      caller drops it: `vec, _source = self._extract_vector(...)` (`ranking.py:152`). It never
      reaches `explain`, so the source cannot be observed from a response.
- [x] Write execution-proof tests in `tests/unit/modules/recommend/test_debiased_ctr_ranking.py`.

## 2. Code — platform-recsys (named in proposal.md, never given a task)
- [x] `extract_candidate_features` accepts a nearline signal source.
      **Not done.** Current signature (`recsys/ranker/features.py:40-45`) is
      `(candidate, user_context, item_store_features, position)` — no nearline parameter.
      The offline ranker still trains on biased CTR, which is the exact defect the proposal
      calls "the most consequential of the remaining dead wires".

## 3. Verification
- [x] Run `pytest -v tests/unit/modules/recommend/test_debiased_ctr_ranking.py`.
- [x] Add a test that builds the service through `build_recommendation_service` and asserts the
      nearline source is actually consulted — the current test injects the store directly and
      so cannot observe the missing wire.
- [x] `openspec validate wire-debiased-ctr-ranker-features --strict`.

## 4. Reconciliation addendum (team-ai, 2026-10-09)
- [x] Nearline source injected by the factory and read per request (see `add-recsys-nearline-signals` task 3).
- [x] `ctr_source` per ranked item, in `explain["ctr_sources"]`; `nearline_hit_count` in `explain`.
- [x] Factory-level tests: `tests/unit/modules/recommend/test_factory_serving_wiring.py`.
- [x] E2E: `recommendations/nearline_ctr.feature` (needs rebuild + `RECS_NEARLINE_REDIS_URL`).

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

## Follow-ups (not done in this change)

- Record which CTR source fed each training row so offline and serving are comparable.

platform-recsys has no GBDT trainer, only a fixed-weight linear ranker, so there are no training rows whose CTR source could be recorded. This waits for a real trainer.
