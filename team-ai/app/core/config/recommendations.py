"""Recommendation serving settings.

Mirrors the ``RAG_*`` block in ``ai.py``: a single ``RECS_ENABLED`` gate, a
``memory | qdrant`` backend seam (team-ai runs no model in-process — it only
reads the Qdrant collection + Redis pre-computed lists that the offline ALS
training job populates), plus the two-stage Top-K knobs, the Redis cache
key/TTL, and the retrieval latency cap.
"""

from __future__ import annotations

from pydantic import BaseModel, Field


class RecommendationSettingsMixin(BaseModel):
    # Gate — identical semantics to RAG_ENABLED. When false the servicer aborts
    # UNAVAILABLE and no Qdrant/Redis access is attempted.
    RECS_ENABLED: bool = False

    # Retrieval backend seam (mirrors RAG_BACKEND): "memory" is an in-process
    # deterministic fixture for offline dev / e2e; "qdrant" is the real path that
    # reads the training job's collection.
    RECS_BACKEND: str = "memory"  # "memory" | "qdrant"

    # Qdrant collection contract shared with build-als-training-job (team-ai
    # only reads it). Name/dim/metric must match the training job exactly; a
    # mismatch is a loud startup contract break, not a silent empty result.
    RECS_QDRANT_URL: str = "http://localhost:6333"
    RECS_QDRANT_COLLECTION: str = "item_als_vectors"
    RECS_VECTOR_DIM: int = Field(default=64, gt=0)
    RECS_QDRANT_DISTANCE: str = "Cosine"

    # Two-stage Top-K: candidate retrieval (recall) vs result (precision).
    RECS_CANDIDATE_TOP_K: int = Field(default=100, gt=0)
    RECS_RESULT_TOP_K: int = Field(default=10, gt=0)

    # Redis pre-computed cache fast path. Key layout:
    # ``{RECS_CACHE_PREFIX}:{RECS_CACHE_SCHEMA_VERSION}:user:{user_id}`` and the
    # popularity fallback ``{prefix}:{schema}:popular``.
    RECS_CACHE_PREFIX: str = "recs"
    RECS_CACHE_SCHEMA_VERSION: str = "v1"
    RECS_CACHE_TTL_SECONDS: int = Field(default=86400, ge=0)

    # Latency cap for the Qdrant fallback path; on timeout the module serves the
    # popularity fallback rather than exceeding budget.
    RECS_RETRIEVE_TIMEOUT_MS: int = Field(default=15, gt=0)

    # Reported on every RecommendResponse; a serving-side label until the
    # training job stamps its own version into the artifacts it writes.
    RECS_MODEL_VERSION: str = "serving-fallback"
