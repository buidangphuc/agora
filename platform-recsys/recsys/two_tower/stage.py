"""The two-tower stage of the batch run: governed features and pairs in, publishable vectors out.

Inputs, all governed:
- the item catalogue and item features: the latest ``item_popularity@v1`` snapshot (every row is a
  catalogue item, so an item with no interactions in the dataset still gets a vector);
- user features: the latest ``user_activity@v2`` snapshot (a user without a row trains as all zeros);
- attributes (optional, see ``TWO_TOWER_REQUIRE_ATTRIBUTES``): the latest ``item_attributes@v1`` snapshot
  (category, price; a listing present only here is in the catalogue with no engagement) and the latest
  ``user_preferences@v1`` snapshot (preferred categories);
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
ATTRIBUTES_VIEW, ATTRIBUTES_VERSION = "item_attributes", 1
PREFERENCES_VIEW, PREFERENCES_VERSION = "user_preferences", 1


@dataclass(frozen=True)
class StageInputs:
    item_snapshot: features.Snapshot
    user_snapshot: features.Snapshot
    attributes_snapshot: features.Snapshot | None = None
    preferences_snapshot: features.Snapshot | None = None

    @property
    def lineage(self) -> dict:
        return {
            "items": self.item_snapshot.lineage,
            "users": self.user_snapshot.lineage,
            "attributes": self.attributes_snapshot.lineage if self.attributes_snapshot else None,
            "preferences": self.preferences_snapshot.lineage if self.preferences_snapshot else None,
        }


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
    item = features.resolve_snapshot(
        settings.item_features_dir, settings.item_features_path, ITEM_VIEW, ITEM_VERSION, "ITEM_FEATURES"
    )
    user = features.resolve_snapshot(
        settings.user_features_dir, settings.user_features_path, USER_VIEW, USER_VERSION, "USER_FEATURES"
    )
    resolve_attributes = (
        features.resolve_snapshot
        if settings.two_tower_require_attributes
        else features.resolve_optional_snapshot
    )
    attributes = resolve_attributes(
        settings.item_attributes_dir,
        settings.item_attributes_path,
        ATTRIBUTES_VIEW,
        ATTRIBUTES_VERSION,
        "ITEM_ATTRIBUTES",
    )
    preferences = features.resolve_optional_snapshot(
        settings.user_preferences_dir,
        settings.user_preferences_path,
        PREFERENCES_VIEW,
        PREFERENCES_VERSION,
        "USER_PREFERENCES",
    )
    return StageInputs(item, user, attributes, preferences)


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
    attr_rows = (
        features.read_rows(inputs.attributes_snapshot, "listing_id") if inputs.attributes_snapshot else {}
    )
    pref_rows = (
        features.read_rows(inputs.preferences_snapshot, "user_key") if inputs.preferences_snapshot else {}
    )
    # A listing with attributes but no engagement yet is a catalogue item: that is the cold start.
    listing_ids = list(item_rows) + [lid for lid in attr_rows if lid not in item_rows]
    catalog = [
        {"listing_id": lid, **features.item_features(item_rows.get(lid, {}), attr_rows.get(lid))}
        for lid in listing_ids
    ]
    users = sorted({u for u, _ in pairs})
    profiles = [
        {"user_key": u, **features.user_features(user_rows.get(u, {}), pref_rows.get(u))}
        for u in users
        if u in user_rows or u in pref_rows
    ]
    vocab = (
        features.category_vocabulary(attr_rows, settings.two_tower_max_categories)
        if inputs.attributes_snapshot
        else []
    )
    log.info(
        "two-tower stage: %d catalogue items (%d with attributes), %d/%d training users with features, "
        "%d pairs, %d categories",
        len(catalog),
        len(attr_rows),
        len(profiles),
        len(users),
        len(pairs),
        len(vocab),
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
        category_vocab=vocab or None,
    )
    # category_vocab: size of the vocabulary built from the attribute snapshot (0: the default one).
    return StageResult(
        vectors=vectors, report=_model.report, lineage={**inputs.lineage, "category_vocab": len(vocab)}
    )
