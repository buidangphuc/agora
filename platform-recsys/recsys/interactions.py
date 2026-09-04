"""tracking_events → implicit-feedback (user, item, weight) triples.

- user_id = principal_id when authenticated, else anonymous_id (weights.choose_user_key)
- item_id = listing_id (rows with an empty listing_id are dropped)
- weight  = event_type confidence (weights.event_weight), optionally recency-decayed,
            then summed per (user, item)

ALS needs integer factor ids, so user/item strings are indexed with a
StringIndexer; the reverse label maps are returned so the outputs can be
relabelled back to the original ids.
"""

from __future__ import annotations

from dataclasses import dataclass

from . import weights as W
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


def build_triples(df, settings: Settings):
    """Collapse tracking rows to weighted per-(user,item) interactions.

    Returns a DataFrame[user_key:string, listing_id:string, weight:double].
    Pure-column logic (no UDF) so it stays Catalyst-optimizable; the weight map
    and user-key rule match recsys.weights exactly.
    """
    from pyspark.sql import functions as F  # noqa: PLC0415

    # user_key = principal_id when non-empty, else anonymous_id; else null → drop.
    principal = F.trim(F.coalesce(F.col("principal_id"), F.lit("")))
    anon = F.trim(F.coalesce(F.col("anonymous_id"), F.lit("")))
    user_key = F.when(principal != "", principal).otherwise(F.when(anon != "", anon).otherwise(F.lit(None)))

    listing = F.trim(F.coalesce(F.col("listing_id"), F.lit("")))

    # event_type → base weight via a mapping expression built from the config map,
    # defaulting to the unknown-event floor (matches weights.event_weight).
    etype = F.lower(F.trim(F.coalesce(F.col("event_type"), F.lit(""))))
    weight_expr = F.lit(float(W.UNKNOWN_EVENT_WEIGHT))
    for name, w in settings.event_weights.items():
        weight_expr = F.when(etype == F.lit(name), F.lit(float(w))).otherwise(weight_expr)

    mapped = (
        df.withColumn("user_key", user_key)
        .withColumn("listing_id", listing)
        .withColumn("base_weight", weight_expr)
        .filter(F.col("user_key").isNotNull())
        .filter(F.col("listing_id") != "")
    )

    # Optional recency decay by occurred_at (half-life in days; 0 disables).
    if settings.recency_half_life_days > 0 and "occurred_at" in df.columns:
        age_days = F.datediff(F.current_timestamp(), F.col("occurred_at")).cast("double")
        decay = F.pow(
            F.lit(0.5), F.greatest(age_days, F.lit(0.0)) / F.lit(float(settings.recency_half_life_days))
        )
        mapped = mapped.withColumn("weight_contrib", F.col("base_weight") * decay)
    else:
        mapped = mapped.withColumn("weight_contrib", F.col("base_weight"))

    triples = (
        mapped.groupBy("user_key", "listing_id")
        .agg(F.sum("weight_contrib").alias("weight"))
        .filter(F.col("weight") > 0)
    )
    return triples


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
