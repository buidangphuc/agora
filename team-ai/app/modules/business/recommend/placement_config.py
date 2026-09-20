"""Placement configuration loader and registry (ADR-0012)."""

from __future__ import annotations

from dataclasses import dataclass, field
from pathlib import Path

import yaml

KNOWN_RANKING_MODELS = {"cosine_rank", "gbdt"}
KNOWN_STRATEGIES = {
    "user_precomputed",
    "seed_vector_similarity",
    "cart_cross_similarity",
    "category_popular",
    "global_popular",
}


@dataclass
class LadderStep:
    tier: str
    strategy: str
    min_candidates: int = 1


@dataclass
class PlacementConfig:
    placement_id: str
    name: str
    enabled: bool = True
    result_limit: int = 10
    timeout_ms: int = 50
    allow_cache: bool = True
    candidate_ladder: list[LadderStep] = field(default_factory=list)
    ranking_model: str = "cosine_rank"
    use_featurestore: bool = False
    in_stock_only: bool = True
    exclude_seed: bool = True
    exclude_cart_items: bool = True


class PlacementRegistry:
    """Registry loading placement configurations from YAML with startup validation."""

    def __init__(self, config_path: str | Path | None = None) -> None:
        self._placements: dict[str, PlacementConfig] = {}
        path = Path(config_path) if config_path else Path(__file__).parent / "config" / "placements.yaml"
        if path.exists():
            self._load_from_yaml(path)
        else:
            self._load_defaults()
        self.validate()

    def _load_from_yaml(self, path: Path) -> None:
        with open(path, encoding="utf-8") as f:
            data = yaml.safe_load(f) or {}

        placements_data = data.get("placements", {})
        for pid, cfg in placements_data.items():
            ladder = [
                LadderStep(
                    tier=step.get("tier", ""),
                    strategy=step.get("strategy", ""),
                    min_candidates=step.get("min_candidates", 1),
                )
                for step in cfg.get("candidate_ladder", [])
            ]
            ranking = cfg.get("ranking", {})
            filters = cfg.get("filters", {})
            self._placements[pid] = PlacementConfig(
                placement_id=pid,
                name=cfg.get("name", pid),
                enabled=cfg.get("enabled", True),
                result_limit=cfg.get("result_limit", 10),
                timeout_ms=cfg.get("timeout_ms", 50),
                allow_cache=cfg.get("allow_cache", True),
                candidate_ladder=ladder,
                ranking_model=ranking.get("model", "cosine_rank"),
                use_featurestore=ranking.get("use_featurestore", False),
                in_stock_only=filters.get("in_stock_only", True),
                exclude_seed=filters.get("exclude_seed", True),
                exclude_cart_items=filters.get("exclude_cart_items", True),
            )

    def _load_defaults(self) -> None:
        self._placements["home_feed"] = PlacementConfig(
            placement_id="home_feed",
            name="Home Feed Default",
            candidate_ladder=[
                LadderStep(tier="tier1_personalized", strategy="user_precomputed", min_candidates=5),
                LadderStep(tier="tier4_global_popular", strategy="global_popular", min_candidates=1),
            ],
        )

    def validate(self) -> None:
        """Startup validation: rejects placements naming unbound ranking models or strategies."""
        for pid, cfg in self._placements.items():
            if cfg.ranking_model not in KNOWN_RANKING_MODELS:
                raise ValueError(
                    f"Placement '{pid}' declares unbound ranking model '{cfg.ranking_model}'. "
                    f"Supported models: {sorted(KNOWN_RANKING_MODELS)}"
                )
            for step in cfg.candidate_ladder:
                if step.strategy and step.strategy not in KNOWN_STRATEGIES:
                    raise ValueError(
                        f"Placement '{pid}' declares unbound candidate strategy '{step.strategy}'. "
                        f"Supported strategies: {sorted(KNOWN_STRATEGIES)}"
                    )

    def get(self, placement_id: str) -> PlacementConfig:
        return self._placements.get(placement_id) or self._placements.get("home_feed") or PlacementConfig(
            placement_id=placement_id,
            name=placement_id,
        )

    def list_placements(self) -> list[str]:
        return list(self._placements.keys())
