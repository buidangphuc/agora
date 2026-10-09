"""Upsert ALS and Two-Tower factors into Qdrant (:6333).

Names (recsys-generations):
- ``item_als_vectors`` / ``user_als_vectors`` are ALIASES, a deprecated compatibility shim for
  readers that predate pointer-resolved collections (serving-switch-atomicity). team-ai now names
  ``<alias>__<serving generation>`` from ``recs:v1:serving`` itself, so the aliases decide nothing for
  it; they still point at the serving generation's collections and are removable in a later release.
- ``item_als_vectors__<gen>`` / ``user_als_vectors__<gen>``: the vectors of one ``model_version``,
  created fresh by each publish. The alias moves only after they are written, in one atomic call.
- two-tower collection (default ``item_two_tower_vectors``): not generation-scoped.

The first publish after this layout finds a real collection named ``item_als_vectors``: it is
copied to ``item_als_vectors__legacy``, deleted, and the alias takes its name (``__legacy`` is then
dropped by retention like any generation that is neither serving nor previous).
"""

from __future__ import annotations

import uuid
from dataclasses import dataclass, field
from datetime import datetime, timezone
from types import SimpleNamespace
from typing import Any

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
            @staticmethod
            def CreateAlias(collection_name: str, alias_name: str):
                return SimpleNamespace(collection_name=collection_name, alias_name=alias_name)
            @staticmethod
            def DeleteAlias(alias_name: str):
                return SimpleNamespace(alias_name=alias_name)
            @staticmethod
            def CreateAliasOperation(create_alias: Any):
                return SimpleNamespace(create_alias=create_alias)
            @staticmethod
            def DeleteAliasOperation(delete_alias: Any):
                return SimpleNamespace(delete_alias=delete_alias)
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
    """Delete points not stamped with the current model_version (two-tower collection only)."""
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


def generation_collection(base: str, generation: str) -> str:
    return f"{base}__{generation}"


def _bases(settings) -> tuple[str, str]:
    return settings.qdrant_item_collection, settings.qdrant_user_collection


def _connect(settings):
    from qdrant_client import QdrantClient  # noqa: PLC0415

    return QdrantClient(url=settings.qdrant_url)


def _real_collections(client) -> set[str]:
    return {c.name for c in client.get_collections().collections}


def _alias_targets(client) -> dict[str, str]:
    return {a.alias_name: a.collection_name for a in client.get_aliases().aliases}


def _fresh_collection(client, name: str, dim: int) -> None:
    """Create ``name`` empty; a leftover of an interrupted run of the same generation is dropped.

    A collection an alias points at is serving: it is reused (same version, same point ids)."""
    if name in _real_collections(client) and name not in _alias_targets(client).values():
        client.delete_collection(collection_name=name)
    _ensure_collection(client, name, dim)


def _alias_ops(client, targets: dict[str, str]) -> list:
    """Delete (if present) then create every ``alias -> collection`` in ``targets``."""
    models = _get_models()
    existing = _alias_targets(client)
    ops: list = []
    for alias, collection in targets.items():
        if alias in existing:
            ops.append(models.DeleteAliasOperation(delete_alias=models.DeleteAlias(alias_name=alias)))
        ops.append(
            models.CreateAliasOperation(
                create_alias=models.CreateAlias(collection_name=collection, alias_name=alias)
            )
        )
    return ops


def _move_alias(client, alias: str, collection: str) -> None:
    """Point ``alias`` at ``collection`` in one atomic call (delete + create)."""
    client.update_collection_aliases(change_aliases_operations=_alias_ops(client, {alias: collection}))


def _collection_dim(client, name: str, default: int) -> int:
    """Vector size of an existing collection (the old generation may use another ALS rank)."""
    try:
        vectors = client.get_collection(collection_name=name).config.params.vectors
        return int(getattr(vectors, "size", None) or vectors["size"])
    except Exception:  # noqa: BLE001 - unknown shape: fall back to the caller's dim
        return default


def _migrate_legacy(client, alias: str, dim: int) -> bool:
    """If ``alias`` is still a real collection, copy it to ``<alias>__legacy`` and let the alias
    take its name (pointing at the copy, so readers keep seeing the same data). True if migrated.

    The copy keeps the OLD collection's vector size, not the new model's: the legacy
    generation keeps serving until the new one is activated."""
    if alias not in _real_collections(client):
        return False
    legacy = generation_collection(alias, "legacy")
    _fresh_collection(client, legacy, _collection_dim(client, alias, dim))
    offset = None
    while True:
        points, offset = client.scroll(
            collection_name=alias, limit=256, offset=offset, with_vectors=True, with_payload=True
        )
        if points:
            models = _get_models()
            client.upsert(
                collection_name=legacy,
                points=[
                    models.PointStruct(id=p.id, vector=list(p.vector), payload=dict(p.payload or {}))
                    for p in points
                ],
            )
        if offset is None:
            break
    client.delete_collection(collection_name=alias)
    _move_alias(client, alias, legacy)
    return True


def load_vectors(
    settings,
    model_version: str,
    item_rows,
    user_rows,
    client=None,
) -> dict[str, int]:
    """Write the generation's item + user factors into its own fresh collections.

    Serving is untouched: the aliases move only in ``activate_aliases``. The first call on a
    deployment that still has plain ``item_als_vectors`` / ``user_als_vectors`` collections
    migrates them behind the aliases first.
    """
    if client is None:
        client = _connect(settings)

    updated_at = datetime.now(timezone.utc).isoformat()
    dim = settings.als_rank
    item_base, user_base = _bases(settings)

    migrated = [_migrate_legacy(client, base, dim) for base in (item_base, user_base)]
    item_coll = generation_collection(item_base, model_version)
    user_coll = generation_collection(user_base, model_version)
    _fresh_collection(client, item_coll, dim)
    _fresh_collection(client, user_coll, dim)

    n_items = _upsert(client, item_coll, item_rows, "listing_id", model_version, updated_at)
    n_users = _upsert(client, user_coll, user_rows, "user_key", model_version, updated_at)

    return {"items": n_items, "users": n_users, "legacy_migrated": sum(migrated)}


def generation_present(settings, generation: str, client=None) -> bool:
    """True when both of the generation's collections exist."""
    if client is None:
        client = _connect(settings)
    existing = _real_collections(client)
    return all(generation_collection(b, generation) in existing for b in _bases(settings))


def activate_aliases(settings, generation: str, client=None) -> None:
    """Point the item and user aliases at the generation's collections in ONE atomic call."""
    if client is None:
        client = _connect(settings)
    targets = {base: generation_collection(base, generation) for base in _bases(settings)}
    client.update_collection_aliases(change_aliases_operations=_alias_ops(client, targets))


def prune_generations(settings, keep: set[str], client=None) -> list[str]:
    """Delete every ``<base>__<gen>`` collection whose generation is not in ``keep``.

    Never touches a collection the aliases currently point at, and does nothing when ``keep`` is
    empty: no serving/previous generation could be named, which means "unknown", not "delete all"
    (serving no longer depends on the aliases protecting the live generation). Returns the names
    deleted.
    """
    if not keep:
        return []
    if client is None:
        client = _connect(settings)
    live = set(_alias_targets(client).values())
    deleted: list[str] = []
    for name in sorted(_real_collections(client)):
        for base in _bases(settings):
            prefix = f"{base}__"
            if name.startswith(prefix) and name[len(prefix) :] not in keep and name not in live:
                client.delete_collection(collection_name=name)
                deleted.append(name)
    return deleted


def load_two_tower_vectors(
    settings,
    model_version: str,
    item_vectors: dict[str, list[float]],
    client=None,
) -> int:
    """Upsert Two-Tower candidate item vectors and prune stale generations."""
    if client is None:
        client = _connect(settings)

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
