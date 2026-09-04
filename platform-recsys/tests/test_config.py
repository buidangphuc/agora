"""Config defaults, env overrides, validation, and cache-key contract."""

import pytest

from recsys import config


def test_defaults_load_without_env():
    s = config.load_settings(environ={})
    assert s.warehouse_driver == "duckdb"
    assert s.als_rank == 64
    assert s.top_n == 50
    # Contract literals shared with serve-recommendations-teamai.
    assert s.qdrant_item_collection == "item_als_vectors"
    assert s.cache_prefix == "recs"
    assert s.cache_schema_version == "v1"


def test_env_override():
    s = config.load_settings(environ={"ALS_RANK": "32", "TOP_N": "10", "QDRANT_URL": "http://qdrant:6333"})
    assert s.als_rank == 32
    assert s.top_n == 10
    assert s.qdrant_url == "http://qdrant:6333"


def test_event_weights_parsed_from_json():
    s = config.load_settings(environ={"EVENT_WEIGHTS_JSON": '{"view": 3, "PURCHASE": 9}'})
    assert s.event_weights["view"] == 3.0
    assert s.event_weights["purchase"] == 9.0  # keys lowercased


def test_cache_key_shapes_match_consumer_contract():
    s = config.load_settings(environ={})
    assert s.user_cache_key("user-1") == "recs:v1:user:user-1"
    assert s.item_cache_key("listing-a") == "recs:v1:item:listing-a"
    assert s.popular_cache_key == "recs:v1:popular"
    assert s.model_version_cache_key == "recs:v1:model_version"


def test_bigquery_requires_coordinates():
    with pytest.raises(ValueError):
        config.load_settings(environ={"WAREHOUSE_DRIVER": "bigquery", "BIGQUERY_PROJECT": ""})
    s = config.load_settings(
        environ={
            "WAREHOUSE_DRIVER": "bigquery",
            "BIGQUERY_PROJECT": "p",
            "BIGQUERY_DATASET": "analytics",
            "BIGQUERY_TABLE": "tracking_events",
        }
    )
    assert s.warehouse_driver == "bigquery"


def test_invalid_driver_rejected():
    with pytest.raises(ValueError):
        config.load_settings(environ={"WAREHOUSE_DRIVER": "postgres"})
