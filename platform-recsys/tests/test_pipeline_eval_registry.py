from datetime import datetime, timedelta, timezone
from unittest.mock import MagicMock

import pandas as pd

from recsys.config import Settings
from recsys.pipeline import run
from recsys.registry.metadata import ModelMetadata
from recsys.registry.registry import ModelRegistry


def _create_sample_df():
    now = datetime.now(timezone.utc)
    interactions = [
        ("u1", "l1", "view", now - timedelta(hours=9)),
        ("u1", "l1", "click", now - timedelta(hours=8)),
        ("u1", "l2", "view", now - timedelta(hours=7)),
        ("u1", "l3", "view", now - timedelta(hours=6)),
        ("u2", "l1", "view", now - timedelta(hours=5)),
        ("u2", "l2", "click", now - timedelta(hours=4)),
        ("u2", "l3", "view", now - timedelta(hours=3)),
        ("u3", "l2", "view", now - timedelta(hours=2)),
        ("u3", "l3", "add_to_cart", now - timedelta(hours=1)),
        ("u3", "l1", "view", now),
    ]
    return pd.DataFrame({
        "event_type": [x[2] for x in interactions],
        "principal_id": [x[0] for x in interactions],
        "anonymous_id": ["" for _ in interactions],
        "listing_id": [x[1] for x in interactions],
        "occurred_at": [x[3] for x in interactions],
    })


def test_pipeline_evaluates_and_promotes_initial_model(tmp_path, monkeypatch):
    """A pipeline run evaluates metrics and automatically promotes initial model."""
    parquet_file = tmp_path / "tracking_events.parquet"
    df = _create_sample_df()
    df.to_parquet(parquet_file, coerce_timestamps="ms", allow_truncated_timestamps=True)

    settings = Settings(
        spark_master="local[1]",
        warehouse_driver="duckdb",
        warehouse_parquet_path=str(parquet_file),
        als_max_iter=3,
        als_rank=4,
        top_n=5,
        redis_host="localhost",
        redis_port=6379,
    )

    registry = ModelRegistry()

    import recsys.load.qdrant as qdrant_load
    import recsys.load.redis_cache as redis_cache

    monkeypatch.setattr(qdrant_load, "load_vectors", lambda *args, **kwargs: {"items": 3, "users": 3})
    monkeypatch.setattr(redis_cache, "load_cache", lambda *args, **kwargs: {"items": 3, "users": 3})

    summary = run(settings=settings, registry=registry)

    assert summary["decision"] == "promoted"
    assert "metrics" in summary
    assert "ndcg@10" in summary["metrics"]
    assert summary["qdrant"]["items"] == 3


def test_pipeline_rejects_degraded_candidate_without_loading(tmp_path, monkeypatch):
    """A candidate with degraded NDCG is rejected by the gate and does not load to stores."""
    parquet_file = tmp_path / "tracking_events.parquet"
    df = _create_sample_df()
    df.to_parquet(parquet_file, coerce_timestamps="ms", allow_truncated_timestamps=True)

    settings = Settings(
        spark_master="local[1]",
        warehouse_driver="duckdb",
        warehouse_parquet_path=str(parquet_file),
        als_max_iter=3,
        als_rank=4,
        top_n=5,
        promotion_min_relative_improvement=0.10,
    )

    registry = ModelRegistry()
    # Seed an outstanding champion with NDCG=1.0
    champion = ModelMetadata(
        model_version="champ-v1",
        model_name="recsys-als",
        model_type="als",
        metrics={"ndcg@10": 1.0, "coverage@10": 1.0},
        status="champion",
    )
    registry.register_model(champion)
    registry._set_champion(champion)

    import recsys.load.qdrant as qdrant_load
    import recsys.load.redis_cache as redis_cache

    qdrant_mock = MagicMock()
    redis_mock = MagicMock()
    monkeypatch.setattr(qdrant_load, "load_vectors", qdrant_mock)
    monkeypatch.setattr(redis_cache, "load_cache", redis_mock)

    summary = run(settings=settings, registry=registry)

    assert summary["decision"] == "rejected"
    # Verify qdrant and redis loads were NOT called on rejected model
    qdrant_mock.assert_not_called()
    redis_mock.assert_not_called()
