"""Nearline streaming signal aggregator and real-time store for recommendations.

Implements real-time recents, category affinity, co-views, and position-debiased CTR (IPS).
"""

from __future__ import annotations

import logging
import math
import time
from dataclasses import dataclass, field
from typing import Any

logger = logging.getLogger(__name__)

# Key Prefixes
USER_RECENTS_PREFIX = "recs:nearline:user:"
ITEM_COVIEW_PREFIX = "recs:nearline:coview:"
ITEM_CTR_PREFIX = "recs:nearline:ctr:"
DEFAULT_WINDOW_SECONDS = 86400  # 24 hours


def compute_ips_weight(position: int, gamma: float = 0.5) -> float:
    """Computes Inverse Propensity Score (IPS) weight for position debiasing."""
    pos = max(1, position)
    return float(math.pow(pos, gamma))


@dataclass
class RawInteraction:
    user_id: str
    listing_id: str
    event_type: str  # "view", "impression", "click", "add_to_cart"
    session_id: str = ""
    category: str = ""
    position: int = 1
    timestamp: float = field(default_factory=time.time)


class NearlineSignalStore:
    """Reads real-time nearline session signals from Redis (or in-memory fallback)."""

    def __init__(self, redis_client: Any | None = None) -> None:
        self.redis = redis_client
        self._in_memory_recents: dict[str, list[tuple[str, float]]] = {}
        self._in_memory_categories: dict[str, dict[str, float]] = {}
        self._in_memory_coviews: dict[str, dict[str, float]] = {}
        self._in_memory_debiased_clicks: dict[str, float] = {}
        self._in_memory_debiased_impressions: dict[str, float] = {}

    def get_recent_items(self, user_or_session_id: str, limit: int = 10) -> list[str]:
        """Fetch recently interacted item IDs for a user/session in reverse chronological order."""
        if not user_or_session_id:
            return []

        if self.redis is not None:
            key = f"{USER_RECENTS_PREFIX}{user_or_session_id}:items"
            try:
                items = self.redis.zrevrange(key, 0, limit - 1)
                return [
                    i.decode("utf-8") if isinstance(i, bytes) else str(i)
                    for i in items
                ]
            except Exception as exc:
                logger.warning("Redis get_recent_items error: %s", exc)
                return []
        else:
            entries = self._in_memory_recents.get(user_or_session_id, [])
            sorted_entries = sorted(entries, key=lambda x: -x[1])
            return [item_id for item_id, _ in sorted_entries[:limit]]

    def get_category_affinities(
        self, user_or_session_id: str
    ) -> dict[str, float]:
        """Fetch top category scores for a user/session."""
        if not user_or_session_id:
            return {}

        if self.redis is not None:
            key = f"{USER_RECENTS_PREFIX}{user_or_session_id}:cats"
            try:
                raw_hash = self.redis.hgetall(key)
                return {
                    (
                        k.decode("utf-8") if isinstance(k, bytes) else str(k)
                    ): float(v)
                    for k, v in raw_hash.items()
                }
            except Exception as exc:
                logger.warning("Redis get_category_affinities error: %s", exc)
                return {}
        else:
            return dict(self._in_memory_categories.get(user_or_session_id, {}))

    def get_coviewed_items(self, listing_id: str, limit: int = 10) -> list[str]:
        """Fetch top co-viewed item IDs for a given listing."""
        if not listing_id:
            return []

        if self.redis is not None:
            key = f"{ITEM_COVIEW_PREFIX}{listing_id}"
            try:
                items = self.redis.zrevrange(key, 0, limit - 1)
                return [
                    i.decode("utf-8") if isinstance(i, bytes) else str(i)
                    for i in items
                ]
            except Exception as exc:
                logger.warning("Redis get_coviewed_items error: %s", exc)
                return []
        else:
            coview_map = self._in_memory_coviews.get(listing_id, {})
            sorted_coviews = sorted(coview_map.items(), key=lambda x: -x[1])
            return [item for item, _ in sorted_coviews[:limit]]

    def get_debiased_ctr(self, listing_id: str) -> float:
        """Fetch position-debiased CTR using Inverse Propensity Scoring (IPS)."""
        if not listing_id:
            return 0.0

        if self.redis is not None:
            key = f"{ITEM_CTR_PREFIX}{listing_id}"
            try:
                raw = self.redis.hgetall(key)
                clicks_ips = float(raw.get(b"clicks_ips", raw.get("clicks_ips", 0.0)))
                imprs_ips = float(raw.get(b"imprs_ips", raw.get("imprs_ips", 0.0)))
                if imprs_ips <= 0:
                    return 0.0
                return min(1.0, clicks_ips / imprs_ips)
            except Exception as exc:
                logger.warning("Redis get_debiased_ctr error: %s", exc)
                return 0.0
        else:
            clicks = self._in_memory_debiased_clicks.get(listing_id, 0.0)
            imprs = self._in_memory_debiased_impressions.get(listing_id, 0.0)
            if imprs <= 0:
                return 0.0
            return min(1.0, clicks / imprs)


class NearlineSignalAggregator:
    """Consumes interaction events and updates nearline signal keys in Redis."""

    def __init__(
        self,
        store: NearlineSignalStore,
        ttl_seconds: int = DEFAULT_WINDOW_SECONDS,
    ) -> None:
        self.store = store
        self.ttl = ttl_seconds

    def process_interaction(self, event: RawInteraction) -> None:
        """Update recent items list, category affinity, co-views, and position-debiased CTR."""
        actor_id = event.user_id or event.session_id
        if not event.listing_id:
            return

        ips_weight = compute_ips_weight(event.position)
        ts = event.timestamp

        # 1. Update position-debiased CTR counters
        if event.event_type in ("impression", "view"):
            if self.store.redis is not None:
                key_ctr = f"{ITEM_CTR_PREFIX}{event.listing_id}"
                self.store.redis.hincrbyfloat(key_ctr, "imprs_ips", ips_weight)
                self.store.redis.expire(key_ctr, self.ttl)
            else:
                curr = self.store._in_memory_debiased_impressions.get(event.listing_id, 0.0)
                self.store._in_memory_debiased_impressions[event.listing_id] = curr + ips_weight

        elif event.event_type == "click":
            if self.store.redis is not None:
                key_ctr = f"{ITEM_CTR_PREFIX}{event.listing_id}"
                pipe = self.store.redis.pipeline()
                pipe.hincrbyfloat(key_ctr, "clicks_ips", ips_weight)
                pipe.hincrbyfloat(key_ctr, "imprs_ips", ips_weight)
                pipe.expire(key_ctr, self.ttl)
                pipe.execute()
            else:
                curr_c = self.store._in_memory_debiased_clicks.get(event.listing_id, 0.0)
                self.store._in_memory_debiased_clicks[event.listing_id] = curr_c + ips_weight
                curr_i = self.store._in_memory_debiased_impressions.get(event.listing_id, 0.0)
                self.store._in_memory_debiased_impressions[event.listing_id] = curr_i + ips_weight

        if not actor_id:
            return

        # Weight by event type for category preference
        cat_weight = 1.0
        if event.event_type == "click":
            cat_weight = 2.0
        elif event.event_type == "add_to_cart":
            cat_weight = 5.0

        # 2. Update User Recent Items & Category Affinities & Co-views
        if self.store.redis is not None:
            key_items = f"{USER_RECENTS_PREFIX}{actor_id}:items"
            pipe = self.store.redis.pipeline()
            pipe.zadd(key_items, {event.listing_id: ts})
            pipe.expire(key_items, self.ttl)

            if event.category:
                key_cats = f"{USER_RECENTS_PREFIX}{actor_id}:cats"
                pipe.hincrbyfloat(key_cats, event.category, cat_weight)
                pipe.expire(key_cats, self.ttl)

            pipe.execute()
        else:
            recents = self.store._in_memory_recents.setdefault(actor_id, [])
            recents = [r for r in recents if r[0] != event.listing_id]
            recents.insert(0, (event.listing_id, ts))
            self.store._in_memory_recents[actor_id] = recents[:50]

            if event.category:
                cats = self.store._in_memory_categories.setdefault(
                    actor_id, {}
                )
                cats[event.category] = cats.get(event.category, 0.0) + cat_weight

            # Co-view linking
            for prev_item, _ in recents[1:4]:
                co1 = self.store._in_memory_coviews.setdefault(
                    event.listing_id, {}
                )
                co1[prev_item] = co1.get(prev_item, 0.0) + 1.0
                co2 = self.store._in_memory_coviews.setdefault(prev_item, {})
                co2[event.listing_id] = co2.get(event.listing_id, 0.0) + 1.0
