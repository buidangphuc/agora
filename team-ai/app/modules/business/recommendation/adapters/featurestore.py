import json
import time
from collections.abc import Callable, Sequence
from typing import Any

from redis.asyncio import Redis

from app.modules.business.recommendation.adapters.throttle import ThrottledWarning
from app.modules.business.recommendation.contract.keys import (
    featurestore_current_key,
    featurestore_row_key,
)

_CURRENT_TTL_S = 5.0


class RedisFeatureStore:
    """Reads one view's online rows written by platform-featurestore (decoded responses).

    ``fs:<view>:current`` names the version (memoised 5 s); rows are the flat JSON at
    ``fs:<view>:v<N>:<listing_id>``. A missing key gives no entry; any Redis error gives ``{}``
    for the whole batch (logged at most once a minute), so ranking degrades to running
    without features.
    """

    def __init__(
        self,
        redis: Redis,
        view: str,
        *,
        clock: Callable[[], float] = time.monotonic,
        current_ttl_s: float = _CURRENT_TTL_S,
    ) -> None:
        self._redis = redis
        self._view = view
        self._clock = clock
        self._current_ttl_s = current_ttl_s
        self._version: str | None = None
        self._version_at: float | None = None
        self._read_failed = ThrottledWarning(
            "recs.featurestore.read_failed", clock=clock
        )

    async def _current_version(self) -> str | None:
        now = self._clock()
        if (
            self._version_at is not None
            and now - self._version_at < self._current_ttl_s
        ):
            return self._version
        self._version = (
            await self._redis.get(featurestore_current_key(self._view)) or None
        )
        self._version_at = now
        return self._version

    async def item_features(
        self, listing_ids: Sequence[str]
    ) -> dict[str, dict[str, Any]]:
        if not listing_ids:
            return {}
        try:
            version = await self._current_version()
            if not version:
                return {}
            keys = [
                featurestore_row_key(self._view, version, listing_id)
                for listing_id in listing_ids
            ]
            values = await self._redis.mget(keys)
        except Exception as exc:
            self._read_failed.log(exc)
            return {}
        out: dict[str, dict[str, Any]] = {}
        for listing_id, raw in zip(listing_ids, values, strict=True):
            if not raw:
                continue
            try:
                row = json.loads(raw)
            except ValueError:
                continue
            if isinstance(row, dict):
                out[listing_id] = row
        return out
