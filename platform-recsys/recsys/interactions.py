"""Governed dataset → ALS-ready (user, item, weight) triples.

The dataset (platform-featurestore ``als_interactions``) already carries stitched
user keys and the final per-pair ``weight``; this module uses the weight AS GIVEN
(no event map, no decay) and only prunes sparse users/items.

ALS needs integer factor ids, so user/item strings are indexed with a
StringIndexer; the reverse label maps are returned so the outputs can be
relabelled back to the original ids.
"""

from __future__ import annotations

from dataclasses import dataclass

from .config import Settings


@dataclass
class IndexedInteractions:
    """The ALS-ready frame plus the reverse label maps.

    triples:    DataFrame[user_index:int, item_index:int, weight:double]
    user_labels: DataFrame[user_index:int, user_key:string]
    item_labels: DataFrame[item_index:int, listing_id:string]
    """

    triples: object
    user_labels: object
    item_labels: object


def dataset_triples(df):
    """Select the training triples from a governed dataset frame.

    Returns DataFrame[user_key:string, listing_id:string, weight:double]. Rows with an
    empty key or a non-positive weight carry no signal and are dropped; the weight
    itself is never altered.
    """
    from pyspark.sql import functions as F  # noqa: PLC0415

    return (
        df.select(
            F.trim(F.col("user_key")).alias("user_key"),
            F.trim(F.col("listing_id")).alias("listing_id"),
            F.col("weight").cast("double").alias("weight"),
        )
        .filter(F.col("user_key").isNotNull() & (F.col("user_key") != ""))
        .filter(F.col("listing_id").isNotNull() & (F.col("listing_id") != ""))
        .filter(F.col("weight") > 0)
    )


def _prune_sparse(triples, settings: Settings):
    from pyspark.sql import functions as F  # noqa: PLC0415

    if settings.min_interactions_per_user > 1:
        keep_users = (
            triples.groupBy("user_key").count().filter(F.col("count") >= settings.min_interactions_per_user)
        )
        triples = triples.join(keep_users.select("user_key"), "user_key", "inner")
    if settings.min_interactions_per_item > 1:
        keep_items = (
            triples.groupBy("listing_id").count().filter(F.col("count") >= settings.min_interactions_per_item)
        )
        triples = triples.join(keep_items.select("listing_id"), "listing_id", "inner")
    return triples


def index_interactions(triples, settings: Settings) -> IndexedInteractions:
    """StringIndex user_key/listing_id → integer ids; keep the reverse maps."""
    from pyspark.ml.feature import StringIndexer  # noqa: PLC0415
    from pyspark.sql import functions as F  # noqa: PLC0415

    triples = _prune_sparse(triples, settings)

    user_indexer = StringIndexer(inputCol="user_key", outputCol="user_index", handleInvalid="skip")
    item_indexer = StringIndexer(inputCol="listing_id", outputCol="item_index", handleInvalid="skip")

    user_model = user_indexer.fit(triples)
    indexed = user_model.transform(triples)
    item_model = item_indexer.fit(indexed)
    indexed = item_model.transform(indexed)

    indexed = indexed.withColumn("user_index", F.col("user_index").cast("int")).withColumn(
        "item_index", F.col("item_index").cast("int")
    )

    als_frame = indexed.select("user_index", "item_index", "weight")

    user_labels = indexed.select("user_index", "user_key").dropDuplicates(["user_index"])
    item_labels = indexed.select("item_index", "listing_id").dropDuplicates(["item_index"])

    return IndexedInteractions(triples=als_frame, user_labels=user_labels, item_labels=item_labels)
