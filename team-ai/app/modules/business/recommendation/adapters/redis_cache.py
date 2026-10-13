import time
from collections.abc import Callable

from loguru import logger
from pydantic import BaseModel, TypeAdapter, ValidationError
from redis.asyncio import Redis

from app.modules.business.recommendation.contract.keys import ServingKeys
from app.modules.business.recommendation.schemas import Candidate

_POINTER_TTL_S = 5.0


class _Entry(BaseModel):
    listing_id: str
    score: float | None = None
    in_stock: bool = True


# A list value is either an array of listing ids or an array of ``{"listing_id", "score"}``.
_LIST = TypeAdapter(list[str | _Entry])


class RedisServingCache:
    """Reads the producers' precomputed lists of the serving generation (decoded responses).

    The serving pointer is memoised for 5 s. Every read is fail-open: a Redis error, a
    missing key or a malformed value is a miss, so a cache problem never fails a request.
    """

    def __init__(
        self,
        redis: Redis,
        keys: ServingKeys,
        *,
        clock: Callable[[], float] = time.monotonic,
        pointer_ttl_s: float = _POINTER_TTL_S,
    ) -> None:
        self._redis = redis
        self._keys = keys
        self._clock = clock
        self._pointer_ttl_s = pointer_ttl_s
        self._pointer: str | None = None
        self._pointer_at: float | None = None

    async def serving_generation(self) -> str | None:
        """The serving generation, memoised; a Redis error counts as absent."""
        now = self._clock()
        if (
            self._pointer_at is not None
            and now - self._pointer_at < self._pointer_ttl_s
        ):
            return self._pointer
        try:
            pointer = await self._redis.get(self._keys.serving) or None
        except Exception as exc:
            logger.warning("recs.cache.serving_pointer_failed err={}", exc)
            pointer = None
        self._pointer = pointer
        self._pointer_at = now
        return pointer

    async def user_candidates(
        self, user_id: str, generation: str | None
    ) -> list[Candidate]:
        if not user_id:
            return []
        return await self._read(self._keys.user(user_id, generation))

    async def popular_candidates(self, generation: str | None) -> list[Candidate]:
        return await self._read(self._keys.popular(generation))

    async def model_version(self, generation: str | None) -> str | None:
        """The serving generation names the model; the legacy key serves older producers."""
        if generation:
            return generation
        try:
            return await self._redis.get(self._keys.model_version) or None
        except Exception as exc:
            logger.warning("recs.cache.get_model_version_failed err={}", exc)
            return None

    async def ranker_artifact(self, generation: str) -> str | None:
        key = self._keys.ranker(generation)
        try:
            return await self._redis.get(key) or None
        except Exception as exc:
            logger.warning("recs.cache.read_failed key={} err={}", key, exc)
            return None

    async def _read(self, key: str) -> list[Candidate]:
        try:
            raw = await self._redis.get(key)
        except Exception as exc:
            logger.warning("recs.cache.read_failed key={} err={}", key, exc)
            return []
        if not raw:
            return []
        try:
            entries = _LIST.validate_json(raw)
        except ValidationError:
            logger.warning("recs.cache.malformed key={}", key)
            return []
        return _candidates(entries)


def _candidates(entries: list[str | _Entry]) -> list[Candidate]:
    # A plain id has no score: the stored order is kept by scoring it descending by position.
    out: list[Candidate] = []
    for position, entry in enumerate(entries):
        fallback_score = float(len(entries) - position)
        if isinstance(entry, str):
            if entry:
                out.append(Candidate(listing_id=entry, score=fallback_score))
        elif entry.listing_id:
            score = fallback_score if entry.score is None else entry.score
            out.append(
                Candidate(
                    listing_id=entry.listing_id, score=score, in_stock=entry.in_stock
                )
            )
    return out
