import re
from collections.abc import Sequence
from typing import TYPE_CHECKING, Any, Protocol

from app.modules.business.ai_assistant.catalog import CATALOG
from app.modules.business.ai_assistant.schemas import ProductCard

if TYPE_CHECKING:
    from llama_index.core.schema import NodeWithScore
from app.modules.business.ai_assistant.text import as_int, strip_accents

_RAG_OVERFETCH = 3


class ListingSearch(Protocol):
    async def search(
        self, query: str, *, top_k: int | None = None
    ) -> Sequence["NodeWithScore"]: ...


class ProductRetriever(Protocol):
    async def retrieve(self, query: str, *, top_k: int) -> list[ProductCard]: ...


class RagProductRetriever:
    """Distinct listings from the RAG store, best match first, at most ``top_k``."""

    def __init__(self, search: ListingSearch, *, min_score: float) -> None:
        self._search = search
        self._min_score = min_score

    async def retrieve(self, query: str, *, top_k: int) -> list[ProductCard]:
        # Over-fetch: several chunks of one listing would otherwise eat the slots.
        nodes = await self._search.search(query, top_k=top_k * _RAG_OVERFETCH)
        best: dict[str, tuple[float, dict[str, Any]]] = {}
        for node in nodes:
            meta = node.node.metadata
            listing_id = str(
                meta.get("listing_id")
                or meta.get("document_id")
                or node.node.ref_doc_id
                or ""
            )
            score = float(node.score) if node.score is not None else 0.0
            if not listing_id or score < self._min_score:
                continue
            if listing_id not in best or score > best[listing_id][0]:
                best[listing_id] = (score, meta)
        ranked = sorted(best.items(), key=lambda kv: kv[1][0], reverse=True)
        return [
            ProductCard(
                listing_id=listing_id,
                title=str(meta.get("title", "")),
                price=as_int(meta.get("price")),
                currency=str(meta.get("currency") or "VND"),
                rating_text="",
            )
            for listing_id, (_, meta) in ranked[:top_k]
        ]


class DemoCatalogRetriever:
    """The built-in catalogue; only used when no RAG store is configured."""

    def __init__(self) -> None:
        pass

    async def retrieve(self, query: str, *, top_k: int) -> list[ProductCard]:
        norm_query = strip_accents(query)
        words = set(re.findall(r"\w+", norm_query))

        # Check for price constraint (e.g. "dưới 300k", "< 500000")
        max_price = _extract_max_price(norm_query)

        scored: list[tuple[float, dict[str, Any]]] = []
        for item in CATALOG:
            score = 0.0
            item_text = strip_accents(
                item["title"] + " " + " ".join(item.get("keywords", []))
            )

            for word in words:
                if len(word) >= 2 and word in item_text:
                    score += 2.0
                if word in strip_accents(item["title"]):
                    score += 3.0

            # Direct substring match boost
            if norm_query in item_text:
                score += 10.0

            # Price constraint filter / penalty
            if max_price is not None:
                if item["price"] <= max_price:
                    score += 4.0
                else:
                    score -= 10.0

            if score > 0:
                scored.append((score, item))

        scored.sort(key=lambda x: x[0], reverse=True)
        candidates = [x[1] for x in scored[:top_k]] if scored else CATALOG[:top_k]

        return [
            ProductCard(
                listing_id=c["listing_id"],
                title=c["title"],
                price=c["price"],
                currency=c.get("currency", "VND"),
                image_url=c.get("image_url", ""),
                discount_rate=c.get("discount_rate", 0),
                rating=c.get("rating", 4.8),
                rating_text=c.get("rating_text", "4.8/5"),
            )
            for c in candidates
        ]


def _extract_max_price(norm_query: str) -> int | None:
    # Matches patterns like "duoi 300k", "< 500k", "tam 200k"
    match_k = re.search(
        r"(?:duoi|tam|gia re|khoang|duoi muc|<|<=)\s*(\d+)\s*(?:k|nghin|ngan)",
        norm_query,
    )
    if match_k:
        return int(match_k.group(1)) * 1000
    match_vnd = re.search(r"(?:duoi|tam|<)\s*(\d{4,9})", norm_query)
    if match_vnd:
        return int(match_vnd.group(1))
    return None
