"""Config defaults, env overrides, validation, and cache-key contract."""

import pytest

from recsys import config


def test_defaults_load_without_env():
    s = config.load_settings(environ={})
    assert s.dataset_dir == "/features/datasets/als_interactions/v1"
    assert s.dataset_path == ""
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


def test_dataset_settings_from_env():
    s = config.load_settings(environ={"DATASET_DIR": "/d", "DATASET_PATH": "/d/as_of=x.parquet"})
    assert (s.dataset_dir, s.dataset_path) == ("/d", "/d/as_of=x.parquet")


def test_raw_event_als_settings_are_gone():
    names = set(config.env_names())
    assert not names & {"WAREHOUSE_PARQUET_PATH", "INTERACTION_WINDOW_DAYS", "EVENT_WEIGHTS_JSON"}


def test_cache_key_shapes_match_consumer_contract():
    s = config.load_settings(environ={})
    assert s.user_cache_key("user-1") == "recs:v1:user:user-1"
    assert s.item_cache_key("listing-a") == "recs:v1:item:listing-a"
    assert s.popular_cache_key == "recs:v1:popular"
    assert s.model_version_cache_key == "recs:v1:model_version"


def test_promotion_settings_are_removed():
    # RAT-35 / task 1.2: the registry decides; no env var can force a promotion.
    names = set(config.env_names())
    assert not {n for n in names if n.startswith("PROMOTION_")}
    s = config.load_settings(environ={"PROMOTION_FORCE": "true"})
    assert not any(a.startswith("promotion_") for a in vars(s))


def test_mlflow_uri_required():
    # RAT-21: unset or empty is a configuration error (the job exits 2 before Spark).
    ok = config.load_settings(environ={"MLFLOW_TRACKING_URI": "http://mlflow:5050"})
    assert ok.mlflow_tracking_uri == "http://mlflow:5050"
    for env in ({}, {"MLFLOW_TRACKING_URI": ""}, {"MLFLOW_TRACKING_URI": "  "}):
        with pytest.raises(config.ConfigError):
            config.load_settings(environ=env).validate_for_training()


def test_max_dataset_age_bounds():
    # RAT-48 (negative), RAT-24 (0 = guard off, the default)
    assert config.load_settings(environ={}).max_dataset_age_minutes == 0
    assert config.load_settings(environ={"MAX_DATASET_AGE_MINUTES": "0"}).max_dataset_age_minutes == 0
    assert config.load_settings(environ={"MAX_DATASET_AGE_MINUTES": "1500"}).max_dataset_age_minutes == 1500
    for bad in ("-5", "abc", "1.5"):
        with pytest.raises(ValueError):
            config.load_settings(environ={"MAX_DATASET_AGE_MINUTES": bad})


def test_holdout_fraction_bounds():
    # RAT-44: strictly inside (0, 1); default 0.1
    assert config.load_settings(environ={}).two_tower_holdout_fraction == 0.1
    assert (
        config.load_settings(environ={"TWO_TOWER_HOLDOUT_FRACTION": "0.25"}).two_tower_holdout_fraction
        == 0.25
    )
    for bad in ("0", "1", "1.5", "-0.1"):
        with pytest.raises(ValueError):
            config.load_settings(environ={"TWO_TOWER_HOLDOUT_FRACTION": bad})
