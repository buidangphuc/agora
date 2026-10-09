"""Nearline streaming signal aggregator and real-time store for recommendations.

Implements real-time recents, category affinity, co-views, and position-debiased CTR (IPS).

Redis key layout (the contract team-ai reads; none of it is generation-scoped, none of it is touched
by publish, rollback or retention, and every key carries the nearline TTL):

- ``recs:nearline:user:{actor}:items``   ZSET  member listing_id, score event epoch seconds; the 50
  most recent. Read newest first with ZREVRANGE.
- ``recs:nearline:user:{actor}:cats``    HASH  category -> float affinity (view 1, click 2, add_to_cart 5).
- ``recs:nearline:coview:{listing_id}``  ZSET  member other listing_id, score co-view count; the top 50.
- ``recs:nearline:ctr:{listing_id}``     HASH  ``clicks_ips`` (clicks weighted by position**0.5) and
  ``imprs_ips`` (impression + view count, unweighted); CTR = clicks_ips / imprs_ips, capped at 1.
- ``recs:nearline:session:{session}:items`` ZSET  internal: a session's recent listings (co-view source).
- ``recs:nearline:seen:{event_id}``      STRING internal: replay guard (``SET NX EX``).

``actor`` is the warehouse ``user_key`` (the principal's id for a signed-in user, else
``anon:<anonymous_id>``); an event with neither falls back to its session id.
"""

from __future__ import annotations

import logging
import math
import time
from collections.abc import Callable
from dataclasses import dataclass, field
from typing import Any

logger = logging.getLogger(__name__)

# Key Prefixes
USER_RECENTS_PREFIX = "recs:nearline:user:"
ITEM_COVIEW_PREFIX = "recs:nearline:coview:"
ITEM_CTR_PREFIX = "recs:nearline:ctr:"
SESSION_PREFIX = "recs:nearline:session:"
SEEN_PREFIX = "recs:nearline:seen:"
DEFAULT_WINDOW_SECONDS = 86400  # 24 hours
MAX_RECENT_ITEMS = 50
MAX_COVIEWS_PER_ITEM = 50
# A new listing is co-viewed with the last few distinct listings of its session.
COVIEW_LOOKBACK = 3
# A redelivered event (an uncommitted batch replayed after a crash) is applied once within this time.
SEEN_TTL_SECONDS = 900

# Events that say what the actor looks at (the others only feed CTR, or nothing).
ENGAGEMENT_EVENTS = ("view", "click", "add_to_cart")
CATEGORY_WEIGHTS = {"view": 1.0, "click": 2.0, "add_to_cart": 5.0}


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
    event_id: str = ""


def _text(value: Any) -> str:
    return value.decode("utf-8") if isinstance(value, bytes) else str(value)


class NearlineSignalStore:
    """Reads real-time nearline session signals from Redis (or in-memory fallback)."""

    def __init__(self, redis_client: Any | None = None) -> None:
        self.redis = redis_client
        self._in_memory_recents: dict[str, list[tuple[str, float]]] = {}
        self._in_memory_categories: dict[str, dict[str, float]] = {}
        self._in_memory_coviews: dict[str, dict[str, float]] = {}
        self._in_memory_sessions: dict[str, list[str]] = {}
        self._in_memory_seen: set[str] = set()
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
                return [_text(i) for i in items]
            except Exception as exc:
                logger.warning("Redis get_recent_items error: %s", exc)
                return []
        else:
            entries = self._in_memory_recents.get(user_or_session_id, [])
            sorted_entries = sorted(entries, key=lambda x: -x[1])
            return [item_id for item_id, _ in sorted_entries[:limit]]

    def get_category_affinities(self, user_or_session_id: str) -> dict[str, float]:
        """Fetch top category scores for a user/session."""
        if not user_or_session_id:
            return {}

        if self.redis is not None:
            key = f"{USER_RECENTS_PREFIX}{user_or_session_id}:cats"
            try:
                raw_hash = self.redis.hgetall(key)
                return {_text(k): float(v) for k, v in raw_hash.items()}
            except Exception as exc:
                logger.warning("Redis get_category_affinities error: %s", exc)
                return {}
        else:
            return dict(self._in_memory_categories.get(user_or_session_id, {}))

    def get_coview_counts(self, listing_id: str, limit: int = 10) -> list[tuple[str, float]]:
        """Top co-viewed listings of ``listing_id`` with their co-view counts, highest first."""
        if not listing_id:
            return []

        if self.redis is not None:
            key = f"{ITEM_COVIEW_PREFIX}{listing_id}"
            try:
                rows = self.redis.zrevrange(key, 0, limit - 1, withscores=True)
                return [(_text(member), float(score)) for member, score in rows]
            except Exception as exc:
                logger.warning("Redis get_coview_counts error: %s", exc)
                return []
        else:
            coview_map = self._in_memory_coviews.get(listing_id, {})
            return sorted(coview_map.items(), key=lambda x: -x[1])[:limit]

    def get_coviewed_items(self, listing_id: str, limit: int = 10) -> list[str]:
        """Fetch top co-viewed item IDs for a given listing."""
        return [item for item, _ in self.get_coview_counts(listing_id, limit)]

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
    """Consumes interaction events and updates nearline signal keys in Redis.

    Redis errors propagate: the consumer must not commit an offset for an event that was not applied.
    """

    def __init__(
        self,
        store: NearlineSignalStore,
        ttl_seconds: int = DEFAULT_WINDOW_SECONDS,
        clock: Callable[[], float] = time.time,
    ) -> None:
        self.store = store
        self.ttl = ttl_seconds
        self._clock = clock

    def process_interaction(self, event: RawInteraction) -> bool:
        """Update recent items list, category affinity, co-views, and position-debiased CTR.

        Returns False when the event was ignored: no listing, older than the window, or already seen.
        """
        if not event.listing_id:
            return False
        if self._clock() - event.timestamp > self.ttl:
            return False
        if event.event_id and not self._first_delivery(event.event_id):
            return False

        self._update_ctr(event)
        if event.event_type in ENGAGEMENT_EVENTS:
            actor_id = event.user_id or event.session_id
            if actor_id:
                self._update_actor(event, actor_id)
        return True

    # ── internals ────────────────────────────────────────────────────────────
    def _first_delivery(self, event_id: str) -> bool:
        if self.store.redis is not None:
            ttl = min(SEEN_TTL_SECONDS, self.ttl)
            return bool(self.store.redis.set(f"{SEEN_PREFIX}{event_id}", 1, nx=True, ex=ttl))
        if event_id in self.store._in_memory_seen:
            return False
        self.store._in_memory_seen.add(event_id)
        return True

    def _update_ctr(self, event: RawInteraction) -> None:
        """Position-debiased CTR = clicks_ips / imprs_ips.

        Inverse propensity scoring: a click is weighted by 1 / P(seen at its position), here
        position ** gamma, so a click at a worse position counts more. An impression (or view) is
        plain exposure: it adds 1 to ``imprs_ips`` whatever its position. Weighting both by the same
        position factor would cancel and leave the raw CTR.
        """
        if event.event_type in ("impression", "view"):
            field_name, amount = "imprs_ips", 1.0
        elif event.event_type == "click":
            field_name, amount = "clicks_ips", compute_ips_weight(event.position)
        else:
            return
        if self.store.redis is not None:
            key_ctr = f"{ITEM_CTR_PREFIX}{event.listing_id}"
            pipe = self.store.redis.pipeline()
            pipe.hincrbyfloat(key_ctr, field_name, amount)
            pipe.expire(key_ctr, self.ttl)
            pipe.execute()
        elif field_name == "imprs_ips":
            curr = self.store._in_memory_debiased_impressions.get(event.listing_id, 0.0)
            self.store._in_memory_debiased_impressions[event.listing_id] = curr + amount
        else:
            curr = self.store._in_memory_debiased_clicks.get(event.listing_id, 0.0)
            self.store._in_memory_debiased_clicks[event.listing_id] = curr + amount

    def _update_actor(self, event: RawInteraction, actor_id: str) -> None:
        """Recents, category affinity and session co-views of one engagement event."""
        lid = event.listing_id
        ts = event.timestamp
        session_id = event.session_id or actor_id
        cat_weight = CATEGORY_WEIGHTS[event.event_type]

        if self.store.redis is not None:
            session_key = f"{SESSION_PREFIX}{session_id}:items"
            # The listings this session looked at just before this one (read first: the pipeline
            # below adds this one).
            recent = [_text(i) for i in self.store.redis.zrevrange(session_key, 0, COVIEW_LOOKBACK)]
            previous = [p for p in recent if p != lid][:COVIEW_LOOKBACK]

            pipe = self.store.redis.pipeline()
            key_items = f"{USER_RECENTS_PREFIX}{actor_id}:items"
            pipe.zadd(key_items, {lid: ts})
            pipe.zremrangebyrank(key_items, 0, -(MAX_RECENT_ITEMS + 1))
            pipe.expire(key_items, self.ttl)
            if event.category:
                key_cats = f"{USER_RECENTS_PREFIX}{actor_id}:cats"
                pipe.hincrbyfloat(key_cats, event.category, cat_weight)
                pipe.expire(key_cats, self.ttl)
            for prev in previous:
                for a, b in ((lid, prev), (prev, lid)):
                    key_co = f"{ITEM_COVIEW_PREFIX}{a}"
                    pipe.zincrby(key_co, 1.0, b)
                    pipe.zremrangebyrank(key_co, 0, -(MAX_COVIEWS_PER_ITEM + 1))
                    pipe.expire(key_co, self.ttl)
            pipe.zadd(session_key, {lid: ts})
            pipe.zremrangebyrank(session_key, 0, -(MAX_RECENT_ITEMS + 1))
            pipe.expire(session_key, self.ttl)
            pipe.execute()
            return

        recents = self.store._in_memory_recents.setdefault(actor_id, [])
        recents = [r for r in recents if r[0] != lid]
        recents.insert(0, (lid, ts))
        self.store._in_memory_recents[actor_id] = recents[:MAX_RECENT_ITEMS]

        if event.category:
            cats = self.store._in_memory_categories.setdefault(actor_id, {})
            cats[event.category] = cats.get(event.category, 0.0) + cat_weight

        session = self.store._in_memory_sessions.setdefault(session_id, [])
        previous = [p for p in session[:COVIEW_LOOKBACK] if p != lid]
        for prev in previous:
            co1 = self.store._in_memory_coviews.setdefault(lid, {})
            co1[prev] = co1.get(prev, 0.0) + 1.0
            co2 = self.store._in_memory_coviews.setdefault(prev, {})
            co2[lid] = co2.get(lid, 0.0) + 1.0
        session[:] = [lid] + [s for s in session if s != lid]
        del session[MAX_RECENT_ITEMS:]
