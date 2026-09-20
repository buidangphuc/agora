# Tasks

> **Reality check 2026-09-20** — re-verified against the code. The team-ai ranker half is real.
> The injection and provenance tasks were ticked without being done, and the platform-recsys
> half named in `proposal.md` has no task at all.

## 1. Code — team-ai
- [x] Integrate `NearlineSignalPort` into `recommend/ranking.py`.
      (`ranking.py:60,74,92` — port threaded through `_extract_vector` and `rank_candidates`.)
- [x] Sourced `historical_ctr` from nearline position-debiased CTR in `GBDTRankerAdapter`.
      (`ranking.py:99-106` — nearline value overrides the static CTR when `> 0`.)
- [ ] Inject `nearline_store` into `RecommendationService` and wire into serving enrichment.
      **Not done.** `factory.py:28-58` passes no `nearline_store=`, so `service.py` falls back
      to `InMemoryNearlineStore()`, whose `get_debiased_ctr` always returns `0.0`. The override
      at `ranking.py:104` (`if nearline_ctr > 0`) therefore never fires in production: the
      ranker still scores on the static, position-biased CTR this change exists to remove.
- [ ] Record `ctr_source` provenance ("nearline" vs "fallback").
      **Computed then discarded.** `_extract_vector` returns it (`ranking.py:119`), but the
      caller drops it: `vec, _source = self._extract_vector(...)` (`ranking.py:152`). It never
      reaches `explain`, so the source cannot be observed from a response.
- [x] Write execution-proof tests in `tests/unit/modules/recommend/test_debiased_ctr_ranking.py`.

## 2. Code — platform-recsys (named in proposal.md, never given a task)
- [ ] `extract_candidate_features` accepts a nearline signal source.
      **Not done.** Current signature (`recsys/ranker/features.py:40-45`) is
      `(candidate, user_context, item_store_features, position)` — no nearline parameter.
      The offline ranker still trains on biased CTR, which is the exact defect the proposal
      calls "the most consequential of the remaining dead wires".
- [ ] Record which CTR source fed each training row so offline and serving are comparable.

## 3. Verification
- [x] Run `pytest -v tests/unit/modules/recommend/test_debiased_ctr_ranking.py`.
- [ ] Add a test that builds the service through `build_recommendation_service` and asserts the
      nearline source is actually consulted — the current test injects the store directly and
      so cannot observe the missing wire.
- [x] `openspec validate wire-debiased-ctr-ranker-features --strict`.
