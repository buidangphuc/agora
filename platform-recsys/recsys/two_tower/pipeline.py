"""Two-Tower retrieval pipeline for dense item candidate indexing."""

from __future__ import annotations

import logging
from typing import Any

from recsys.two_tower.model import TwoTowerModel

logger = logging.getLogger("recsys.two_tower.pipeline")


def train_and_index_two_tower(
    catalog_items: list[dict[str, Any]],
    user_profiles: list[dict[str, Any]] | None = None,
    embedding_dim: int = 32,
) -> tuple[TwoTowerModel, dict[str, list[float]]]:
    """Trains/initializes TwoTowerModel and indexes full catalog (including cold items)."""
    model = TwoTowerModel(embedding_dim=embedding_dim)
    model.index_items(catalog_items)

    item_vectors: dict[str, list[float]] = {}
    for it in catalog_items:
        lid = str(it.get("listing_id") or it.get("id", ""))
        if lid:
            item_vectors[lid] = model.item_tower.project(it)

    logger.info("Indexed %d items via TwoTowerModel", len(item_vectors))
    return model, item_vectors
