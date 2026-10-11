"""Spark-gated dataset-triple tests.

Skipped automatically when PySpark is not installed on the host (Spark runs in
Docker/CI). Asserts the ALS input uses the dataset weight exactly as given and drops
only rows that carry no signal.
"""

import pytest

pyspark = pytest.importorskip("pyspark")

from datetime import datetime  # noqa: E402

from recsys.interactions import dataset_triples  # noqa: E402


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


def _dataset(spark, rows):
    cols = ["user_key", "listing_id", "weight", "interactions", "last_occurred_at"]
    return spark.createDataFrame(rows, cols)


def test_dataset_triples_uses_weight_as_given_and_drops_no_signal_rows(spark):
    t = datetime(2026, 10, 5, 2, 0, 0)
    df = _dataset(
        spark,
        [
            ("user-1", "listing-a", 3.7, 4, t),  # kept verbatim (not re-weighted)
            ("user-1", "", 9.0, 1, t),  # dropped: empty listing
            ("", "listing-b", 9.0, 1, t),  # dropped: empty user
            ("user-2", "listing-b", 0.0, 1, t),  # dropped: no positive signal
            ("anon-9", "listing-b", 0.25, 1, t),
        ],
    )
    got = {(r["user_key"], r["listing_id"]): r["weight"] for r in dataset_triples(df).collect()}
    assert got == {("user-1", "listing-a"): 3.7, ("anon-9", "listing-b"): 0.25}


def test_timestamped_interactions_reads_timestamp_ntz(spark):
    """last_occurred_at may be TIMESTAMP_NTZ; the evaluation rows must still carry epoch
    seconds (a CAST to DOUBLE is an analysis error there)."""
    from pyspark.sql import functions as F

    from recsys.pipeline import timestamped_interactions

    df = _dataset(
        spark,
        [
            ("user-1", "listing-a", 1.0, 1, datetime(2026, 10, 5, 2, 34, 0)),
            ("anon-1", "", 1.0, 1, datetime(2026, 10, 5, 3, 0, 0)),  # no listing: dropped
            ("anon-2", "listing-b", 2.0, 1, datetime(2026, 10, 5, 4, 0, 0)),
        ],
    ).withColumn("last_occurred_at", F.col("last_occurred_at").cast("timestamp_ntz"))
    got = sorted(
        (r["user_id"], r["listing_id"], r["timestamp"]) for r in timestamped_interactions(df).collect()
    )
    assert [(u, lid) for u, lid, _ in got] == [("anon-2", "listing-b"), ("user-1", "listing-a")]
    assert all(ts > 1.7e9 for _, _, ts in got)
