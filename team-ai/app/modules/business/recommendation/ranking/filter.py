from collections.abc import Iterable, Sequence

from app.modules.business.recommendation.placements import Filters
from app.modules.business.recommendation.schemas import Candidate

LEGACY_FILTERS = Filters(in_stock_only=True, exclude_seed=True)


def eligible(
    candidates: Iterable[Candidate],
    seed_listing_id: str = "",
    cart_listing_ids: Sequence[str] = (),
    *,
    filters: Filters = LEGACY_FILTERS,
) -> list[Candidate]:
    """The candidates every ranker may score: no empty id, visible, one per id, and what the
    placement's filters keep.

    A duplicate keeps its best score; ids keep the order they were first seen in. A candidate
    without a seller id is never deduplicated by seller.
    """
    cart = set(cart_listing_ids) if filters.exclude_cart_items else ()
    best: dict[str, Candidate] = {}
    for cand in candidates:
        if not cand.listing_id or not cand.visible:
            continue
        if filters.exclude_seed and cand.listing_id == seed_listing_id:
            continue
        if filters.in_stock_only and cand.in_stock is not True:
            continue
        if cand.listing_id in cart:
            continue
        prior = best.get(cand.listing_id)
        if prior is None or cand.score > prior.score:
            best[cand.listing_id] = cand
    if not filters.deduplicate_sellers:
        return list(best.values())
    top: dict[str, Candidate] = {}
    for cand in best.values():
        prior = top.get(cand.seller_id)
        if cand.seller_id and (prior is None or cand.score > prior.score):
            top[cand.seller_id] = cand
    return [c for c in best.values() if not c.seller_id or top[c.seller_id] is c]
