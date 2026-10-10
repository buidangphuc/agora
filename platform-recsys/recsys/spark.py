"""SparkSession construction — local mode by default, cluster via env.

Local dev / CI use ``local[*]`` (no cluster, no YARN/k8s executors); a real
submit overrides SPARK_MASTER.
"""

from __future__ import annotations

from .config import Settings


def build_spark(settings: Settings):
    """Build a SparkSession. Imports pyspark lazily so config/tests load without it."""
    from pyspark.sql import SparkSession  # noqa: PLC0415 — lazy: keep host import-light

    builder = (
        SparkSession.builder.appName(settings.spark_app_name).master(settings.spark_master)
        # Small, deterministic shuffle for the local/CI sample; a cluster submit
        # can override via --conf.
        .config("spark.sql.shuffle.partitions", "8")
    )
    spark = builder.getOrCreate()
    spark.sparkContext.setLogLevel("WARN")
    return spark
