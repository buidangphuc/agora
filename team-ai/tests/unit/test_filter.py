from typing import Any

from fakeredis import aioredis

from app.modules.business.recommendation.placements import Filters
from app.modules.business.recommendation.ranking.filter import eligible
from app.modules.business.recommendation.schemas import Candidate, RecommendationQuery
from tests.unit.recs_support import (
    FakeStock,
    Source,
    build_service,
    cand,
    popular_entry,
    seed_popular,
    write_placements,
)


def _ids(cands: Any) -> list[str]:
    return [c.listing_id for c in cands]


def _eligible(cands: Any, **kw: Any) -> list[Any]:
    filters = kw.pop("filters", Filters())
    return eligible(
        cands,
        filters=filters,
        seed_listing_id=kw.pop("seed", ""),
        cart_listing_ids=kw.pop("cart", ()),
    )


def test_filters_default_to_all_off():
    f = Filters()
    assert (
        f.in_stock_only,
        f.deduplicate_sellers,
        f.exclude_seed,
        f.exclude_cart_items,
    ) == (False, False, False, False)


def test_seed_excluded_when_exclude_seed_is_on():
    got = _eligible(
        [cand("seed"), cand("a")], seed="seed", filters=Filters(exclude_seed=True)
    )
    assert _ids(got) == ["a"]


def test_seed_eligible_when_exclude_seed_is_off():
    got = _eligible(
        [cand("seed"), cand("a")], seed="seed", filters=Filters(exclude_seed=False)
    )
    assert sorted(_ids(got)) == ["a", "seed"]


def test_cart_items_excluded_when_exclude_cart_items_is_on():
    got = _eligible(
        [cand("c1"), cand("c2"), cand("a")],
        cart=["c1", "c2"],
        filters=Filters(exclude_cart_items=True),
    )
    assert _ids(got) == ["a"]


def test_cart_items_eligible_when_exclude_cart_items_is_off():
    got = _eligible(
        [cand("c1"), cand("a")],
        cart=["c1"],
        filters=Filters(exclude_cart_items=False),
    )
    assert sorted(_ids(got)) == ["a", "c1"]


def test_out_of_stock_dropped_when_in_stock_only_is_on():
    got = _eligible(
        [cand("a", in_stock=False), cand("b")], filters=Filters(in_stock_only=True)
    )
    assert _ids(got) == ["b"]


def test_unknown_stock_never_passes_in_stock_only():
    got = _eligible(
        [Candidate("a", 1.0), Candidate("b", 0.9, in_stock=True)],
        filters=Filters(in_stock_only=True),
    )
    assert _ids(got) == ["b"]


def test_a_candidate_has_unknown_stock_by_default():
    assert Candidate("a", 1.0).in_stock is None


def test_out_of_stock_eligible_when_in_stock_only_is_off():
    got = _eligible(
        [cand("a", in_stock=False), cand("b")], filters=Filters(in_stock_only=False)
    )
    assert sorted(_ids(got)) == ["a", "b"]


def test_deduplicate_sellers_keeps_the_highest_scored_per_seller():
    cands = [
        cand("x1", 0.5, seller_id="S"),
        cand("x2", 0.9, seller_id="S"),
        cand("x3", 0.7, seller_id="S"),
        cand("y", 0.1, seller_id="T"),
    ]
    got = _eligible(cands, filters=Filters(deduplicate_sellers=True))
    assert sorted(_ids(got)) == ["x2", "y"]


def test_deduplicate_sellers_off_keeps_all_sellers_listings():
    cands = [cand(f"x{i}", 0.1 * i, seller_id="S") for i in range(1, 4)]
    got = _eligible(cands, filters=Filters(deduplicate_sellers=False))
    assert sorted(_ids(got)) == ["x1", "x2", "x3"]


def test_missing_seller_id_is_not_deduplicated():
    cands = [cand("n1", 0.9), cand("n2", 0.8), cand("s1", 0.7, seller_id="S")]
    got = _eligible(cands, filters=Filters(deduplicate_sellers=True))
    assert sorted(_ids(got)) == ["n1", "n2", "s1"]


def test_missing_seller_id_does_not_count_toward_any_seller():
    cands = [cand("n1", 0.9), cand("s1", 0.7, seller_id="S")]
    got = _eligible(cands, filters=Filters(deduplicate_sellers=True))
    assert sorted(_ids(got)) == ["n1", "s1"]


def test_hidden_listings_never_appear():
    got = _eligible([cand("a", visible=False), cand("b")], filters=Filters())
    assert _ids(got) == ["b"]


def test_hidden_listings_never_appear_with_every_filter_on():
    got = _eligible(
        [cand("a", visible=False), cand("b")],
        filters=Filters(
            in_stock_only=True,
            deduplicate_sellers=True,
            exclude_seed=True,
            exclude_cart_items=True,
        ),
    )
    assert _ids(got) == ["b"]


def test_a_duplicate_listing_id_keeps_its_best_score():
    got = _eligible([cand("a", 0.2), cand("a", 0.8), cand("b", 0.5)])
    assert sorted((c.listing_id, c.score) for c in got) == [("a", 0.8), ("b", 0.5)]


def test_an_empty_listing_id_is_dropped():
    assert _ids(_eligible([cand(""), cand("a")])) == ["a"]


_PLACEMENTS = """\
  p:
    result_limit: 10
    candidate_ladder:
      - strategy: global_popular
    ranking:
      model: cosine_rank
"""


async def test_hidden_listings_never_appear_in_the_response(tmp_path):
    redis = aioredis.FakeRedis(decode_responses=True)
    await seed_popular(
        redis,
        [
            popular_entry("ok", 0.9),
            popular_entry("hidden", 0.8, visible=False),
            popular_entry("unknown", 0.7, visible=None),
        ],
    )
    service = build_service(Source(), redis, write_placements(tmp_path, _PLACEMENTS))
    result = await service.recommend(RecommendationQuery(placement_id="p"))
    assert [i.listing_id for i in result.items] == ["ok"]


async def test_missing_visible_fails_closed_in_the_response(tmp_path):
    redis = aioredis.FakeRedis(decode_responses=True)
    await seed_popular(
        redis, [popular_entry("a", 0.9, visible=None), popular_entry("b", 0.8)]
    )
    service = build_service(Source(), redis, write_placements(tmp_path, _PLACEMENTS))
    result = await service.recommend(RecommendationQuery(placement_id="p"))
    assert [i.listing_id for i in result.items] == ["b"]


async def test_hidden_listings_never_appear_on_any_shipped_placement():
    for placement_id in ("home_feed", "similar_items", "cart_cross_sell"):
        redis = aioredis.FakeRedis(decode_responses=True)
        await seed_popular(
            redis,
            [popular_entry("ok", 0.9), popular_entry("hidden", 0.8, visible=False)],
        )
        source = Source({"seed": [cand("ok", 0.9), cand("hidden", 0.8, visible=False)]})
        service = build_service(source, redis, stock_lookup=FakeStock())
        result = await service.recommend(
            RecommendationQuery(
                placement_id=placement_id,
                seed_listing_id="seed",
                context_listing_ids=["seed"],
            )
        )
        assert "hidden" not in [i.listing_id for i in result.items], placement_id
