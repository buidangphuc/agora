"""Recommendation serving settings.

Names and defaults match team-ai. The service only reads what ``platform-recsys`` and
``platform-featurestore`` write (a Qdrant collection, Redis lists and rows), so a mismatch of
collection, dimension or distance is a startup contract break, not a silent empty result.
"""

from __future__ import annotations

from typing import Literal

from pydantic import BaseModel, Field

RecsDistance = Literal["Cosine", "Dot", "Euclid"]


class RecommendationSettingsMixin(BaseModel):
    # The candidate source reads the training job's Qdrant collection.
    RECS_QDRANT_URL: str = "http://localhost:6333"
    RECS_QDRANT_COLLECTION: str = "item_als_vectors"
    RECS_VECTOR_DIM: int = Field(default=64, gt=0)
    RECS_QDRANT_DISTANCE: RecsDistance = "Cosine"

    # Two-stage Top-K: candidate retrieval (recall) vs result (precision).
    RECS_CANDIDATE_TOP_K: int = Field(default=100, gt=0)
    RECS_RESULT_TOP_K: int = Field(default=10, gt=0)

    # Key layout ``{prefix}:{schema}:...`` of the lists the training job writes to the shared
    # Redis.
    RECS_CACHE_PREFIX: str = "recs"
    RECS_CACHE_SCHEMA_VERSION: str = "v1"

    # Budget of one Qdrant similarity query; on timeout the ladder moves on.
    RECS_RETRIEVE_TIMEOUT_MS: int = Field(default=15, gt=0)

    # Reported on every response; ``serving-fallback`` defers to the serving generation's
    # own model version.
    RECS_MODEL_VERSION: str = "serving-fallback"

    # platform-featurestore's Redis. Empty = off: ranking runs without online features.
    RECS_FEATURESTORE_REDIS_URL: str = ""

    # platform-recsys's nearline CTR Redis. Empty = off: ranking runs on its prior CTR.
    # Weighted impressions below the minimum count as no usable data.
    RECS_NEARLINE_REDIS_URL: str = ""
    RECS_NEARLINE_PREFIX: str = "recs:nearline"
    RECS_NEARLINE_MIN_IMPRESSIONS: float = Field(default=1.0, ge=0)
    RECS_NEARLINE_TIMEOUT_MS: int = Field(default=20, gt=0)
