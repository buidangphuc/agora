from collections.abc import Sequence
from typing import Any, Protocol

from app.modules.business.recommendation.schemas import Candidate


class CandidateSource(Protocol):
    async def similar(
        self, seed_listing_id: str, *, top_k: int, generation: str | None
    ) -> list[Candidate]:
        """Candidates near the seed listing's vector, within the request's time budget.

        Empty when the seed has no vector, the budget is spent or the backend fails.
        """
        ...

    async def ready(self, generation: str | None) -> bool:
        """Whether the backend matches the contract of the producers."""
        ...


class ServingCache(Protocol):
    """The producers' precomputed lists of one serving generation. Reads fail open: an
    unreadable or malformed value is a miss (an empty list or ``None``)."""

    async def serving_generation(self) -> str | None: ...

    async def user_candidates(
        self, user_id: str, generation: str | None
    ) -> list[Candidate]: ...

    async def popular_candidates(self, generation: str | None) -> list[Candidate]: ...

    async def model_version(self, generation: str | None) -> str | None: ...

    async def ranker_artifact(self, generation: str) -> str | None: ...


class FeatureStore(Protocol):
    async def item_features(
        self, listing_ids: Sequence[str]
    ) -> dict[str, dict[str, Any]]:
        """Online feature rows by listing id; items without a row are absent. Never raises."""
        ...


class NearlineSignals(Protocol):
    async def debiased_ctrs(self, listing_ids: Sequence[str]) -> dict[str, float]:
        """Position-debiased CTR by listing id; items with no usable data are absent.
        Never raises, and returns within its own time budget."""
        ...
