"""Redis pre-computed-list fast path (read-only, fail-open).

The ALS training job writes a ready-made Top-N list per user (and a global
popularity list) to Redis; team-ai only reads them. Every read is fail-open: a
Redis error, a missing key, or a malformed value is treated as a cache miss
(returns ``None``) so a cache problem never fails the RPC — the caller falls
back to the live Qdrant path.

Value schema (either form accepted):
  - a JSON array of listing-id strings, or
  - a JSON array of objects ``{"listing_id": str, "score": float, "in_stock": bool}``
"""

from __future__ import annotations

import json
from typing import TYPE_CHECKING, Any

from loguru import logger

from app.modules.business.recommend.schemas import Candidate

if TYPE_CHECKING:
    from redis.asyncio import Redis


class PrecomputedCache:
    def __init__(
        self,
        redis: Redis | None,
        *,
        prefix: str,
        schema_version: str,
    ) -> None:
        self._redis = redis
        self._prefix = prefix
        self._schema_version = schema_version

    def user_key(self, user_id: str) -> str:
        return f"{self._prefix}:{self._schema_version}:user:{user_id}"

    def popular_key(self) -> str:
        return f"{self._prefix}:{self._schema_version}:popular"

    async def get_user_candidates(self, user_id: str) -> list[Candidate] | None:
        if not user_id:
            return None
        return await self._read(self.user_key(user_id))

    async def get_popular_candidates(self) -> list[Candidate] | None:
        return await self._read(self.popular_key())

    async def _read(self, key: str) -> list[Candidate] | None:
        if self._redis is None:
            return None
        try:
            raw = await self._redis.get(key)
        except Exception as exc:  # fail-open: never fail the RPC on a cache error
            logger.warning("recs.cache.read_failed key={} err={}", key, exc)
            return None
        if not raw:
            return None
        return _parse_value(raw, key)


def _parse_value(raw: str, key: str) -> list[Candidate] | None:
    try:
        data = json.loads(raw)
    except (ValueError, TypeError):
        logger.warning("recs.cache.malformed key={}", key)
        return None
    if not isinstance(data, list):
        logger.warning("recs.cache.malformed key={} (not a list)", key)
        return None

    out: list[Candidate] = []
    # Descending rank order in the stored list -> descending synthetic score so
    # rank_and_filter preserves the training job's ordering after filtering.
    for position, entry in enumerate(data):
        cand = _entry_to_candidate(entry, position=position, total=len(data))
        if cand is not None:
            out.append(cand)
    return out or None


def _entry_to_candidate(entry: Any, *, position: int, total: int) -> Candidate | None:
    fallback_score = float(total - position)
    if isinstance(entry, str):
        return Candidate(listing_id=entry, score=fallback_score) if entry else None
    if isinstance(entry, dict):
        listing_id = str(entry.get("listing_id") or entry.get("id") or "")
        if not listing_id:
            return None
        score = entry.get("score")
        return Candidate(
            listing_id=listing_id,
            score=float(score) if score is not None else fallback_score,
            in_stock=bool(entry.get("in_stock", True)),
            category_id=str(entry.get("category_id", "")),
            seller_id=str(entry.get("seller_id", "")),
        )
    return None
