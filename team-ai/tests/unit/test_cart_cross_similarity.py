import pytest
from fakeredis import aioredis

from app.modules.business.recommendation.schemas import RecommendationQuery
from tests.unit.recs_support import (
    Source,
    build_service,
    cand,
    popular_entry,
    seed_popular,
    write_placements,
)

_CROSS = """\
  cross:
    result_limit: 8
    candidate_ladder:
      - strategy: cart_cross_similarity
    ranking:
      model: cosine_rank
"""


def _ids(result):
    return [i.listing_id for i in result.items]


async def test_similar_listings_for_cart_lines_score_is_max_across_lines(tmp_path):
    source = Source(
        {
            "A": [cand("x", 0.9), cand("y", 0.5)],
            "B": [cand("x", 0.6), cand("y", 0.8), cand("z", 0.7)],
        }
    )
    service = build_service(source, placements=write_placements(tmp_path, _CROSS))
    result = await service.recommend(
        RecommendationQuery(placement_id="cross", context_listing_ids=["A", "B"])
    )
    assert [(i.listing_id, i.score) for i in result.items] == [
        ("x", pytest.approx(0.9)),
        ("y", pytest.approx(0.8)),
        ("z", pytest.approx(0.7)),
    ]
    assert sorted(source.similar_calls) == ["A", "B"]


async def test_results_are_at_most_the_placement_limit_in_descending_score(
    tmp_path,
):
    source = Source({"A": [cand(f"n{i}", 1.0 - i / 100) for i in range(20)]})
    service = build_service(source)
    result = await service.recommend(
        RecommendationQuery(placement_id="cart_cross_sell", context_listing_ids=["A"])
    )
    scores = [i.score for i in result.items]
    assert 0 < len(result.items) <= 8
    assert scores == sorted(scores, reverse=True)


async def test_unknown_context_listing_is_skipped(tmp_path):
    source = Source({"A": [cand("x", 0.9)]})
    service = build_service(source, placements=write_placements(tmp_path, _CROSS))
    result = await service.recommend(
        RecommendationQuery(placement_id="cross", context_listing_ids=["A", "unknown"])
    )
    assert _ids(result) == ["x"]


async def test_exactly_50_context_ids_are_served(tmp_path):
    ids = [f"c{i}" for i in range(50)]
    source = Source({"c49": [cand("x", 0.9)]})
    service = build_service(source, placements=write_placements(tmp_path, _CROSS))
    result = await service.recommend(
        RecommendationQuery(placement_id="cross", context_listing_ids=ids)
    )
    assert _ids(result) == ["x"]
    assert len(source.similar_calls) == 50


async def test_empty_context_ends_at_global_popular():
    redis = aioredis.FakeRedis(decode_responses=True)
    await seed_popular(redis, [popular_entry("p1", 0.9), popular_entry("p2", 0.8)])
    result = await build_service(Source(), redis).recommend(
        RecommendationQuery(placement_id="cart_cross_sell")
    )
    assert _ids(result) == ["p1", "p2"]


async def test_cart_items_and_out_of_stock_excluded():
    source = Source(
        {
            "A": [cand("B", 0.99), cand("x", 0.9), cand("oos", 0.8, in_stock=False)],
            "B": [cand("A", 0.99), cand("y", 0.5)],
        }
    )
    result = await build_service(source).recommend(
        RecommendationQuery(
            placement_id="cart_cross_sell", context_listing_ids=["A", "B"]
        )
    )
    assert sorted(_ids(result)) == ["x", "y"]


async def test_one_listing_per_seller_on_cart_cross_sell():
    source = Source(
        {
            "A": [
                cand("s1", 0.9, seller_id="S"),
                cand("s2", 0.8, seller_id="S"),
                cand("s3", 0.7, seller_id="S"),
                cand("t1", 0.6, seller_id="T"),
            ]
        }
    )
    result = await build_service(source).recommend(
        RecommendationQuery(placement_id="cart_cross_sell", context_listing_ids=["A"])
    )
    assert sorted(_ids(result)) == ["s1", "t1"]


async def test_seed_excluded_on_similar_items_when_it_is_its_own_neighbour():
    source = Source({"seed": [cand("seed", 1.0), cand("a", 0.9)]})
    result = await build_service(source).recommend(
        RecommendationQuery(placement_id="similar_items", seed_listing_id="seed")
    )
    assert _ids(result) == ["a"]


async def test_seed_is_eligible_when_exclude_seed_is_off(tmp_path):
    body = """\
  keep_seed:
    candidate_ladder:
      - strategy: seed_vector_similarity
    ranking:
      model: cosine_rank
    filters:
      exclude_seed: false
"""
    source = Source({"seed": [cand("seed", 1.0), cand("a", 0.9)]})
    service = build_service(source, placements=write_placements(tmp_path, body))
    result = await service.recommend(
        RecommendationQuery(placement_id="keep_seed", seed_listing_id="seed")
    )
    assert sorted(_ids(result)) == ["a", "seed"]
