from dataclasses import dataclass

from pydantic import BaseModel

# The model version of a result served from the popular-list fallback, and the
# configured value ("RECS_MODEL_VERSION") that defers to the serving generation.
SERVING_FALLBACK = "serving-fallback"


@dataclass(frozen=True, slots=True)
class Candidate:
    """A retrieval candidate: a listing id and a score, as the producers write them."""

    listing_id: str
    score: float
    in_stock: bool = True


class RecommendationQuery(BaseModel):
    user_id: str = ""
    anonymous_id: str = ""
    seed_listing_id: str = ""
    placement_id: str = ""
    limit: int = 0


class RecommendedItem(BaseModel):
    listing_id: str
    score: float
    rank: int


class RecommendationResult(BaseModel):
    items: list[RecommendedItem]
    model_version: str
    placement_id: str
    # True when the ladder found nothing or a dependency failed and the result is the
    # popular-list floor.
    fallback: bool = False
