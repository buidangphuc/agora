"""In-process ports for tests and local use: seeded data, no infrastructure."""

from collections.abc import Mapping, Sequence
from typing import Any

from app.modules.business.recommendation.schemas import Candidate


class MemoryCandidateSource:
    """Vector neighbours as seeded: ``similar[seed]`` is the candidate list of that seed."""

    def __init__(
        self, similar: Mapping[str, Sequence[Candidate]] | None = None
    ) -> None:
        self._similar = similar or {}

    async def similar(
        self, seed_listing_id: str, *, top_k: int, generation: str | None
    ) -> list[Candidate]:
        return list(self._similar.get(seed_listing_id, [])[:top_k])

    async def ready(self, generation: str | None) -> bool:
        return True


class MemoryFeatureStore:
    def __init__(self, rows: Mapping[str, dict[str, Any]] | None = None) -> None:
        self._rows = rows or {}

    async def item_features(
        self, listing_ids: Sequence[str]
    ) -> dict[str, dict[str, Any]]:
        return {lid: self._rows[lid] for lid in listing_ids if lid in self._rows}


class MemoryNearlineSignals:
    def __init__(self, ctrs: Mapping[str, float] | None = None) -> None:
        self._ctrs = ctrs or {}

    async def debiased_ctrs(self, listing_ids: Sequence[str]) -> dict[str, float]:
        return {
            lid: self._ctrs[lid] for lid in listing_ids if self._ctrs.get(lid, 0.0) > 0
        }
