from fakeredis import aioredis

from app.modules.business.recommendation.schemas import RecommendationQuery
from tests.unit.recs_support import (
    Source,
    build_service,
    popular_entry,
    seed_popular,
    write_placements,
)

_CAT = """\
  cat:
    result_limit: 10
    candidate_ladder:
      - strategy: category_popular
      - strategy: global_popular
    ranking:
      model: cosine_rank
"""

_POPULAR = [
    popular_entry("c1-a", 0.9, category_id="C1"),
    popular_entry("c2-a", 0.8, category_id="C2"),
    popular_entry("c1-b", 0.7, category_id="C1"),
    popular_entry("d-a", 0.6, category_id="D"),
    popular_entry("none-a", 0.5),
]
_CATEGORIES = {"seed": "C1", "a": "C1", "b": "C2", "ctxd": "D"}


async def _serve(tmp_path, source, **query):
    redis = aioredis.FakeRedis(decode_responses=True)
    await seed_popular(redis, _POPULAR)
    service = build_service(source, redis, write_placements(tmp_path, _CAT))
    result = await service.recommend(RecommendationQuery(placement_id="cat", **query))
    return [i.listing_id for i in result.items]


async def test_seed_category(tmp_path):
    ids = await _serve(tmp_path, Source(categories=_CATEGORIES), seed_listing_id="seed")
    assert sorted(ids) == ["c1-a", "c1-b"]


async def test_seed_category_wins_over_context_categories(tmp_path):
    ids = await _serve(
        tmp_path,
        Source(categories=_CATEGORIES),
        seed_listing_id="seed",
        context_listing_ids=["ctxd"],
    )
    assert sorted(ids) == ["c1-a", "c1-b"]
    assert "d-a" not in ids


async def test_cart_categories(tmp_path):
    ids = await _serve(
        tmp_path, Source(categories=_CATEGORIES), context_listing_ids=["a", "b"]
    )
    assert sorted(ids) == ["c1-a", "c1-b", "c2-a"]


async def test_context_listing_without_category_contributes_nothing(tmp_path):
    ids = await _serve(
        tmp_path,
        Source(categories=_CATEGORIES),
        context_listing_ids=["a", "no-category"],
    )
    assert sorted(ids) == ["c1-a", "c1-b"]


async def test_listings_without_category_are_never_returned(tmp_path):
    ids = await _serve(tmp_path, Source(categories=_CATEGORIES), seed_listing_id="seed")
    assert "none-a" not in ids


async def test_no_seed_and_no_context_continues_to_global_popular(tmp_path):
    ids = await _serve(tmp_path, Source(categories=_CATEGORIES))
    assert ids == ["c1-a", "c2-a", "c1-b", "d-a", "none-a"]


async def test_seed_without_category_and_no_match_continues_to_global_popular(
    tmp_path,
):
    ids = await _serve(tmp_path, Source(categories={}), seed_listing_id="seed")
    assert sorted(ids) == sorted(e["listing_id"] for e in _POPULAR)


async def test_hidden_listings_of_the_category_never_appear(tmp_path):
    redis = aioredis.FakeRedis(decode_responses=True)
    await seed_popular(
        redis,
        [
            popular_entry("shown", 0.9, category_id="C1"),
            popular_entry("hidden", 0.8, category_id="C1", visible=False),
        ],
    )
    service = build_service(
        Source(categories=_CATEGORIES), redis, write_placements(tmp_path, _CAT)
    )
    result = await service.recommend(
        RecommendationQuery(placement_id="cat", seed_listing_id="seed")
    )
    assert [i.listing_id for i in result.items] == ["shown"]
