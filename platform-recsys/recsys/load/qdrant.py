"""Upsert ALS and Two-Tower factors into Qdrant (:6333).

Collections:
- item collection (default ``item_als_vectors``): ALS item factors
- user collection (default ``user_als_vectors``): ALS user factors
- two-tower collection (default ``item_two_tower_vectors``): Dense neural candidate vectors

Each run writes under a fresh ``model_version`` and prunes stale generations.
"""

from __future__ import annotations

from dataclasses import dataclass, field
from datetime import datetime, timezone
from typing import Any
import uuid

# Stable namespace so a given listing/user maps to the same point id every run.
_NS = uuid.UUID("6f7a1e2c-9b3d-4c5a-8e21-0d9f4a2b1c00")


@dataclass
class PointStruct:
    id: str
    vector: list[float]
    payload: dict[str, Any] = field(default_factory=dict)


def point_id(source_id: str) -> str:
    return str(uuid.uuid5(_NS, source_id))


def _get_models():
    try:
        from qdrant_client import models
        return models
    except ImportError:
        class FakeModels:
            PointStruct = PointStruct
            class Distance:
                COSINE = "Cosine"
            @staticmethod
            def VectorParams(size: int, distance: str):
                return {"size": size, "distance": distance}
            @staticmethod
            def FilterSelector(filter: Any):
                return {"filter": filter}
            @staticmethod
            def Filter(must_not: list):
                return {"must_not": must_not}
            @staticmethod
            def FieldCondition(key: str, match: Any):
                return {"key": key, "match": match}
            @staticmethod
            def MatchValue(value: Any):
                return {"value": value}
        return FakeModels


def _ensure_collection(client, name: str, dim: int) -> None:
    models = _get_models()
    existing = {c.name for c in client.get_collections().collections}
    if name in existing:
        return
    client.create_collection(
        collection_name=name,
        vectors_config=models.VectorParams(size=dim, distance=models.Distance.COSINE),
    )


def _upsert(client, name: str, rows, id_field: str, model_version: str, updated_at: str) -> int:
    """rows: iterable of (source_id, normalized_vector). Returns count upserted."""
    models = _get_models()
    points = []
    count = 0
    for source_id, vector in rows:
        points.append(
            models.PointStruct(
                id=point_id(source_id),
                vector=list(vector),
                payload={id_field: source_id, "model_version": model_version, "updated_at": updated_at},
            )
        )
        count += 1
        if len(points) >= 256:
            client.upsert(collection_name=name, points=points)
            points = []
    if points:
        client.upsert(collection_name=name, points=points)
    return count


def _prune_stale(client, name: str, model_version: str) -> None:
    """Delete points not stamped with the current model_version."""
    models = _get_models()
    client.delete(
        collection_name=name,
        points_selector=models.FilterSelector(
            filter=models.Filter(
                must_not=[
                    models.FieldCondition(key="model_version", match=models.MatchValue(value=model_version))
                ]
            )
        ),
    )


def load_vectors(
    settings,
    model_version: str,
    item_rows,
    user_rows,
    client=None,
) -> dict[str, int]:
    """Upsert item + user factors and prune stale generations."""
    if client is None:
        from qdrant_client import QdrantClient  # noqa: PLC0415
        client = QdrantClient(url=settings.qdrant_url)

    updated_at = datetime.now(timezone.utc).isoformat()
    dim = settings.als_rank

    _ensure_collection(client, settings.qdrant_item_collection, dim)
    _ensure_collection(client, settings.qdrant_user_collection, dim)

    n_items = _upsert(
        client, settings.qdrant_item_collection, item_rows, "listing_id", model_version, updated_at
    )
    n_users = _upsert(
        client, settings.qdrant_user_collection, user_rows, "user_key", model_version, updated_at
    )

    _prune_stale(client, settings.qdrant_item_collection, model_version)
    _prune_stale(client, settings.qdrant_user_collection, model_version)

    return {"items": n_items, "users": n_users}


def load_two_tower_vectors(
    settings,
    model_version: str,
    item_vectors: dict[str, list[float]],
    client=None,
) -> int:
    """Upsert Two-Tower candidate item vectors and prune stale generations."""
    if client is None:
        from qdrant_client import QdrantClient  # noqa: PLC0415
        client = QdrantClient(url=settings.qdrant_url)

    updated_at = datetime.now(timezone.utc).isoformat()
    dim = settings.two_tower_dim
    coll_name = settings.qdrant_two_tower_collection

    _ensure_collection(client, coll_name, dim)

    n_items = _upsert(
        client,
        coll_name,
        item_vectors.items(),
        "listing_id",
        model_version,
        updated_at,
    )

    _prune_stale(client, coll_name, model_version)
    return n_items
