"""Local-mode ALS train→load smoke test against in-memory store fakes.

Gated on PySpark + qdrant-client + redis being importable (all Docker/CI). It
fits a tiny ALS model on the sample interactions and asserts the two Qdrant
collections and the Redis keys are populated and stamped with the run's
model_version — the backend integration assertion from the spec, run without
real containers.
"""

import pytest

pytest.importorskip("pyspark")
pytest.importorskip("qdrant_client")
pytest.importorskip("redis")

from datetime import datetime, timezone  # noqa: E402

from recsys import recommend  # noqa: E402
from recsys.config import load_settings  # noqa: E402
from recsys.interactions import build_triples, index_interactions  # noqa: E402
from recsys.load import qdrant as qdrant_load  # noqa: E402
from recsys.load import redis_cache  # noqa: E402
from recsys.train import train_als  # noqa: E402
from tests.fakes import FakeQdrantClient, FakeRedis  # noqa: E402


@pytest.fixture(scope="module")
def spark():
    from pyspark.sql import SparkSession

    try:
        s = SparkSession.builder.appName("recsys-smoke").master("local[1]").getOrCreate()
    except Exception as exc:  # no JVM / Java runtime on the host → Docker/CI only
        pytest.skip(f"Spark could not start (no Java runtime?): {exc}")
    s.sparkContext.setLogLevel("ERROR")
    yield s
    s.stop()


def test_train_and_load_populates_artifacts(spark):
    now = datetime.now(timezone.utc)
    rows = []
    # Build sample rows inline (avoid importing the packaged sample module path).
    interactions = [
        ("user-1", "", "listing-a", "view"),
        ("user-1", "", "listing-a", "click"),
        ("user-1", "", "listing-b", "view"),
        ("user-2", "", "listing-a", "view"),
        ("user-2", "", "listing-b", "click"),
        ("", "anon-1", "listing-b", "view"),
        ("", "anon-1", "listing-c", "click"),
        ("user-3", "", "listing-c", "add_to_cart"),
        ("user-3", "", "listing-a", "view"),
    ]
    for pid, anon, listing, etype in interactions:
        rows.append((etype, listing, anon, pid, now))
    df = spark.createDataFrame(
        rows, ["event_type", "listing_id", "anonymous_id", "principal_id", "occurred_at"]
    )

    settings = load_settings(environ={"ALS_RANK": "8", "ALS_MAX_ITER": "3", "TOP_N": "5"})
    model_version = "als-test-1"

    triples = build_triples(df, settings)
    indexed = index_interactions(triples, settings)
    artifacts = train_als(indexed, settings)

    item_ids, item_vecs = [], []
    for r in artifacts.item_factors.collect():
        item_ids.append(r["listing_id"])
        item_vecs.append(recommend.l2_normalize(list(r["features"])))
    user_ids, user_vecs = [], []
    for r in artifacts.user_factors.collect():
        user_ids.append(r["user_key"])
        user_vecs.append(recommend.l2_normalize(list(r["features"])))

    assert item_ids and user_ids  # model produced factors

    fake_q = FakeQdrantClient()
    item_rows = zip(item_ids, item_vecs, strict=False)
    user_rows = zip(user_ids, user_vecs, strict=False)
    counts = qdrant_load.load_vectors(settings, model_version, item_rows, user_rows, client=fake_q)
    assert counts["items"] == len(item_ids)
    assert counts["users"] == len(user_ids)
    # Collections exist and points are stamped with the model_version.
    item_pts = fake_q.collections[settings.qdrant_item_collection]
    assert item_pts and all(p["payload"]["model_version"] == model_version for p in item_pts.values())

    user_recs = recommend.top_n_for_users(user_ids, user_vecs, item_ids, item_vecs, settings.top_n)
    item_recs = recommend.similar_items(item_ids, item_vecs, settings.top_n)
    popular = recommend.popularity_ranking([(i, 1.0) for i in item_ids], settings.top_n)

    fake_r = FakeRedis()
    redis_cache.load_cache(settings, model_version, user_recs, item_recs, popular, client=fake_r)

    assert fake_r.store[settings.model_version_cache_key] == model_version
    assert any(k.startswith("recs:v1:user:") for k in fake_r.store)
    assert fake_r.store[settings.popular_cache_key]
