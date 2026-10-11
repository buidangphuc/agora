"""Config defaults, env overrides, validation, and cache-key contract."""

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


def test_promotion_force_is_off_by_default_and_parsed():
    assert config.load_settings(environ={}).promotion_force is False
    assert config.load_settings(environ={"PROMOTION_FORCE": "true"}).promotion_force is True
