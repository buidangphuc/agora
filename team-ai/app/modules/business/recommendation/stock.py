import asyncio
import time
from collections.abc import Callable, Sequence

from loguru import logger

from app.modules.business.recommendation.ports import (
    StockEntry,
    StockLookup,
    StockLookupError,
)
from app.modules.business.recommendation.schemas import RecommendedItem

CHUNK_SIZE = 10
MAX_FIRST_PHASE = 20


class StockTimeout(StockLookupError):
    pass


class RequestStock:
    def __init__(
        self,
        lookup: StockLookup | None,
        budget_s: float,
        clock: Callable[[], float] = time.monotonic,
    ) -> None:
        self._lookup = lookup
        self._budget_s = budget_s
        self._clock = clock
        self._deadline: float | None = None
        self._known: dict[str, int | None] = {}
        self._requested = 0

    async def confirm(
        self, items: Sequence[RecommendedItem], limit: int
    ) -> list[RecommendedItem]:
        if self._lookup is None:
            raise StockLookupError("stock lookup is not configured")
        ids = [item.listing_id for item in items]
        first = min(limit * 2, MAX_FIRST_PHASE) if limit > 0 else MAX_FIRST_PHASE
        target = limit if limit > 0 else len(ids)
        outcome = "ok"
        self._requested = 0
        try:
            await self._fetch(ids[:first])
            position = min(first, len(ids))
            while self._in_stock(items[:position]) < target and position < len(ids):
                batch = ids[position : position + CHUNK_SIZE]
                position += len(batch)
                try:
                    await self._fetch(batch)
                except StockTimeout:
                    outcome = "timeout"
                    break
            kept = [i for i in items[:position] if self._stock_of(i) > 0][:target]
        except StockTimeout:
            outcome = "timeout"
            raise
        except StockLookupError:
            outcome = "error"
            raise
        finally:
            if self._requested or outcome != "ok":
                logger.debug(
                    "recs.stock.lookup outcome={} candidates={}",
                    outcome,
                    self._requested,
                )
        return [
            RecommendedItem(listing_id=i.listing_id, score=i.score, rank=rank)
            for rank, i in enumerate(kept, start=1)
        ]

    def _stock_of(self, item: RecommendedItem) -> int:
        return self._known.get(item.listing_id) or 0

    def _in_stock(self, items: Sequence[RecommendedItem]) -> int:
        return sum(1 for item in items if self._stock_of(item) > 0)

    def _remaining(self) -> float:
        now = self._clock()
        if self._deadline is None:
            self._deadline = now + self._budget_s
        return self._deadline - now

    async def _fetch(self, ids: Sequence[str]) -> None:
        need = [i for i in dict.fromkeys(ids) if i not in self._known]
        if not need:
            return
        self._requested += len(need)
        remaining = self._remaining()
        if remaining <= 0:
            raise StockTimeout("stock lookup budget spent")
        chunks = [need[i : i + CHUNK_SIZE] for i in range(0, len(need), CHUNK_SIZE)]
        try:
            results = await asyncio.wait_for(
                asyncio.gather(
                    *(self._call(chunk, remaining) for chunk in chunks),
                    return_exceptions=True,
                ),
                remaining,
            )
        except TimeoutError as exc:
            raise StockTimeout("stock lookup timed out") from exc
        failure: BaseException | None = None
        for result in results:
            if isinstance(result, BaseException):
                failure = failure or result
                continue
            for entry in result:
                self._known[entry.listing_id] = entry.stock
        if failure is not None:
            if isinstance(failure, StockLookupError):
                raise failure
            if isinstance(failure, TimeoutError):
                raise StockTimeout("stock lookup timed out") from failure
            raise StockLookupError("stock lookup failed") from failure

    async def _call(self, chunk: list[str], timeout_s: float) -> list[StockEntry]:
        assert self._lookup is not None
        entries = await self._lookup.lookup(chunk, timeout_s=timeout_s)
        if [e.listing_id for e in entries] != chunk:
            raise StockLookupError("malformed stock response")
        return entries
