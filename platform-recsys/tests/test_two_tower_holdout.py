"""The two-tower candidate is evaluated on a seeded holdout before it can publish (RAT-13, RAT-14, RAT-43).

INTERFACE ASSUMPTIONS (recsys/two_tower/stage.py):
  split_pairs(pairs, fraction, seed=42) -> (train_pairs, heldout_pairs)    pure, seeded, disjoint
  run_stage(settings, inputs, pairs, model_version) -> StageResult         as before plus
      .decision : modelregistry.Decision of ``evaluate`` (None when the stage is skipped: no candidate)
      .metrics  : {"recall@10": float, "coverage@10": float} on the held-out pairs ({} when skipped)
      .vectors  : the publishable item vectors ({} when skipped)
      .report.pairs : number of pairs the towers trained on
  A skipped stage registers nothing in the registry.
"""

from __future__ import annotations

import pytest

from recsys.config import Settings
from recsys.two_tower import stage
from tests.dataset_fixture import sample_rows, write_dataset
from tests.fakes import FakeQdrantClient, FakeRedis
from tests.pipeline_fixture import (
    ALS,
    TWO_TOWER,
    requires_spark,
    run_main,
    settings,
    tower_features,
    versions,
)

N_USERS, N_ITEMS = 50, 20
ITEMS = [f"it-{i:02d}" for i in range(N_ITEMS)]
USERS = [f"us-{n:02d}" for n in range(N_USERS)]
ALL_PAIRS = [(u, i) for u in USERS for i in ITEMS]  # 1000 distinct pairs


def _features(tmp_path):
    items = [[lid, 10 * (k + 1), k, k // 2, k, 0, None, 0.1] for k, lid in enumerate(ITEMS)]
    users = [[u, 3 * n, n % 5, 0, 0, 0, n % 2] for n, u in enumerate(USERS)]
    return tower_features(tmp_path, items, users)


def _stage(tmp_path, reg, pairs, version="g1", **kw):
    cfg = Settings(
        mlflow_tracking_uri=reg.tracking_uri, two_tower_dim=8, two_tower_epochs=1, **_features(tmp_path), **kw
    )
    return stage.run_stage(cfg, stage.resolve_inputs(cfg), list(pairs), version)


def _run_metrics(reg, version):
    run_id = reg.client.get_model_version(TWO_TOWER, str(version)).run_id
    return reg.client.get_run(run_id).data.metrics


def test_holdout_is_disjoint_and_reproducible(tmp_path, reg):
    # RAT-13: 1000 pairs, default fraction, seed 42 -> exactly 100 held out, none of them trained on
    train, held = stage.split_pairs(ALL_PAIRS, 0.1, seed=42)
    assert len(held) == 100 and len(train) == 900
    assert not set(held) & set(train)
    assert set(held) | set(train) == set(ALL_PAIRS)
    assert stage.split_pairs(ALL_PAIRS, 0.1, seed=42) == (train, held)  # reproducible
    assert stage.split_pairs(ALL_PAIRS, 0.1, seed=7)[1] != held  # and actually seeded

    result = _stage(tmp_path, reg, ALL_PAIRS)

    assert result.report.pairs == 900  # the towers trained on the remainder only
    assert result.decision is not None and result.decision.version in versions(reg, TWO_TOWER)
    for k in ("recall@10", "coverage@10"):
        assert 0.0 <= result.metrics[k] <= 1.0
    logged = _run_metrics(reg, result.decision.version)
    assert 0.0 <= logged["recall_at_10"] <= 1.0 and 0.0 <= logged["coverage_at_10"] <= 1.0


def test_too_few_pairs_skipped(tmp_path, reg):
    # RAT-14: 9 pairs -> no candidate, nothing registered, nothing to publish
    result = _stage(tmp_path, reg, ALL_PAIRS[:9])

    assert result.decision is None
    assert not result.vectors and not result.metrics
    assert versions(reg, TWO_TOWER) == []


def test_ten_pairs_is_a_candidate(tmp_path, reg):
    # RAT-43: exactly 10 pairs -> one held out, a version with numeric metrics is registered
    ten = ALL_PAIRS[:10]
    train, held = stage.split_pairs(ten, 0.1, seed=42)
    assert len(held) == 1 and len(train) == 9

    result = _stage(tmp_path, reg, ten)

    assert result.decision is not None
    assert versions(reg, TWO_TOWER) == [result.decision.version]
    logged = _run_metrics(reg, result.decision.version)
    assert {"recall_at_10", "coverage_at_10"} <= set(logged)


def test_a_holdout_without_usable_pairs_is_not_a_candidate(tmp_path, reg):
    # RAT-14 (holdout has no pairs): twelve pairs, none of them on a catalogue item
    unknown = [(u, f"not-in-catalogue-{n}") for n, u in enumerate(USERS[:12])]

    result = _stage(tmp_path, reg, unknown)

    assert result.decision is None and versions(reg, TWO_TOWER) == []


@requires_spark
def test_nine_pair_dataset_skips_the_stage_and_the_job_still_completes(tmp_path, reg, monkeypatch):
    # RAT-14 (job level): exits 0 with the two_tower decision "skipped"; ALS is unaffected
    from recsys.pipeline import run

    write_dataset(tmp_path / "d", sample_rows())  # 3 users x 3 listings = 9 pairs
    items = [[f"l{i}", 10 * i, i, 0, 0, 0, None, 0.1] for i in (1, 2, 3)]
    users = [[f"u{n}", 5 * n, n, 0, 0, 0, 0] for n in (1, 2, 3)]
    feats = tower_features(tmp_path, items, users)
    cfg = settings(
        tmp_path / "d", "g1", reg, enable_two_tower=True, two_tower_dim=8, gate_max_list_overlap=1.0, **feats
    )
    r, q = FakeRedis(), FakeQdrantClient()

    summary = run(cfg, redis_client=r, qdrant_client=q)

    assert summary["two_tower"]["decision"] == "skipped"
    assert versions(reg, TWO_TOWER) == []
    assert not [c for c in q.collections if "two_tower" in c]
    assert summary["decision"] == "promoted" and versions(reg, ALS)
    assert run_main(monkeypatch, cfg, FakeRedis(), FakeQdrantClient()) == 0


@pytest.mark.parametrize("fraction", ["0", "1", "1.5"])
def test_job_refuses_a_bad_holdout_fraction_before_spark(tmp_path, reg, monkeypatch, fraction):
    # RAT-44 (job level): exit 2, Spark never starts, nothing registered
    from recsys import __main__ as entry
    from tests.pipeline_fixture import forbid_spark

    write_dataset(tmp_path / "d", sample_rows())
    monkeypatch.setenv("DATASET_DIR", str(tmp_path / "d"))
    monkeypatch.setenv("TWO_TOWER_HOLDOUT_FRACTION", fraction)
    forbid_spark(monkeypatch)

    assert entry.main([]) == 2
    assert not reg.client.search_registered_models()
