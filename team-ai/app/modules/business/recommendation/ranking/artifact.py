"""The trained GBDT ranker published by platform-recsys (artifact ``agora-gbdt/1``).

``recs:v1:gen:<generation>:ranker`` holds the LambdaMART as JSON. The loader reads the key of
the request's serving generation, accepts it only when its ``features`` equal
``RANKING_FEATURES``, parses it once per generation and scores in pure Python. Anything else
(absent key, malformed JSON, other format, other feature list) leaves the fixed weights in
charge, and a rejected artifact is logged once per load.
"""

import json
import time
from collections.abc import Callable, Iterable, Mapping
from typing import Any, Self

from loguru import logger
from pydantic import BaseModel, Field, ValidationError, model_validator

from app.modules.business.recommendation.contract.features import (
    GBDT_ARTIFACT_FORMAT,
    RANKING_FEATURES,
)
from app.modules.business.recommendation.ports import ServingCache
from app.modules.business.recommendation.ranking.filter import eligible
from app.modules.business.recommendation.ranking.rows import number
from app.modules.business.recommendation.schemas import Candidate, RecommendedItem

CTR_FEATURE = "item_popularity.ctr_7d"


class ArtifactError(ValueError):
    """The artifact is unusable; the message is the reason."""


class _Tree(BaseModel):
    """Node ``i`` is internal when ``feature[i] >= 0``: go to ``left[i]`` when
    ``x[feature[i]] <= threshold[i]`` else ``right[i]``. A leaf scores ``value[i]``."""

    feature: list[int]
    threshold: list[float]
    left: list[int]
    right: list[int]
    value: list[float]

    @model_validator(mode="after")
    def _is_a_finite_tree(self) -> Self:
        n = len(self.feature)
        if n == 0 or not all(
            len(column) == n
            for column in (self.threshold, self.left, self.right, self.value)
        ):
            raise ValueError("tree columns differ in length")
        seen: set[int] = set()
        pending = [0]
        while pending:
            node = pending.pop()
            if node in seen:
                raise ValueError("tree revisits a node")
            seen.add(node)
            feature = self.feature[node]
            if feature >= len(RANKING_FEATURES):
                raise ValueError("tree feature index out of range")
            if feature >= 0:
                for child in (self.left[node], self.right[node]):
                    if not 0 <= child < n:
                        raise ValueError("tree child index out of range")
                    pending.append(child)
        return self

    def score(self, x: list[float]) -> float:
        node = 0
        while (feature := self.feature[node]) >= 0:
            node = (
                self.left[node]
                if x[feature] <= self.threshold[node]
                else self.right[node]
            )
        return self.value[node]


class _Header(BaseModel):
    format: Any = None
    features: Any = None


class _Document(BaseModel):
    ctr_feature: str | None = None
    base_score: float = Field(default=0.0, allow_inf_nan=False)
    learning_rate: float = Field(allow_inf_nan=False)
    trees: list[_Tree] = Field(min_length=1)


class TrainedRanker:
    """A parsed ``agora-gbdt/1`` model: ``score(x)`` and candidate ranking."""

    def __init__(self, document: _Document) -> None:
        self._trees = document.trees
        self._base = document.base_score
        self._lr = document.learning_rate
        self._ctr_index = (
            RANKING_FEATURES.index(document.ctr_feature)
            if document.ctr_feature in RANKING_FEATURES
            else -1
        )
        self._columns = [tuple(name.split(".", 1)) for name in RANKING_FEATURES]

    @classmethod
    def parse(cls, raw: str) -> Self:
        """Parse and validate; raises ``ArtifactError`` with the reason."""
        try:
            data = json.loads(raw)
            header = _Header.model_validate(data)
        except (ValueError, ValidationError) as exc:
            raise ArtifactError("malformed") from exc
        if header.format != GBDT_ARTIFACT_FORMAT:
            raise ArtifactError("format_mismatch")
        if header.features != list(RANKING_FEATURES):
            raise ArtifactError("feature_mismatch")
        try:
            return cls(_Document.model_validate(data))
        except ValidationError as exc:
            raise ArtifactError("malformed") from exc

    def score(self, x: list[float]) -> float:
        return self._base + self._lr * sum(tree.score(x) for tree in self._trees)

    def vector(
        self,
        popularity_row: dict[str, Any],
        attribute_row: dict[str, Any],
        nearline_ctr: float = 0.0,
    ) -> list[float]:
        """``x`` for one candidate: 0.0 where the row, key or value is missing, null or not
        finite; a usable (> 0) nearline debiased CTR replaces ``ctr_7d``."""
        rows = {"item_popularity": popularity_row, "item_attributes": attribute_row}
        x = [number(rows[view].get(name)) or 0.0 for view, name in self._columns]
        if self._ctr_index >= 0 and nearline_ctr > 0:
            x[self._ctr_index] = min(1.0, nearline_ctr)
        return x

    def rank_candidates(
        self,
        candidates: Iterable[Candidate],
        seed_listing_id: str,
        popularity_rows: Mapping[str, dict[str, Any]],
        attribute_rows: Mapping[str, dict[str, Any]],
        nearline: Mapping[str, float],
        limit: int,
    ) -> list[RecommendedItem]:
        """Descending model score; retrieval similarity breaks ties."""
        scored = [
            (
                cand,
                self.score(
                    self.vector(
                        popularity_rows.get(cand.listing_id, {}),
                        attribute_rows.get(cand.listing_id, {}),
                        nearline.get(cand.listing_id, 0.0),
                    )
                ),
            )
            for cand in eligible(candidates, seed_listing_id)
        ]
        scored.sort(key=lambda pair: (-pair[1], -pair[0].score))
        if limit > 0:
            scored = scored[:limit]
        return [
            RecommendedItem(listing_id=cand.listing_id, score=score, rank=rank)
            for rank, (cand, score) in enumerate(scored, start=1)
        ]


class TrainedRankerLoader:
    """Per-generation cache of the parsed model; reloads when the serving generation moves.

    The key is re-read after ``recheck_s`` whatever the last verdict was, so a late, replaced
    or deleted artifact is picked up within that delay; an unchanged payload is not re-parsed.
    """

    def __init__(
        self,
        cache: ServingCache,
        *,
        clock: Callable[[], float] = time.monotonic,
        recheck_s: float = 5.0,
    ) -> None:
        self._cache = cache
        self._clock = clock
        self._recheck_s = recheck_s
        self._loaded_at = 0.0
        self._generation: str | None = None
        self._raw: str | None = None
        self._ranker: TrainedRanker | None = None

    async def get(self, generation: str | None) -> TrainedRanker | None:
        """The model of ``generation``; ``None`` when there is no generation, no artifact or
        the artifact is rejected."""
        if not generation:
            return None
        if (
            generation == self._generation
            and self._clock() - self._loaded_at < self._recheck_s
        ):
            return self._ranker
        raw = await self._cache.ranker_artifact(generation)
        if generation == self._generation and raw == self._raw:
            self._loaded_at = self._clock()
            return self._ranker
        ranker: TrainedRanker | None = None
        if raw:
            try:
                ranker = TrainedRanker.parse(raw)
            except ArtifactError as exc:
                logger.warning("recs.ranker.rejected gen={} reason={}", generation, exc)
        self._generation, self._raw, self._ranker = generation, raw, ranker
        self._loaded_at = self._clock()
        return ranker
