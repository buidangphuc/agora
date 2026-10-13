"""The formats shared with platform-recsys and platform-featurestore, pinned inline."""

import uuid

import pytest

from app.modules.business.recommendation.contract.features import (
    GBDT_ARTIFACT_FORMAT,
    ITEM_POPULARITY_FEATURES,
    RANKING_FEATURES,
)
from app.modules.business.recommendation.contract.keys import (
    ServingKeys,
    featurestore_current_key,
    featurestore_row_key,
    nearline_ctr_key,
)
from app.modules.business.recommendation.contract.qdrant import (
    POINT_ID_NAMESPACE,
    generation_collection,
    point_id,
)
from app.modules.business.recommendation.ranking.rows import (
    item_feature,
    missing_item_features,
    popularity,
)

# uuid5(namespace, "listing-1") as platform-recsys recsys/load/qdrant.py computes it.
PRODUCER_POINT_ID = "25a4b2d5-6531-5357-90f2-06e92d1e1191"


def test_serving_keys_match_the_producer_layout() -> None:
    keys = ServingKeys("recs", "v1")
    assert keys.serving == "recs:v1:serving"
    assert keys.model_version == "recs:v1:model_version"
    assert keys.user("u1", "g1") == "recs:v1:gen:g1:user:u1"
    assert keys.popular("g1") == "recs:v1:gen:g1:popular"
    assert keys.ranker("g1") == "recs:v1:gen:g1:ranker"


def test_serving_keys_without_a_generation_are_the_legacy_unscoped_keys() -> None:
    keys = ServingKeys("recs", "v1")
    assert keys.user("u1", None) == "recs:v1:user:u1"
    assert keys.popular(None) == "recs:v1:popular"


def test_serving_keys_follow_the_configured_prefix_and_schema() -> None:
    keys = ServingKeys("rx", "v9")
    assert keys.serving == "rx:v9:serving"
    assert keys.user("u", "g") == "rx:v9:gen:g:user:u"


def test_featurestore_keys_match_the_producer_layout() -> None:
    assert featurestore_current_key("item_popularity") == "fs:item_popularity:current"
    assert (
        featurestore_row_key("item_popularity", "3", "L1") == "fs:item_popularity:v3:L1"
    )


def test_nearline_key_matches_the_producer_layout() -> None:
    assert nearline_ctr_key("recs:nearline", "L1") == "recs:nearline:ctr:L1"


def test_point_id_namespace_and_pin() -> None:
    assert uuid.UUID("6f7a1e2c-9b3d-4c5a-8e21-0d9f4a2b1c00") == POINT_ID_NAMESPACE
    assert point_id("listing-1") == PRODUCER_POINT_ID


def test_collection_naming() -> None:
    assert generation_collection("item_als_vectors", "g1") == "item_als_vectors__g1"
    assert generation_collection("item_als_vectors", None) == "item_als_vectors"


def test_item_popularity_features_pin() -> None:
    assert list(ITEM_POPULARITY_FEATURES) == [
        "views_7d",
        "clicks_7d",
        "add_to_cart_7d",
        "favorites_current",
        "review_count",
        "avg_rating",
        "ctr_7d",
    ]
    assert set(ITEM_POPULARITY_FEATURES.values()) == {0.0}


def test_ranking_features_pin() -> None:
    assert RANKING_FEATURES == (
        "item_popularity.views_7d",
        "item_popularity.clicks_7d",
        "item_popularity.add_to_cart_7d",
        "item_popularity.favorites_current",
        "item_popularity.review_count",
        "item_popularity.avg_rating",
        "item_popularity.ctr_7d",
        "item_attributes.price",
    )
    assert RANKING_FEATURES[:7] == tuple(
        f"item_popularity.{name}" for name in ITEM_POPULARITY_FEATURES
    )


def test_artifact_format_pin() -> None:
    assert GBDT_ARTIFACT_FORMAT == "agora-gbdt/1"


# The pre-registry ad-hoc names the ranker used to read; no view ever produced them.
OLD_NAMES = {
    "category_match": 1.0,
    "popularity_score": 99.0,
    "historical_ctr": 0.9,
    "conversion_rate": 0.9,
    "freshness_score": 1.0,
    "price": 1.0,
}


def test_old_names_are_not_features_and_count_as_missing() -> None:
    assert missing_item_features(OLD_NAMES) == list(ITEM_POPULARITY_FEATURES)
    assert popularity(OLD_NAMES) == 0.0


def test_missing_null_and_garbage_values_take_the_documented_default() -> None:
    row = {
        "views_7d": 5,
        "avg_rating": None,
        "ctr_7d": "n/a",
        "clicks_7d": float("nan"),
        "review_count": float("inf"),
    }
    assert item_feature(row, "views_7d") == (5.0, False)
    for name in ("avg_rating", "ctr_7d", "clicks_7d", "review_count"):
        assert item_feature(row, name) == (0.0, True)
    assert set(missing_item_features(row)) == set(ITEM_POPULARITY_FEATURES) - {
        "views_7d"
    }


def test_popularity_saturates_at_one() -> None:
    assert popularity({"views_7d": 1e12}) == 1.0
    assert popularity({}) == 0.0


@pytest.mark.parametrize("name", list(ITEM_POPULARITY_FEATURES))
def test_every_feature_has_a_numeric_default(name: str) -> None:
    assert item_feature({}, name) == (0.0, True)
