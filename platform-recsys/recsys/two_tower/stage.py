"""The two-tower stage of the batch run: governed features and pairs in, publishable vectors out.

Inputs, all governed:
- the item catalogue and item features: the latest ``item_popularity@v1`` snapshot (every row is a
  catalogue item, so an item with no interactions in the dataset still gets a vector);
- user features: the latest ``user_activity@v2`` snapshot (a user without a row trains as all zeros);
- attributes (optional, see ``TWO_TOWER_REQUIRE_ATTRIBUTES``): the latest ``item_attributes@v1`` snapshot
  (category, price; a listing present only here is in the catalogue with no engagement) and the latest
  ``user_preferences@v1`` snapshot (preferred categories);
- the training pairs: the run's own ``als_interactions`` dataset pairs (sampled to
  ``TWO_TOWER_MAX_PAIRS``); a seeded fraction of them (``TWO_TOWER_HOLDOUT_FRACTION``) is held out and
  the towers are judged on it (``recall@10`` and ``coverage@10``) before they can publish.
"""

from __future__ import annotations

import logging
import random
import tempfile
from collections.abc import Sequence
from dataclasses import dataclass, field
from pathlib import Path
from typing import Any

import pandas as pd
from modelregistry import Decision

from recsys import registry_client as rc
from recsys.config import Settings
from recsys.two_tower import features
from recsys.two_tower.pipeline import TwoTowerReport, train_and_index_two_tower

log = logging.getLogger("recsys.two_tower.stage")

ITEM_VIEW, ITEM_VERSION = "item_popularity", 1
USER_VIEW, USER_VERSION = "user_activity", 2
ATTRIBUTES_VIEW, ATTRIBUTES_VERSION = "item_attributes", 1
PREFERENCES_VIEW, PREFERENCES_VERSION = "user_preferences", 1

# The candidate's evaluation protocol: a seeded holdout of the training pairs, retrieval over the catalogue.
EVAL_PROTOCOL = "seeded-pair-holdout-v1"
MIN_PAIRS = 10
RETRIEVAL_K = 10


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
    """``decision`` is the registry's verdict on the candidate (None when there is no candidate: skipped);
    ``metrics`` are its held-out ``recall@10`` / ``coverage@10``."""

    vectors: dict[str, list[float]]
    report: TwoTowerReport
    lineage: dict
    decision: Decision | None = None
    metrics: dict[str, float] = field(default_factory=dict)
    reason: str = ""

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


def split_pairs(
    pairs: Sequence[tuple[str, str]], fraction: float, seed: int = 42
) -> tuple[list[tuple[str, str]], list[tuple[str, str]]]:
    """Seeded split into (train, held-out) pairs: ``round(len * fraction)`` pairs (at least one, never all)
    are held out. The two lists are disjoint and the split is reproducible for a given seed."""
    pairs = list(pairs)
    if len(pairs) < 2:
        return pairs, []
    n_held = min(len(pairs) - 1, max(1, round(len(pairs) * fraction)))
    held_idx = set(random.Random(seed).sample(range(len(pairs)), n_held))
    train = [p for i, p in enumerate(pairs) if i not in held_idx]
    held = [p for i, p in enumerate(pairs) if i in held_idx]
    return train, held


@dataclass
class _Loaded:
    catalog: list[dict[str, Any]]
    user_rows: dict[str, dict]
    pref_rows: dict[str, dict]
    vocab: list[str]
    attr_count: int


def _load(settings: Settings, inputs: StageInputs) -> _Loaded:
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
    vocab = (
        features.category_vocabulary(attr_rows, settings.two_tower_max_categories)
        if inputs.attributes_snapshot
        else []
    )
    return _Loaded(catalog, user_rows, pref_rows, vocab, len(attr_rows))


def _train(
    settings: Settings,
    loaded: _Loaded,
    pairs: list[tuple[str, str]],
    users: list[str],
    profiles_by_user: dict[str, dict] | None = None,
):
    """Train on ``pairs``; ``users`` are the users that get a profile (when they have a feature row)."""
    profiles = [
        {"user_key": u, **features.user_features(loaded.user_rows.get(u, {}), loaded.pref_rows.get(u))}
        for u in users
        if u in loaded.user_rows or u in loaded.pref_rows
    ]
    if profiles_by_user is not None:
        profiles_by_user.update({p["user_key"]: p for p in profiles})
    log.info(
        "two-tower stage: %d catalogue items (%d with attributes), %d/%d users with features, "
        "%d training pairs, %d categories",
        len(loaded.catalog),
        loaded.attr_count,
        len(profiles),
        len(users),
        len(pairs),
        len(loaded.vocab),
    )
    return train_and_index_two_tower(
        loaded.catalog,
        profiles,
        embedding_dim=settings.two_tower_dim,
        pairs=pairs,
        epochs=settings.two_tower_epochs,
        lr=settings.two_tower_lr,
        batch_size=settings.two_tower_batch_size,
        temperature=settings.two_tower_temperature,
        category_vocab=loaded.vocab or None,
    )


def _holdout_metrics(
    model, vectors: dict[str, list[float]], profiles: dict[str, dict], held: list[tuple[str, str]]
) -> dict[str, float]:
    """recall@10 (mean over the held-out users) and coverage@10 (share of the indexed catalogue that
    appears in some held-out user's top 10) of item-tower retrieval against the held-out pairs."""
    wanted: dict[str, set[str]] = {}
    for u, i in held:
        wanted.setdefault(u, set()).add(i)
    recalls: list[float] = []
    retrieved: set[str] = set()
    for u, items in wanted.items():
        u_vec = model.user_tower.project(profiles.get(u, {}))
        ranked = sorted(
            vectors, key=lambda lid: (-sum(a * b for a, b in zip(u_vec, vectors[lid], strict=False)), lid)
        )[:RETRIEVAL_K]
        retrieved.update(ranked)
        recalls.append(len(items & set(ranked)) / len(items))
    return {
        "recall@10": round(sum(recalls) / len(recalls), 6),
        "coverage@10": round(len(retrieved) / len(vectors), 6),
    }


def _skipped(reason: str) -> StageResult:
    log.info("two-tower candidate skipped: %s", reason)
    return StageResult(vectors={}, report=TwoTowerReport(), lineage={}, reason=reason)


def run_stage(
    settings: Settings,
    inputs: StageInputs,
    pairs: list[tuple[str, str]],
    model_version: str | None = None,
) -> StageResult:
    """Train the towers and embed the snapshot's catalogue.

    With ``model_version`` the stage is a candidate: a seeded fraction of ``pairs``
    (``TWO_TOWER_HOLDOUT_FRACTION``) is held out, the towers train on the rest, ``recall@10`` and
    ``coverage@10`` are measured on the held-out pairs, and the result is registered and evaluated as
    ``recsys-two-tower`` (``decision``; the pipeline applies it after the publish). Fewer than 10 pairs, or
    a holdout without a pair on a catalogue item, make no candidate: nothing is registered and no vectors
    are returned. Without ``model_version`` it only trains and embeds (no holdout, no registry)."""
    loaded = _load(settings, inputs)
    if model_version is None:
        users = sorted({u for u, _ in pairs})
        model, vectors = _train(settings, loaded, pairs, users)
        # category_vocab: size of the vocabulary built from the attribute snapshot (0: the default one).
        lineage = {**inputs.lineage, "category_vocab": len(loaded.vocab)}
        return StageResult(vectors=vectors, report=model.report, lineage=lineage)

    if len(pairs) < MIN_PAIRS:
        return _skipped(f"{len(pairs)} pairs, fewer than the {MIN_PAIRS} a holdout needs")
    train, held = split_pairs(pairs, settings.two_tower_holdout_fraction)
    catalogue = {str(it["listing_id"]) for it in loaded.catalog}
    held = [(u, i) for u, i in held if i in catalogue]
    if not held:
        return _skipped("no held-out pair is on a catalogue item")

    profiles: dict[str, dict] = {}
    model, vectors = _train(settings, loaded, train, sorted({u for u, _ in pairs}), profiles)
    metrics = _holdout_metrics(model, vectors, profiles, held)
    lineage = {**inputs.lineage, "category_vocab": len(loaded.vocab)}

    run_id = rc.start_run(
        f"recsys-two-tower-{model_version}",
        {
            "two_tower_dim": settings.two_tower_dim,
            "two_tower_epochs": settings.two_tower_epochs,
            "two_tower_lr": settings.two_tower_lr,
            "two_tower_batch_size": settings.two_tower_batch_size,
            "two_tower_temperature": settings.two_tower_temperature,
            "two_tower_max_pairs": settings.two_tower_max_pairs,
            "two_tower_holdout_fraction": settings.two_tower_holdout_fraction,
        },
    )
    tags: dict[str, Any] = {
        "eval_protocol": EVAL_PROTOCOL,
        "model_version": model_version,
        **rc.snapshot_lineage("item_features", inputs.item_snapshot.lineage),
        **rc.snapshot_lineage("user_features", inputs.user_snapshot.lineage),
    }
    with tempfile.TemporaryDirectory() as tmp:
        pd.DataFrame({"listing_id": list(vectors), "vector": list(vectors.values())}).to_parquet(
            Path(tmp) / "item_vectors.parquet", index=False
        )
        decision = rc.evaluate(rc.TWO_TOWER_MODEL, run_id, metrics, Path(tmp), tags)
    return StageResult(
        vectors=vectors,
        report=model.report,
        lineage=lineage,
        decision=decision,
        metrics=metrics,
        reason=decision.reason,
    )
