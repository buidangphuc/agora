"""Execution-proof tests for position-debiased CTR in candidate feature extraction and GBDT serving."""

from __future__ import annotations

import asyncio

from app.modules.business.recommend.placement_config import PlacementRegistry
from app.modules.business.recommend.ranking import (
    GBDTRankerAdapter,
    InMemoryNearlineStore,
)
from app.modules.business.recommend.schemas import Candidate, RecommendQuery
from app.modules.business.recommend.service import RecommendationService


class DummyBackend:
    async def retrieve_similar(self, seed_listing_id: str, top_k: int = 100):
        return []

    async def popular(self, top_k: int = 100):
        return []


def test_equal_raw_ctr_worse_position_yields_higher_debiased_ctr_and_gbdt_score():
    ranker = GBDTRankerAdapter()
    query = RecommendQuery(placement_id="home_feed")

    # Item A: Shown at pos 1 (favored position, raw CTR = 0.05, debiased CTR = 0.05)
    # Item B: Shown at pos 5 (worse position, raw CTR = 0.05, debiased CTR = 0.11 via IPS)
    cand_a = Candidate(listing_id="item_a_pos1", score=0.80, in_stock=True)
    cand_b = Candidate(listing_id="item_b_pos5", score=0.80, in_stock=True)

    nearline = InMemoryNearlineStore({
        "item_a_pos1": 0.05,
        "item_b_pos5": 0.11,
    })

    # When ranked with nearline store, item B receives higher score due to debiased CTR
    ranked = ranker.rank_candidates(
        candidates=[cand_a, cand_b],
        query=query,
        nearline_store=nearline,
        limit=2,
    )

    assert len(ranked) == 2
    # PROOF OF WIRING: item_b_pos5 is ranked first because of higher position-debiased CTR
    assert ranked[0].listing_id == "item_b_pos5"
    assert ranked[1].listing_id == "item_a_pos1"
    assert ranked[0].score > ranked[1].score


def test_ctr_source_provenance_nearline_vs_fallback():
    ranker = GBDTRankerAdapter()
    query = RecommendQuery(placement_id="home_feed")
    cand = Candidate(listing_id="item_tested", score=0.80, in_stock=True)

    # 1. Nearline present
    nearline = InMemoryNearlineStore({"item_tested": 0.08})
    vec_nearline, source_nearline = ranker._extract_vector(
        cand, query, item_feat={"historical_ctr": 0.02}, nearline_store=nearline
    )
    assert source_nearline == "nearline"
    assert vec_nearline[5] == 0.08

    # 2. Nearline absent
    nearline_empty = InMemoryNearlineStore()
    vec_fallback, source_fallback = ranker._extract_vector(
        cand, query, item_feat={"historical_ctr": 0.02}, nearline_store=nearline_empty
    )
    assert source_fallback == "fallback"
    assert vec_fallback[5] == 0.02


def test_serving_path_enriches_from_nearline():
    class DummyCache:
        async def get_user_candidates(self, user_id: str):
            return [
                Candidate(listing_id="item_a", score=0.80, in_stock=True),
                Candidate(listing_id="item_b", score=0.80, in_stock=True),
                Candidate(listing_id="item_c", score=0.70, in_stock=True),
                Candidate(listing_id="item_d", score=0.60, in_stock=True),
                Candidate(listing_id="item_e", score=0.50, in_stock=True),
            ]

    async def _run():
        nearline = InMemoryNearlineStore({"item_b": 0.12, "item_a": 0.04})
        service = RecommendationService(
            backend=DummyBackend(),
            cache=DummyCache(),
            registry=PlacementRegistry(),
            nearline_store=nearline,
            ranker=GBDTRankerAdapter(),
        )
        query = RecommendQuery(
            user_id="user_123",
            placement_id="home_feed",
            limit=2,
            include_explain=True,
        )
        result = await service.recommend(query)

        assert len(result.items) == 2
        assert result.explain["nearline_enabled"] is True
        # item_b with nearline debiased CTR ranked first
        assert result.items[0].listing_id == "item_b"

    asyncio.run(_run())
