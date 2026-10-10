"""Two-tower retrieval pipeline: train on interactions, embed the catalogue, refuse degenerate vectors."""

from __future__ import annotations

import logging
import math
from collections.abc import Sequence
from dataclasses import dataclass, field
from typing import Any

from recsys.two_tower.model import TwoTowerModel
from recsys.two_tower.train import train_towers

logger = logging.getLogger("recsys.two_tower.pipeline")

_ZERO_NORM = 1e-9


class TwoTowerError(RuntimeError):
    """The two-tower stage cannot produce vectors that are safe to publish."""


class DegenerateEmbeddingError(TwoTowerError):
    """Every item embedding is all zeros or not finite."""


@dataclass
class TwoTowerReport:
    """What a training/indexing pass did (recorded on the model and in the run summary)."""

    items: int = 0  # vectors produced, i.e. safe to upsert
    refused: list[str] = field(default_factory=list)  # listing ids whose vector was zero / not finite
    pairs: int = 0  # training pairs used
    epochs: int = 0
    losses: list[float] = field(default_factory=list)  # mean loss per epoch

    def as_dict(self) -> dict[str, Any]:
        return {
            "items": self.items,
            "refused": len(self.refused),
            "pairs": self.pairs,
            "epochs": len(self.losses),
            "loss_first": round(self.losses[0], 6) if self.losses else None,
            "loss_last": round(self.losses[-1], 6) if self.losses else None,
        }


def is_degenerate(vector: Sequence[float]) -> bool:
    """True for a vector that carries no direction: all zeros, or any non-finite value."""
    if any(not math.isfinite(v) for v in vector):
        return True
    return math.sqrt(sum(v * v for v in vector)) <= _ZERO_NORM


def train_and_index_two_tower(
    catalog_items: list[dict[str, Any]],
    user_profiles: list[dict[str, Any]] | None = None,
    embedding_dim: int = 32,
    *,
    pairs: Sequence[tuple[str, str]] | None = None,
    epochs: int = 5,
    lr: float = 0.05,
    batch_size: int = 256,
    temperature: float = 0.1,
    seed: int = 42,
    category_vocab: list[str] | None = None,
) -> tuple[TwoTowerModel, dict[str, list[float]]]:
    """Train the towers on ``pairs`` of (user_key, listing_id), then embed the whole catalogue.

    ``catalog_items`` are item feature dicts with a ``listing_id`` (cold items included: an item needs
    features, not interactions). ``user_profiles`` are user feature dicts with a ``user_key``. Without
    ``pairs`` (or ``epochs=0``) the towers keep their initial weights. ``category_vocab`` replaces the
    towers' built-in category names (the stage builds it from the item attribute snapshot). Returns the
    model (its ``report`` says what happened) and the vectors that are safe to publish: a zero or
    non-finite vector is refused and counted, and if no vector is left ``DegenerateEmbeddingError`` is
    raised.
    """
    model = TwoTowerModel(embedding_dim=embedding_dim, category_vocab=category_vocab, seed=seed)

    item_features = {
        str(it.get("listing_id") or it.get("id", "")): it
        for it in catalog_items
        if it.get("listing_id") or it.get("id")
    }
    user_features = {str(p["user_key"]): p for p in (user_profiles or []) if p.get("user_key")}
    losses = train_towers(
        model,
        list(pairs or []),
        user_features,
        item_features,
        epochs=epochs,
        lr=lr,
        batch_size=batch_size,
        temperature=temperature,
        seed=seed,
    )

    vectors: dict[str, list[float]] = {}
    refused: list[str] = []
    for lid, features in item_features.items():
        vec = model.item_tower.project(features)
        if is_degenerate(vec):
            refused.append(lid)
        else:
            vectors[lid] = vec
    model.index_items([item_features[lid] for lid in vectors])

    model.report = TwoTowerReport(
        items=len(vectors),
        refused=refused,
        pairs=len([1 for _, i in (pairs or []) if i in item_features]),
        epochs=epochs,
        losses=losses,
    )
    if refused:
        logger.warning("refused %d degenerate item vectors (e.g. %s)", len(refused), refused[:5])
    if not vectors:
        raise DegenerateEmbeddingError(
            f"no usable two-tower vector: {len(refused)} of {len(item_features)} catalogue items "
            "embed to zero"
            if item_features
            else "the two-tower catalogue is empty"
        )
    logger.info("Indexed %d items via TwoTowerModel (loss %s)", len(vectors), losses[-1:] or "untrained")
    return model, vectors
