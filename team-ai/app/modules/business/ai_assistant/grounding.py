import asyncio
import re
from dataclasses import dataclass, field
from typing import Final, Protocol

from loguru import logger

from app.modules.business.ai_assistant.adapters.listing import ListingFacts
from app.modules.business.ai_assistant.adapters.search import (
    SearchHitFacts,
    SearchOutcome,
)
from app.modules.business.ai_assistant.schemas import ProductCard
from app.modules.business.ai_assistant.text import strip_accents

MAX_CARDS: Final = 5

_PRICE_PHRASE = re.compile(
    r"(?:duoi|khong qua|toi da|under|below|max)\s*(\d+(?:[.,]\d+)*)\s*"
    r"(k|nghin|ngan|trieu|tr|m)?(?![a-z0-9])"
)
_UNIT_FACTOR: Final = {
    "k": 1_000,
    "nghin": 1_000,
    "ngan": 1_000,
    "trieu": 1_000_000,
    "tr": 1_000_000,
    "m": 1_000_000,
}


def parse_max_price(message: str) -> int | None:
    norm = strip_accents(message).replace("đ", "d")
    match = _PRICE_PHRASE.search(norm)
    if match is None:
        return None
    digits, unit = match.groups()
    try:
        if unit:
            value = float(digits.replace(",", ".")) * _UNIT_FACTOR[unit]
        else:
            value = float(re.sub(r"[.,]", "", digits))
    except ValueError:
        return None
    price = int(value)
    return price if price > 0 else None


class SearchPort(Protocol):
    async def search(
        self,
        query: str,
        *,
        max_price: int | None,
        page_size: int,
        timeout_s: float,
    ) -> SearchOutcome: ...


class ListingPort(Protocol):
    async def get(
        self, listing_id: str, *, timeout_s: float
    ) -> ListingFacts | None: ...


@dataclass(frozen=True, slots=True)
class GroundedCards:
    cards: list[ProductCard] = field(default_factory=list)
    search_model_version: str | None = None
    failed: bool = False


def _rating_text(hit: SearchHitFacts) -> str:
    if hit.review_count <= 0:
        return ""
    return f"{hit.rating_avg:.1f}/5 ({hit.review_count} đánh giá)"


class ListingGrounding:
    def __init__(
        self,
        *,
        search: SearchPort,
        listing: ListingPort,
        search_timeout_s: float,
        listing_timeout_s: float,
    ) -> None:
        self._search = search
        self._listing = listing
        self._search_timeout_s = search_timeout_s
        self._listing_timeout_s = listing_timeout_s

    async def ground(self, query: str) -> GroundedCards:
        max_price = parse_max_price(query)
        try:
            outcome = await asyncio.wait_for(
                self._search.search(
                    query,
                    max_price=max_price,
                    page_size=MAX_CARDS,
                    timeout_s=self._search_timeout_s,
                ),
                timeout=self._search_timeout_s,
            )
        except Exception as exc:
            logger.warning(
                "shopping_assistant.search_failed error={}: {}",
                type(exc).__name__,
                str(exc)[:200],
            )
            return GroundedCards(failed=True)

        hits = outcome.hits[:MAX_CARDS]
        results = await asyncio.gather(
            *(self._hydrate(hit) for hit in hits), return_exceptions=True
        )
        failures = [r for r in results if isinstance(r, BaseException)]
        for exc in failures:
            logger.warning(
                "shopping_assistant.listing_failed error={}: {}",
                type(exc).__name__,
                str(exc)[:200],
            )
        cards = [
            card
            for card in results
            if isinstance(card, ProductCard)
            and (max_price is None or card.price <= max_price)
        ]
        return GroundedCards(
            cards=cards,
            search_model_version=outcome.model_version or None,
            failed=bool(hits) and len(failures) == len(hits),
        )

    async def _hydrate(self, hit: SearchHitFacts) -> ProductCard | None:
        facts = await asyncio.wait_for(
            self._listing.get(hit.listing_id, timeout_s=self._listing_timeout_s),
            timeout=self._listing_timeout_s,
        )
        if facts is None or not facts.published:
            return None
        return ProductCard(
            listing_id=facts.listing_id,
            title=facts.title,
            price=facts.price,
            currency=facts.currency,
            rating=hit.rating_avg if hit.review_count > 0 else 0.0,
            rating_text=_rating_text(hit),
        )
