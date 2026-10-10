"""The trained GBDT ranker published by platform-recsys (artifact ``agora-gbdt/1``).

``recs:v1:gen:<generation>:ranker`` holds the LambdaMART as JSON (change
``recsys-gbdt-trainer``, design D7). The loader reads the key of the request's serving
generation, accepts it only when its ``features`` equal ``RANKING_FEATURES``, parses it once
per generation and scores in pure Python. Anything else (absent key, malformed JSON, other
format, other feature list, Redis error) leaves the built-in fixed weights in charge and is
counted; nothing here raises into a request.
"""

from __future__ import annotations

import json
import math
import time
from collections.abc import Callable, Iterable
from dataclasses import dataclass
from typing import TYPE_CHECKING, Any

from loguru import logger

from app.modules.business.recommend.features import RANKING_FEATURES
from app.modules.business.recommend.schemas import (
    Candidate,
    RecommendedItem,
    RecommendQuery,
)

if TYPE_CHECKING:
    from app.modules.business.recommend.cache import PrecomputedCache
    from app.modules.business.recommend.ranking import NearlineSignalPort

ARTIFACT_FORMAT = "agora-gbdt/1"
CTR_FEATURE = "item_popularity.ctr_7d"


class ArtifactError(ValueError):
    """The artifact is unusable; the message is the fallback reason."""


@dataclass(frozen=True)
class _Tree:
    feature: tuple[int, ...]
    threshold: tuple[float, ...]
    left: tuple[int, ...]
    right: tuple[int, ...]
    value: tuple[float, ...]

    def score(self, x: list[float]) -> float:
        node = 0
        for _ in range(len(self.feature)):  # bounded: a cycle cannot hang a request
            f = self.feature[node]
            if f < 0:
                return self.value[node]
            node = self.left[node] if x[f] <= self.threshold[node] else self.right[node]
        raise ArtifactError("tree does not reach a leaf")


def _ints(raw: Any, n: int, name: str) -> tuple[int, ...]:
    if not isinstance(raw, list) or len(raw) != n:
        raise ArtifactError(f"tree.{name} malformed")
    return tuple(int(v) for v in raw)


def _floats(raw: Any, n: int, name: str) -> tuple[float, ...]:
    if not isinstance(raw, list) or len(raw) != n:
        raise ArtifactError(f"tree.{name} malformed")
    return tuple(float(v) for v in raw)


def _parse_tree(raw: Any, n_features: int) -> _Tree:
    if not isinstance(raw, dict) or not isinstance(raw.get("feature"), list):
        raise ArtifactError("tree malformed")
    n = len(raw["feature"])
    if n == 0:
        raise ArtifactError("empty tree")
    tree = _Tree(
        feature=_ints(raw["feature"], n, "feature"),
        threshold=_floats(raw.get("threshold"), n, "threshold"),
        left=_ints(raw.get("left"), n, "left"),
        right=_ints(raw.get("right"), n, "right"),
        value=_floats(raw.get("value"), n, "value"),
    )
    for i, f in enumerate(tree.feature):
        if f >= n_features:
            raise ArtifactError("tree feature index out of range")
        if f >= 0 and not (0 <= tree.left[i] < n and 0 <= tree.right[i] < n):
            raise ArtifactError("tree child index out of range")
    return tree


class TrainedRanker:
    """A parsed ``agora-gbdt/1`` model: ``score(x)`` and candidate ranking."""

    def __init__(
        self,
        *,
        features: tuple[str, ...],
        trees: list[_Tree],
        base_score: float,
        learning_rate: float,
        model_version: str,
        ctr_feature: str | None,
    ) -> None:
        self.features = features
        self._trees = trees
        self._base = base_score
        self._lr = learning_rate
        self.model_version = model_version
        self._ctr_index = features.index(ctr_feature) if ctr_feature in features else -1
        self._columns = [tuple(name.split(".", 1)) for name in features]

    @classmethod
    def parse(cls, raw: str | bytes) -> TrainedRanker:
        """Parse and validate; raises ``ArtifactError`` with the fallback reason."""
        try:
            data = json.loads(raw)
        except (ValueError, TypeError) as exc:
            raise ArtifactError("malformed") from exc
        if not isinstance(data, dict):
            raise ArtifactError("malformed")
        if data.get("format") != ARTIFACT_FORMAT:
            raise ArtifactError("format_mismatch")
        features = data.get("features")
        if not isinstance(features, list) or tuple(features) != RANKING_FEATURES:
            raise ArtifactError("feature_mismatch")
        try:
            trees = [_parse_tree(t, len(features)) for t in data["trees"]]
            base = float(data.get("base_score", 0.0))
            lr = float(data["learning_rate"])
        except ArtifactError:
            raise
        except (KeyError, TypeError, ValueError) as exc:
            raise ArtifactError("malformed") from exc
        if not trees or not all(map(math.isfinite, (base, lr))):
            raise ArtifactError("malformed")
        ctr = data.get("ctr_feature")
        return cls(
            features=tuple(features),
            trees=trees,
            base_score=base,
            learning_rate=lr,
            model_version=str(data.get("model_version") or ""),
            ctr_feature=ctr if isinstance(ctr, str) else None,
        )

    def score(self, x: list[float]) -> float:
        return self._base + self._lr * sum(t.score(x) for t in self._trees)

    def vector(
        self,
        popularity_row: dict[str, Any],
        attribute_row: dict[str, Any],
        nearline_ctr: float = 0.0,
    ) -> tuple[list[float], str]:
        """``x`` for one candidate and its CTR source (``nearline`` | ``fallback``).

        A value is 0.0 when the row, key or value is missing, null or not finite; the
        nearline debiased CTR replaces ``ctr_7d`` when usable (> 0).
        """
        rows = {"item_popularity": popularity_row, "item_attributes": attribute_row}
        x: list[float] = []
        for view, name in self._columns:
            try:
                v = float(rows[view].get(name))  # type: ignore[arg-type]
            except (TypeError, ValueError):
                v = 0.0
            x.append(v if math.isfinite(v) else 0.0)
        source = "fallback"
        if self._ctr_index >= 0 and nearline_ctr > 0:
            x[self._ctr_index] = min(1.0, nearline_ctr)
            source = "nearline"
        return x, source

    def rank_candidates(
        self,
        candidates: Iterable[Candidate],
        query: RecommendQuery,
        popularity_rows: dict[str, dict[str, Any]],
        attribute_rows: dict[str, dict[str, Any]],
        nearline_store: NearlineSignalPort | None = None,
        limit: int = 0,
    ) -> list[RecommendedItem]:
        """Descending model score; retrieval similarity breaks ties."""
        best: dict[str, Candidate] = {}
        for cand in candidates:
            if (
                not cand.listing_id
                or cand.listing_id == query.seed_listing_id
                or not cand.in_stock
            ):
                continue
            prior = best.get(cand.listing_id)
            if prior is None or cand.score > prior.score:
                best[cand.listing_id] = cand
        scored: list[tuple[Candidate, float, str]] = []
        for cand in best.values():
            nl = (
                nearline_store.get_debiased_ctr(cand.listing_id)
                if nearline_store
                else 0.0
            )
            x, source = self.vector(
                popularity_rows.get(cand.listing_id, {}),
                attribute_rows.get(cand.listing_id, {}),
                nl,
            )
            scored.append((cand, self.score(x), source))
        scored.sort(key=lambda t: (-t[1], -t[0].score))
        if limit > 0:
            scored = scored[:limit]
        return [
            RecommendedItem(listing_id=c.listing_id, score=s, rank=r, ctr_source=src)
            for r, (c, s, src) in enumerate(scored, start=1)
        ]


class TrainedRankerLoader:
    """Per-generation cache of the parsed model; reloads when the serving generation moves.

    ``get`` returns ``(ranker | None, reason)``; ``reason`` is ``None`` when a model is
    returned, else why the fixed weights are used: ``no_generation``, ``absent``,
    ``malformed``, ``format_mismatch``, ``feature_mismatch``, ``read_error``. Every
    non-``None`` reason except ``no_generation`` and ``absent`` is a defect and counted in
    ``fallbacks``. The key is re-read after ``recheck_s`` whatever the last verdict was, so a late,
    replaced or deleted artifact is picked up within that delay; an unchanged payload is not
    re-parsed. A Redis error is not remembered at all.
    """

    def __init__(
        self,
        redis: Any,
        cache: PrecomputedCache,
        *,
        clock: Callable[[], float] = time.monotonic,
        recheck_s: float = 5.0,
        absent_recheck_s: float | None = None,
    ) -> None:
        self._redis = redis
        self._cache = cache
        self._clock = clock
        # ``absent_recheck_s`` is the older name of ``recheck_s``.
        self._recheck_s = (
            absent_recheck_s if absent_recheck_s is not None else recheck_s
        )
        self._loaded_at = 0.0
        self._raw: str | bytes | None = None
        self._generation: str | None = None
        self._ranker: TrainedRanker | None = None
        self._reason: str | None = "no_generation"
        self.fallbacks = 0

    async def get(self) -> tuple[TrainedRanker | None, str | None]:
        try:
            gen = await self._cache.serving_generation()
        except Exception as exc:
            logger.warning("recs.ranker.generation_failed err={}", exc)
            return self._fail("read_error")
        if not gen or self._redis is None:
            return None, "no_generation"
        if (
            gen == self._generation
            and self._clock() - self._loaded_at < self._recheck_s
        ):
            return self._ranker, self._reason
        try:
            raw = await self._redis.get(self._cache.ranker_key(gen))
        except Exception as exc:
            logger.warning("recs.ranker.read_failed gen={} err={}", gen, exc)
            return self._fail("read_error")
        if gen == self._generation and raw == self._raw:
            self._loaded_at = self._clock()
            return self._ranker, self._reason
        ranker: TrainedRanker | None = None
        reason: str | None = None
        if not raw:
            reason = "absent"
        else:
            try:
                ranker = TrainedRanker.parse(raw)
            except ArtifactError as exc:
                reason = str(exc)
                logger.warning("recs.ranker.rejected gen={} reason={}", gen, reason)
        if reason not in (None, "absent"):
            self.fallbacks += 1
        self._generation, self._ranker, self._reason, self._raw = (
            gen,
            ranker,
            reason,
            raw,
        )
        self._loaded_at = self._clock()
        return ranker, reason

    def _fail(self, reason: str) -> tuple[None, str]:
        self.fallbacks += 1
        return None, reason
