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
import time
from collections.abc import Callable
from contextvars import ContextVar, Token
from typing import TYPE_CHECKING, Any

from loguru import logger

from app.modules.business.recommend.schemas import Candidate

if TYPE_CHECKING:
    from redis.asyncio import Redis


_POINTER_TTL_S = 5.0

# The generation fixed for the request being served (see ``pin_generation``): ``(gen,)`` so
# "pinned to no generation" (legacy keys) is distinguishable from "not pinned".
_PINNED: ContextVar[tuple[str | None] | None] = ContextVar(
    "recs_pinned_generation", default=None
)


class PrecomputedCache:
    """Reads the pre-computed lists of the *serving generation*.

    ``{prefix}:{schema}:serving`` names the generation to serve; it is memoised
    per process for 5 s. When it is set, keys are ``{prefix}:{schema}:gen:<gen>:...``;
    when absent (or unreadable) the legacy unscoped keys are used.
    """

    def __init__(
        self,
        redis: Redis | None,
        *,
        prefix: str,
        schema_version: str,
        clock: Callable[[], float] = time.monotonic,
        pointer_ttl_s: float = _POINTER_TTL_S,
    ) -> None:
        self._redis = redis
        self._prefix = prefix
        self._schema_version = schema_version
        self._clock = clock
        self._pointer_ttl_s = pointer_ttl_s
        self._pointer: str | None = None
        self._pointer_at: float | None = None

    def _base(self, gen: str | None) -> str:
        base = f"{self._prefix}:{self._schema_version}"
        return f"{base}:gen:{gen}" if gen else base

    def serving_key(self) -> str:
        return f"{self._prefix}:{self._schema_version}:serving"

    def user_key(self, user_id: str, gen: str | None = None) -> str:
        return f"{self._base(gen)}:user:{user_id}"

    def popular_key(self, gen: str | None = None) -> str:
        return f"{self._base(gen)}:popular"

    def item_key(self, item_id: str, gen: str | None = None) -> str:
        return f"{self._base(gen)}:item:{item_id}"

    def model_version_key(self) -> str:
        return f"{self._prefix}:{self._schema_version}:model_version"

    async def pin_generation(self) -> Token:
        """Fix the serving generation for the current request (contextvar scope).

        Every cache read and every Qdrant collection choice made under the pin sees the
        same generation even if the 5 s memo expires and the pointer moves mid-request.
        Pass the returned token to ``unpin_generation`` in a ``finally``.
        """
        return _PINNED.set((await self._resolve_generation(),))

    def unpin_generation(self, token: Token) -> None:
        _PINNED.reset(token)

    async def serving_generation(self) -> str | None:
        """The serving generation (the request's pinned one if any)."""
        pinned = _PINNED.get()
        if pinned is not None:
            return pinned[0]
        return await self._resolve_generation()

    async def _resolve_generation(self) -> str | None:
        """The serving generation, memoised; a Redis error counts as absent."""
        if self._redis is None:
            return None
        now = self._clock()
        if (
            self._pointer_at is not None
            and now - self._pointer_at < self._pointer_ttl_s
        ):
            return self._pointer
        try:
            val = await self._redis.get(self.serving_key())
            if isinstance(val, bytes):
                val = val.decode("utf-8")
            pointer = str(val) if val else None
        except Exception as exc:
            logger.warning("recs.cache.serving_pointer_failed err={}", exc)
            pointer = None
        self._pointer = pointer
        self._pointer_at = now
        return pointer

    async def get_user_candidates(self, user_id: str) -> list[Candidate] | None:
        if not user_id:
            return None
        gen = await self.serving_generation()
        return await self._read(self.user_key(user_id, gen))

    async def get_popular_candidates(self) -> list[Candidate] | None:
        gen = await self.serving_generation()
        return await self._read(self.popular_key(gen))

    async def get_model_version(self) -> str | None:
        if self._redis is None:
            return None
        gen = await self.serving_generation()
        if gen:
            return gen
        try:
            val = await self._redis.get(self.model_version_key())
            if isinstance(val, bytes):
                return val.decode("utf-8")
            return str(val) if val is not None else None
        except Exception as exc:
            logger.warning("recs.cache.get_model_version_failed err={}", exc)
            return None

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
