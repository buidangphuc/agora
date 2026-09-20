"""Unit tests for config-driven multi-placement recommendation engine (ADR-0012)."""

import asyncio

from app.modules.business.recommend.placement_config import PlacementRegistry
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
                Candidate(listing_id=f"vip_{i}", score=0.95, in_stock=True)
                for i in range(10)
            ]
        }

    async def get_user_candidates(self, user_id: str):
        return self.user_recs.get(user_id, [])

    async def get_popular_candidates(self):
        return []


def test_home_feed_personalized_tier1():
    async def _run():
        service = RecommendationService(
            backend=DummyBackend(),
            cache=DummyCache(),
            registry=PlacementRegistry(),
        )
        query = RecommendQuery(
            user_id="user_vip",
            placement_id="home_feed",
            limit=5,
            include_explain=True,
        )
        result = await service.recommend(query)

        assert len(result.items) == 5
        assert result.source == "cache"
        assert result.placement_id == "home_feed"
        assert result.fallback_tier == "tier1_personalized"
        assert result.status == "cached"
        assert result.explain["placement"] == "home_feed"
        assert result.explain["tier_chosen"] == "tier1_personalized"
        assert len(result.explain["ladder_traversed"]) >= 1

    asyncio.run(_run())


def test_home_feed_cold_start_fallback_tier4():
    async def _run():
        service = RecommendationService(
            backend=DummyBackend(),
            cache=DummyCache(),
            registry=PlacementRegistry(),
        )
        query = RecommendQuery(
            user_id="cold_user_999",
            placement_id="home_feed",
            limit=5,
            include_explain=True,
        )
        result = await service.recommend(query)

        assert len(result.items) == 5
        assert result.source == "popular"
        assert result.fallback_tier == "tier4_global_popular"
        assert result.status == "fallback"
        assert result.explain["status"] == "fallback"

    asyncio.run(_run())


def test_similar_items_placement():
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
        assert result.explain["strategy"] == "seed_vector_similarity"

    asyncio.run(_run())


def test_cart_cross_sell_placement():
    async def _run():
        service = RecommendationService(
            backend=DummyBackend(),
            cache=DummyCache(),
            registry=PlacementRegistry(),
        )
        query = RecommendQuery(
            cart_listing_ids=["seed_123"],
            placement_id="cart_cross_sell",
            limit=2,
            include_explain=True,
        )
        result = await service.recommend(query)

        assert len(result.items) == 2
        assert result.source == "ann"
        assert result.placement_id == "cart_cross_sell"
        assert result.status == "real"

    asyncio.run(_run())
