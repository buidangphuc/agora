"""Unit and execution-proof tests for config-driven multi-placement recommendation engine (ADR-0012).

Tests strict end-to-end wiring of GBDT ranking, FeatureStore enrichment, startup validation, and degradation.
"""

from __future__ import annotations

import asyncio

import pytest

from app.modules.business.recommend.placement_config import (
    PlacementConfig,
    PlacementRegistry,
)
from app.modules.business.recommend.ranking import (
    GBDTRankerAdapter,
    InMemoryFeatureStore,
)
from app.modules.business.recommend.schemas import Candidate, RecommendQuery
from app.modules.business.recommend.service import RecommendationService


class DummyBackend:
    def __init__(self):
        self.popular_items = [
            Candidate(listing_id=f"pop_{i}", score=0.5, in_stock=True)
            for i in range(10)
        ]
        self.similar_map = {
            "seed_123": [
                Candidate(listing_id="sim_1", score=0.9, in_stock=True),
                Candidate(listing_id="sim_2", score=0.8, in_stock=True),
                Candidate(listing_id="sim_3", score=0.7, in_stock=True),
                Candidate(listing_id="sim_4", score=0.6, in_stock=True),
                Candidate(listing_id="sim_5", score=0.5, in_stock=True),
            ]
        }

    async def retrieve_similar(self, seed_listing_id: str, top_k: int = 100):
        return self.similar_map.get(seed_listing_id, [])

    async def popular(self, top_k: int = 100):
        return self.popular_items


class DummyCache:
    def __init__(self):
        self.user_recs = {
            "user_vip": [
                # Item A: higher raw cosine (0.95), but no feature store metadata
                Candidate(listing_id="item_cos_high", score=0.95, in_stock=True, category_id="cat_other"),
                # Item B: lower raw cosine (0.80), but strong category match and high CTR in feature store
                Candidate(listing_id="item_gbdt_favored", score=0.80, in_stock=True, category_id="cat_target"),
                Candidate(listing_id="item_3", score=0.70, in_stock=True),
                Candidate(listing_id="item_4", score=0.60, in_stock=True),
                Candidate(listing_id="item_5", score=0.50, in_stock=True),
            ]
        }

    async def get_user_candidates(self, user_id: str):
        return self.user_recs.get(user_id, [])

    async def get_popular_candidates(self):
        return []


def test_home_feed_personalized_gbdt_and_featurestore_hit_count():
    async def _run():
        fs = InMemoryFeatureStore({
            "item_gbdt_favored": {
                "category_match": 1.0,
                "historical_ctr": 0.15,
                "conversion_rate": 0.08,
                "popularity_score": 90.0,
            },
            "item_cos_high": {
                "category_match": 0.0,
                "historical_ctr": 0.01,
                "conversion_rate": 0.005,
                "popularity_score": 10.0,
            },
        })

        service = RecommendationService(
            backend=DummyBackend(),
            cache=DummyCache(),
            registry=PlacementRegistry(),
            feature_store=fs,
            ranker=GBDTRankerAdapter(),
        )
        query = RecommendQuery(
            user_id="user_vip",
            placement_id="home_feed",
            category_id="cat_target",
            limit=2,
            include_explain=True,
        )
        result = await service.recommend(query)

        assert len(result.items) == 2
        assert result.placement_id == "home_feed"
        assert result.fallback_tier == "tier1_personalized"
        assert result.explain["ranking_model"] == "gbdt"
        assert result.explain["ranking_source"] == "gbdt"
        assert result.explain["featurestore_hit_count"] == 2

        # PROOF OF WIRING: GBDT elevates item_gbdt_favored above item_cos_high despite lower cosine
        assert result.items[0].listing_id == "item_gbdt_favored"
        assert result.items[1].listing_id == "item_cos_high"

    asyncio.run(_run())


def test_similar_items_placement_keeps_cosine_order():
    async def _run():
        service = RecommendationService(
            backend=DummyBackend(),
            cache=DummyCache(),
            registry=PlacementRegistry(),
        )
        query = RecommendQuery(
            seed_listing_id="seed_123",
            placement_id="similar_items",
            limit=3,
            include_explain=True,
        )
        result = await service.recommend(query)

        assert len(result.items) == 3
        assert result.source == "ann"
        assert result.placement_id == "similar_items"
        assert result.status == "real"
        assert result.explain["ranking_model"] == "cosine_rank"
        assert result.explain["ranking_source"] == "cosine"
        # Ordered by cosine similarity descending
        assert result.items[0].listing_id == "sim_1"
        assert result.items[1].listing_id == "sim_2"


def test_startup_validation_rejects_unbound_ranking_model():
    registry = PlacementRegistry()
    registry._placements["bad_placement"] = PlacementConfig(
        placement_id="bad_placement",
        name="Bad Placement",
        ranking_model="unsupported_deep_transformer",
    )

    with pytest.raises(ValueError, match="declares unbound ranking model 'unsupported_deep_transformer'"):
        registry.validate()


def test_ranker_failure_degrades_to_cosine_order():
    class FailingRanker:
        def rank_candidates(self, *args, **kwargs):
            raise RuntimeError("Model weight corrupted")

    async def _run():
        service = RecommendationService(
            backend=DummyBackend(),
            cache=DummyCache(),
            registry=PlacementRegistry(),
            ranker=FailingRanker(),
        )
        query = RecommendQuery(
            user_id="user_vip",
            placement_id="home_feed",
            limit=2,
            include_explain=True,
        )
        result = await service.recommend(query)

        assert len(result.items) == 2
        assert result.fallback_tier == "tier1_personalized"
        assert result.status == "degraded"
        assert result.explain["ranking_source"] == "degraded_cosine"
        # Reverts to cosine order
        assert result.items[0].listing_id == "item_cos_high"

    asyncio.run(_run())
