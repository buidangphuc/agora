"""Feature names shared with platform-featurestore and the platform-recsys GBDT trainer.

``ITEM_POPULARITY_FEATURES`` mirrors ``item_popularity@v1`` in
``platform-featurestore/registry/features.yaml``: the online rows at
``fs:item_popularity:v<N>:<listing_id>``. Each name maps to the default used when a row
lacks it or holds a non-number.

``RANKING_FEATURES`` is ``recsys/ranker/contract.py`` ``RANKING_FEATURES``: ``<view>.<feature>``
names in the order of a trained artifact's ``features`` list. A loader accepts an artifact only
when its ``features`` equal this tuple.
"""

ITEM_POPULARITY_VIEW = "item_popularity"
ITEM_ATTRIBUTES_VIEW = "item_attributes"

# name -> default when missing. Order and names follow the registry entry.
ITEM_POPULARITY_FEATURES: dict[str, float] = {
    "views_7d": 0.0,
    "clicks_7d": 0.0,
    "add_to_cart_7d": 0.0,
    "favorites_current": 0.0,
    "review_count": 0.0,
    "avg_rating": 0.0,  # NULL in the view for an item without reviews
    "ctr_7d": 0.0,
}

GBDT_ARTIFACT_FORMAT = "agora-gbdt/1"

RANKING_FEATURES: tuple[str, ...] = (
    "item_popularity.views_7d",
    "item_popularity.clicks_7d",
    "item_popularity.add_to_cart_7d",
    "item_popularity.favorites_current",
    "item_popularity.review_count",
    "item_popularity.avg_rating",
    "item_popularity.ctr_7d",
    "item_attributes.price",
)
