from collections.abc import Iterable

from app.modules.business.recommendation.schemas import Candidate


def eligible(candidates: Iterable[Candidate], seed_listing_id: str) -> list[Candidate]:
    """The candidates every ranker may score: no empty id, no seed, in stock, one per id.

    A duplicate keeps its best score; ids keep the order they were first seen in.
    """
    best: dict[str, Candidate] = {}
    for cand in candidates:
        if not cand.listing_id or cand.listing_id == seed_listing_id:
            continue
        if not cand.in_stock:
            continue
        prior = best.get(cand.listing_id)
        if prior is None or cand.score > prior.score:
            best[cand.listing_id] = cand
    return list(best.values())
