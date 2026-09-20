"""Model registry implementation managing model lifecycle and automated promotion gates."""

from __future__ import annotations

import logging
from typing import Any

from recsys.evals.evaluator import ModelEvaluator
from recsys.registry.metadata import ModelMetadata

logger = logging.getLogger(__name__)

CHAMPION_KEY = "recs:model:champion"
METADATA_KEY_PREFIX = "recs:model:meta:"


class ModelRegistry:
    """Manages registered models, metadata persistence, and promotion gates."""

    def __init__(self, redis_client: Any | None = None) -> None:
        self.redis = redis_client
        self._in_memory_store: dict[str, str] = {}
        self._champion_version: str | None = None

    def register_model(self, metadata: ModelMetadata) -> None:
        """Register a new model candidate in the registry."""
        key = f"{METADATA_KEY_PREFIX}{metadata.model_version}"
        json_data = metadata.to_json()
        if self.redis is not None:
            self.redis.set(key, json_data)
        else:
            self._in_memory_store[key] = json_data
        logger.info("Registered model %s (%s)", metadata.model_name, metadata.model_version)

    def get_model(self, model_version: str) -> ModelMetadata | None:
        """Retrieve model metadata by version."""
        key = f"{METADATA_KEY_PREFIX}{model_version}"
        raw: str | None = None
        if self.redis is not None:
            val = self.redis.get(key)
            if val is not None:
                raw = val.decode("utf-8") if isinstance(val, bytes) else str(val)
        else:
            raw = self._in_memory_store.get(key)

        if raw is None:
            return None
        return ModelMetadata.from_json(raw)

    def get_champion_version(self) -> str | None:
        """Get the current active champion version."""
        if self.redis is not None:
            val = self.redis.get(CHAMPION_KEY)
            if val is not None:
                return val.decode("utf-8") if isinstance(val, bytes) else str(val)
            return None
        return self._champion_version

    def evaluate_and_promote(
        self,
        candidate_version: str,
        *,
        primary_metric: str = "ndcg@10",
        min_relative_improvement: float = 0.0,
        min_coverage_ratio: float = 0.8,
        force: bool = False,
    ) -> tuple[bool, str]:
        """Run promotion gate comparing candidate to champion and promote if eligible."""
        candidate = self.get_model(candidate_version)
        if candidate is None:
            return False, f"Candidate model {candidate_version} not found in registry"

        champion_ver = self.get_champion_version()
        if champion_ver is None or force:
            # First model becomes champion automatically
            self._set_champion(candidate)
            return True, f"Model {candidate_version} set as initial champion (no prior champion)"

        champion = self.get_model(champion_ver)
        if champion is None:
            self._set_champion(candidate)
            return True, f"Model {candidate_version} promoted (prior champion metadata missing)"

        passed, reason = ModelEvaluator.compare_models(
            baseline_metrics=champion.metrics,
            candidate_metrics=candidate.metrics,
            primary_metric=primary_metric,
            min_relative_improvement=min_relative_improvement,
            min_coverage_ratio=min_coverage_ratio,
        )

        if passed:
            # Demote current champion
            champion.status = "archived"
            self.register_model(champion)

            # Promote candidate
            self._set_champion(candidate)
            logger.info("Promoted candidate %s to champion: %s", candidate_version, reason)
            return True, reason

        # Reject candidate
        candidate.status = "rejected"
        self.register_model(candidate)
        logger.warning("Rejected candidate %s: %s", candidate_version, reason)
        return False, reason

    def _set_champion(self, model: ModelMetadata) -> None:
        model.status = "champion"
        self.register_model(model)
        if self.redis is not None:
            self.redis.set(CHAMPION_KEY, model.model_version)
        else:
            self._champion_version = model.model_version
