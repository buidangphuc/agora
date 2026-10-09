"""The two-tower stage of the batch run: governed features and pairs in, publishable vectors out.

Inputs, all governed:
- the item catalogue and item features: the latest ``item_popularity@v1`` snapshot (every row is a
  catalogue item, so an item with no interactions in the dataset still gets a vector);
- user features: the latest ``user_activity@v2`` snapshot (a user without a row trains as all zeros);
- the training pairs: the run's own ``als_interactions`` dataset pairs (sampled to
  ``TWO_TOWER_MAX_PAIRS``).
"""

from __future__ import annotations

import logging
from dataclasses import dataclass
from typing import Any

from recsys.config import Settings
from recsys.two_tower import features
from recsys.two_tower.pipeline import TwoTowerReport, train_and_index_two_tower

log = logging.getLogger("recsys.two_tower.stage")

ITEM_VIEW, ITEM_VERSION = "item_popularity", 1
USER_VIEW, USER_VERSION = "user_activity", 2


@dataclass(frozen=True)
class StageInputs:
    item_snapshot: features.Snapshot
    user_snapshot: features.Snapshot

    @property
    def lineage(self) -> dict:
        return {"items": self.item_snapshot.lineage, "users": self.user_snapshot.lineage}


@dataclass
class StageResult:
    vectors: dict[str, list[float]]
    report: TwoTowerReport
    lineage: dict

    def as_parameters(self, dim: int) -> dict[str, Any]:
        """What the model's metadata records (``parameters["two_tower"]``)."""
        return {"dim": dim, **self.report.as_dict(), "features": self.lineage}


def resolve_inputs(settings: Settings) -> StageInputs:
    """Find both feature snapshots. Raises ConfigError (the job exits 2) when one is missing, so the
    run stops before Spark starts and nothing is registered."""
    return StageInputs(
        item_snapshot=features.resolve_snapshot(
            settings.item_features_dir, settings.item_features_path, ITEM_VIEW, ITEM_VERSION, "ITEM_FEATURES"
        ),
        user_snapshot=features.resolve_snapshot(
            settings.user_features_dir, settings.user_features_path, USER_VIEW, USER_VERSION, "USER_FEATURES"
        ),
    )


def collect_pairs(triples, max_pairs: int, seed: int = 42) -> list[tuple[str, str]]:
    """The run's (user_key, listing_id) pairs on the driver, a seeded sample when there are more than
    ``max_pairs``."""
    total = triples.count()
    frame = triples.select("user_key", "listing_id")
    if total > max_pairs > 0:
        frame = frame.sample(withReplacement=False, fraction=max_pairs / total, seed=seed)
    return [(r["user_key"], r["listing_id"]) for r in frame.collect()]


def run_stage(settings: Settings, inputs: StageInputs, pairs: list[tuple[str, str]]) -> StageResult:
    """Train the towers on ``pairs`` and embed the snapshot's catalogue."""
    item_rows = features.read_rows(inputs.item_snapshot, "listing_id")
    user_rows = features.read_rows(inputs.user_snapshot, "user_key")
    catalog = [{"listing_id": lid, **features.item_features(row)} for lid, row in item_rows.items()]
    users = sorted({u for u, _ in pairs})
    profiles = [{"user_key": u, **features.user_features(user_rows[u])} for u in users if u in user_rows]
    log.info(
        "two-tower stage: %d catalogue items, %d/%d training users with features, %d pairs",
        len(catalog),
        len(profiles),
        len(users),
        len(pairs),
    )
    _model, vectors = train_and_index_two_tower(
        catalog,
        profiles,
        embedding_dim=settings.two_tower_dim,
        pairs=pairs,
        epochs=settings.two_tower_epochs,
        lr=settings.two_tower_lr,
        batch_size=settings.two_tower_batch_size,
        temperature=settings.two_tower_temperature,
    )
    return StageResult(vectors=vectors, report=_model.report, lineage=inputs.lineage)
