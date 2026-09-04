"""SparkSession construction — local mode by default, cluster via env.

Local dev / CI use ``local[*]`` (no cluster, no YARN/k8s executors); a real
submit overrides SPARK_MASTER. The BigQuery connector jar is only needed for the
prod read path, so it is requested lazily.
"""

from __future__ import annotations

from .config import DRIVER_BIGQUERY, Settings

# Coordinates of the Spark BigQuery connector (prod read path only). Pinned so a
# spark-submit / Docker build resolves a reproducible jar.
BIGQUERY_CONNECTOR = "com.google.cloud.spark:spark-bigquery-with-dependencies_2.12:0.36.1"


def build_spark(settings: Settings):
    """Build a SparkSession. Imports pyspark lazily so config/tests load without it."""
    from pyspark.sql import SparkSession  # noqa: PLC0415 — lazy: keep host import-light

    builder = (
        SparkSession.builder.appName(settings.spark_app_name).master(settings.spark_master)
        # Small, deterministic shuffle for the local/CI sample; a cluster submit
        # can override via --conf.
        .config("spark.sql.shuffle.partitions", "8")
    )
    if settings.warehouse_driver == DRIVER_BIGQUERY:
        builder = builder.config("spark.jars.packages", BIGQUERY_CONNECTOR)
    spark = builder.getOrCreate()
    spark.sparkContext.setLogLevel("WARN")
    return spark
