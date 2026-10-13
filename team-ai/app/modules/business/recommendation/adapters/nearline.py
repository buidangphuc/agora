import asyncio
import time
from collections.abc import Callable, Sequence
from typing import Any

from redis.asyncio import Redis

from app.modules.business.recommendation.adapters.throttle import ThrottledWarning
from app.modules.business.recommendation.contract.keys import nearline_ctr_key


class RedisNearlineSignals:
    """Reads the position-debiased CTR the platform-recsys nearline consumer maintains.

    ``<prefix>:ctr:<listing_id>`` is a HASH with float fields ``clicks_ips`` and
    ``imprs_ips`` (inverse-propensity weighted click and impression sums over the TTL
    window). The debiased CTR is ``min(1, clicks_ips / imprs_ips)``; an item with fewer than
    ``min_impressions`` weighted impressions has no usable data and is left out, so ranking
    keeps its prior ``ctr_7d``. A missing key, a malformed value, a Redis error or a read
    beyond ``timeout_s`` gives no entry (a failure is logged at most once a minute): nearline
    never fails a request.
    """

    def __init__(
        self,
        redis: Redis,
        *,
        prefix: str,
        min_impressions: float,
        timeout_s: float,
        clock: Callable[[], float] = time.monotonic,
    ) -> None:
        self._redis = redis
        self._prefix = prefix
        self._min_impressions = min_impressions
        self._timeout_s = timeout_s
        self._read_failed = ThrottledWarning("recs.nearline.read_failed", clock=clock)

    async def debiased_ctrs(self, listing_ids: Sequence[str]) -> dict[str, float]:
        if not listing_ids:
            return {}
        try:
            async with asyncio.timeout(self._timeout_s):
                pipe = self._redis.pipeline()
                for listing_id in listing_ids:
                    pipe.hmget(
                        nearline_ctr_key(self._prefix, listing_id),
                        ["clicks_ips", "imprs_ips"],
                    )
                rows = await pipe.execute()
        except Exception as exc:  # includes TimeoutError
            self._read_failed.log(exc)
            return {}
        out: dict[str, float] = {}
        for listing_id, row in zip(listing_ids, rows, strict=True):
            ctr = self._ctr(row)
            if ctr is not None:
                out[listing_id] = ctr
        return out

    def _ctr(self, row: list[Any]) -> float | None:
        clicks_raw, imprs_raw = row
        if clicks_raw is None or imprs_raw is None:
            return None
        try:
            clicks, imprs = float(clicks_raw), float(imprs_raw)
        except ValueError:
            return None
        if imprs <= 0 or imprs < self._min_impressions or clicks < 0:
            return None
        return min(1.0, clicks / imprs)
