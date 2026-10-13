"""Placement configuration (ADR-0012): the candidate ladder and ranking of each slot.

``placements.yaml`` is team-ai's file, copied verbatim. Only the fields the service enforces
are declared here; every other key in the file (``name``, ``enabled``, ``timeout_ms``,
``allow_cache``, ``filters``, ``tier``) is ignored, as it was in team-ai. A placement that
names a strategy or ranking model this module does not implement fails at load.
"""

from pathlib import Path
from typing import Literal, Self

import yaml
from pydantic import BaseModel, Field, model_validator

PLACEMENTS_FILE = Path(__file__).with_name("placements.yaml")
DEFAULT_PLACEMENT = "home_feed"

Strategy = Literal[
    "user_precomputed",
    "seed_vector_similarity",
    "cart_cross_similarity",
    "category_popular",
    "global_popular",
]
RankingModel = Literal["cosine_rank", "gbdt"]


class LadderStep(BaseModel):
    strategy: Strategy
    min_candidates: int = 1


class Ranking(BaseModel):
    model: RankingModel = "cosine_rank"
    use_featurestore: bool = False


class Placement(BaseModel):
    # 0 = fall back to RECS_RESULT_TOP_K.
    result_limit: int = Field(default=0, ge=0)
    candidate_ladder: list[LadderStep] = []
    ranking: Ranking = Ranking()


class Placements(BaseModel):
    by_id: dict[str, Placement] = Field(alias="placements")

    @model_validator(mode="after")
    def _has_the_default(self) -> Self:
        if DEFAULT_PLACEMENT not in self.by_id:
            raise ValueError(f"placements must define {DEFAULT_PLACEMENT!r}")
        return self

    def get(self, placement_id: str) -> Placement:
        """The placement, or ``home_feed`` for an id this file does not define."""
        return self.by_id.get(placement_id, self.by_id[DEFAULT_PLACEMENT])


def load_placements(path: Path = PLACEMENTS_FILE) -> Placements:
    return Placements.model_validate(yaml.safe_load(path.read_text(encoding="utf-8")))
