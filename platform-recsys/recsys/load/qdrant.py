"""Upsert ALS factors into Qdrant (:6333).

Two collections, both vector dim = rank, distance = Cosine (vectors are
L2-normalized on upsert so cosine order ≈ ALS dot-product order):

- item collection (default ``item_als_vectors`` — the name the consumer,
  serve-recommendations-teamai, reads via RECS_QDRANT_COLLECTION): one point per
  listing; payload ``{listing_id, model_version, updated_at}``.
- user collection (default ``user_als_vectors``): one point per user/anonymous
  key; payload ``{user_key, model_version, updated_at}``.

Each run writes under a fresh ``model_version`` and then prunes points whose
``model_version`` is stale, so the online reader always sees one consistent
generation. Point ids are a deterministic UUID5 of the source id (Qdrant point
ids must be uint or UUID, source ids are arbitrary strings).

Payload holds ids + provenance only — no hydrated listing content (Rule 3); the
serving layer hydrates cards.
"""

from __future__ import annotations

import uuid
from datetime import datetime, timezone

# Stable namespace so a given listing/user maps to the same point id every run.
_NS = uuid.UUID("6f7a1e2c-9b3d-4c5a-8e21-0d9f4a2b1c00")


def point_id(source_id: str) -> str:
    return str(uuid.uuid5(_NS, source_id))


def _ensure_collection(client, name: str, dim: int) -> None:
    from qdrant_client import models  # noqa: PLC0415

    existing = {c.name for c in client.get_collections().collections}
    if name in existing:
        return
    client.create_collection(
        collection_name=name,
        vectors_config=models.VectorParams(size=dim, distance=models.Distance.COSINE),
    )


def _upsert(client, name: str, rows, id_field: str, model_version: str, updated_at: str) -> int:
    """rows: iterable of (source_id, normalized_vector). Returns count upserted."""
    from qdrant_client import models  # noqa: PLC0415

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
    from qdrant_client import models  # noqa: PLC0415

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
    """Upsert item + user factors and prune stale generations.

    item_rows / user_rows: iterables of (source_id, L2-normalized vector).
    Returns {"items": n_items, "users": n_users}.
    """
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
