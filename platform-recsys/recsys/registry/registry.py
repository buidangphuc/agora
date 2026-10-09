"""Model registry implementation managing model lifecycle and automated promotion gates."""

from __future__ import annotations

import logging
from dataclasses import dataclass
from typing import Any

from recsys.evals.evaluator import ModelEvaluator
from recsys.registry.metadata import ModelMetadata

logger = logging.getLogger(__name__)

CHAMPION_KEY = "recs:model:champion"
METADATA_KEY_PREFIX = "recs:model:meta:"


@dataclass
class PromotionDecision:
    """What the metric gate decided; applied to the registry only by ``apply_promotion``/``reject``."""

    promoted: bool
    reason: str
    candidate: ModelMetadata | None
    demote: ModelMetadata | None = None


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
        decision = self.evaluate(
            candidate_version,
            primary_metric=primary_metric,
            min_relative_improvement=min_relative_improvement,
            min_coverage_ratio=min_coverage_ratio,
            force=force,
        )
        if decision.promoted:
            self.apply_promotion(decision)
        else:
            self.reject(decision)
        return decision.promoted, decision.reason

    def evaluate(
        self,
        candidate_version: str,
        *,
        primary_metric: str = "ndcg@10",
        min_relative_improvement: float = 0.0,
        min_coverage_ratio: float = 0.8,
        force: bool = False,
    ) -> PromotionDecision:
        """The metric gate's verdict. Changes nothing: ``apply_promotion`` / ``reject`` do."""
        candidate = self.get_model(candidate_version)
        if candidate is None:
            return PromotionDecision(
                False, f"Candidate model {candidate_version} not found in registry", None
            )

        champion_ver = self.get_champion_version()
        if force:
            return PromotionDecision(
                True, f"Model {candidate_version} promoted by force (gate skipped)", candidate
            )
        if champion_ver is None:
            # First model becomes champion automatically
            return PromotionDecision(
                True,
                f"Model {candidate_version} set as initial champion (no prior champion)",
                candidate,
            )

        champion = self.get_model(champion_ver)
        if champion is None:
            return PromotionDecision(
                True,
                f"Model {candidate_version} promoted (prior champion metadata missing)",
                candidate,
            )

        champ_protocol = champion.metrics.get("eval_protocol")
        cand_protocol = candidate.metrics.get("eval_protocol")
        if champ_protocol != cand_protocol:
            # Numbers from different evaluation protocols are not comparable (e.g. the
            # pre-v1 protocol trained on its own holdout); the candidate, measured
            # under the current protocol, replaces the champion.
            return PromotionDecision(
                True,
                f"Model {candidate_version} promoted: champion {champion_ver} was evaluated "
                f"under protocol {champ_protocol or 'unversioned'}, not {cand_protocol}, "
                "so the metrics are not comparable",
                candidate,
                champion,
            )

        passed, reason = ModelEvaluator.compare_models(
            baseline_metrics=champion.metrics,
            candidate_metrics=candidate.metrics,
            primary_metric=primary_metric,
            min_relative_improvement=min_relative_improvement,
            min_coverage_ratio=min_coverage_ratio,
        )
        if passed:
            return PromotionDecision(True, reason, candidate, champion)
        return PromotionDecision(False, reason, candidate)

    def apply_promotion(self, decision: PromotionDecision) -> None:
        """Demote the replaced champion (if any) and make the candidate the champion."""
        if decision.candidate is None:
            return
        if decision.demote is not None:
            decision.demote.status = "archived"
            self.register_model(decision.demote)
        self._set_champion(decision.candidate)
        logger.info("Promoted candidate %s to champion: %s", decision.candidate.model_version, decision.reason)

    def reject(self, decision: PromotionDecision, reason: str | None = None) -> None:
        """Record the candidate as ``rejected``; the champion is not touched."""
        if decision.candidate is None:
            return
        candidate = decision.candidate
        candidate.status = "rejected"
        if reason is not None:
            # Same two places as the structural gate: metrics (string next to eval_protocol) and
            # parameters, for readers that treat metrics as floats.
            candidate.metrics["gate_reason"] = reason
            candidate.parameters["gate_reason"] = reason
        self.register_model(candidate)
        logger.warning("Rejected candidate %s: %s", candidate.model_version, reason or decision.reason)

    def restore_champion(self, model_version: str) -> str | None:
        """Make ``model_version`` the champion again (rollback). The champion it replaces becomes
        ``archived``. Returns the demoted version, if any."""
        demoted = self.get_champion_version()
        if demoted and demoted != model_version:
            old = self.get_model(demoted)
            if old is not None:
                old.status = "archived"
                self.register_model(old)
        target = self.get_model(model_version)
        if target is not None:
            self._set_champion(target)
        elif self.redis is not None:
            self.redis.set(CHAMPION_KEY, model_version)
        else:
            self._champion_version = model_version
        return demoted if demoted != model_version else None

    def _set_champion(self, model: ModelMetadata) -> None:
        model.status = "champion"
        self.register_model(model)
        if self.redis is not None:
            self.redis.set(CHAMPION_KEY, model.model_version)
        else:
            self._champion_version = model.model_version
