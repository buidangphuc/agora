"""Spark-gated interaction-mapping tests.

Skipped automatically when PySpark is not installed on the host (Spark runs in
Docker/CI). Asserts the DataFrame pipeline matches the pure-Python rules: empty
listings dropped, principal-vs-anonymous user key, weights summed per (user,item).
"""

import pytest

pyspark = pytest.importorskip("pyspark")

from recsys.config import load_settings  # noqa: E402
from recsys.interactions import build_triples  # noqa: E402


@pytest.fixture(scope="module")
def spark():
    from pyspark.sql import SparkSession

    try:
        s = SparkSession.builder.appName("recsys-test").master("local[1]").getOrCreate()
    except Exception as exc:  # no JVM / Java runtime on the host → Docker/CI only
        pytest.skip(f"Spark could not start (no Java runtime?): {exc}")
    s.sparkContext.setLogLevel("ERROR")
    yield s
    s.stop()


def _events(spark, rows):
    cols = ["event_type", "listing_id", "anonymous_id", "principal_id", "occurred_at"]
    return spark.createDataFrame(rows, cols)


def test_build_triples_drops_empty_listing_and_sums_weight(spark):
    from datetime import datetime, timezone

    now = datetime.now(timezone.utc)
    rows = [
        ("view", "listing-a", "", "user-1", now),  # 1.0
        ("click", "listing-a", "", "user-1", now),  # 2.0  -> (user-1, listing-a) = 3.0
        ("view", "", "", "user-1", now),  # dropped: empty listing
        ("view", "listing-b", "anon-9", "", now),  # anonymous user key
    ]
    df = _events(spark, rows)
    settings = load_settings(environ={})
    triples = {(r["user_key"], r["listing_id"]): r["weight"] for r in build_triples(df, settings).collect()}

    assert triples[("user-1", "listing-a")] == 3.0
    assert ("user-1", "") not in triples  # empty listing dropped
    assert ("anon-9", "listing-b") in triples  # fell back to anonymous_id
