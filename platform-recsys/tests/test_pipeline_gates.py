"""The two-phase pipeline: evaluate every candidate, publish, then apply (RAT-07..RAT-11, 17, 18, 45..47).

Needs PySpark and a JVM (local mode); skipped without one. State is read from the MLflow file store of the
``reg`` fixture and from the in-memory Redis/Qdrant fakes. See tests/pipeline_fixture.py for the interface
assumptions.
"""

from __future__ import annotations

import json
import logging

import pytest

pytest.importorskip("pyspark")

import modelregistry  # noqa: E402
from modelregistry import RegistryUnavailable  # noqa: E402

from recsys.pipeline import run  # noqa: E402
from tests.dataset_fixture import write_dataset  # noqa: E402
from tests.fakes import FakeQdrantClient, FakeRedis  # noqa: E402
from tests.pipeline_fixture import (  # noqa: E402
    ALS,
    GBDT,
    TWO_TOWER,
    aliases_of,
    als_rows,
    gens,
    newest,
    outcome,
    pointers,
    registered_models,
    requires_spark,
    run_main,
    seed_strong,
    seed_weak,
    settings,
    three_model_world,
    versions,
)

pytestmark = requires_spark

ALL = (ALS, GBDT, TWO_TOWER)


def _cfg3(root, reg, monkeypatch, version="g1", **kw):
    world = three_model_world(root, monkeypatch)
    data = world.pop("dataset_dir")
    return settings(
        data, version, reg, enable_gbdt=True, enable_two_tower=True, gbdt_trees=20,
        gbdt_ctr_min_impressions=10**6, two_tower_dim=8, **{**world, **kw},
    )  # fmt: skip


def _cfg_als(root, reg, version="g1", **kw):
    write_dataset(root, als_rows())
    return settings(root, version, reg, **kw)


class _QdrantFailing(FakeQdrantClient):
    def __init__(self, exc: BaseException):
        super().__init__()
        self._exc = exc

    def update_collection_aliases(self, change_aliases_operations):
        raise self._exc


class _Killed(BaseException):
    """The process dies: no handler may run (not an Exception)."""


def test_all_three_promoted_in_one_generation(tmp_path, reg, monkeypatch):
    # RAT-07
    cfg = _cfg3(tmp_path, reg, monkeypatch)
    old = seed_weak(reg, *ALL)
    r, q = FakeRedis(), FakeQdrantClient()

    summary = run(cfg, redis_client=r, qdrant_client=q)

    for m in ALL:
        a = reg.aliases(m)
        assert a["champion"] != old[m] and a["previous"] == old[m], (m, a)
        assert outcome(reg, m, a["champion"]) == "promoted"
    assert summary["decision"] == "promoted"
    assert r.store["recs:v1:serving"] == "g1"
    assert json.loads(r.store["recs:v1:gen:g1:ranker"])["generation"] == "g1"
    assert "item_two_tower_vectors__g1" in q.collections
    # the version records the generation it built (rollback finds the version through it)
    assert reg.tags(ALS, reg.aliases(ALS)["champion"])["lineage.model_version"] == "g1"


def test_gbdt_rejected_generation_ships_without_ranker(tmp_path, reg, monkeypatch):
    # RAT-08
    cfg = _cfg3(tmp_path, reg, monkeypatch)
    old = seed_strong(reg, GBDT)
    r, q = FakeRedis(), FakeQdrantClient()

    summary = run(cfg, redis_client=r, qdrant_client=q)

    new = [v for v in versions(reg, GBDT) if v != old[GBDT]]
    assert len(new) == 1 and outcome(reg, GBDT, new[0]) == "rejected"
    assert "ndcg@10" in reg.tags(GBDT, new[0])["gate.reason"]
    assert reg.aliases(GBDT) == {"champion": old[GBDT]}
    assert "recs:v1:gen:g1:ranker" not in r.store
    assert r.store["recs:v1:serving"] == "g1" and summary["decision"] == "promoted"
    assert summary["gbdt"]["decision"] == "rejected"
    assert outcome(reg, ALS, reg.aliases(ALS)["champion"]) == "promoted"


def test_two_tower_rejected_generation_ships_without_vectors(tmp_path, reg, monkeypatch):
    # RAT-09
    cfg = _cfg3(tmp_path, reg, monkeypatch)
    old = seed_strong(reg, TWO_TOWER)
    r, q = FakeRedis(), FakeQdrantClient()

    summary = run(cfg, redis_client=r, qdrant_client=q)

    new = [v for v in versions(reg, TWO_TOWER) if v != old[TWO_TOWER]]
    assert len(new) == 1 and outcome(reg, TWO_TOWER, new[0]) == "rejected"
    assert reg.aliases(TWO_TOWER) == {"champion": old[TWO_TOWER]}
    assert not [c for c in q.collections if "two_tower" in c]
    assert r.store["recs:v1:serving"] == "g1" and summary["decision"] == "promoted"
    assert summary["two_tower"]["decision"] == "rejected"
    assert "recs:v1:gen:g1:ranker" in r.store  # the GBDT model of the same generation still ships


def test_als_rejected_skips_other_stages(tmp_path, reg, monkeypatch):
    # RAT-10: neither stage may even run
    def stage_must_not_run(*_a, **_k):
        raise AssertionError("a stage ran after a rejected ALS candidate")

    monkeypatch.setattr("recsys.ranker.stage.run_stage", stage_must_not_run)
    monkeypatch.setattr("recsys.two_tower.stage.run_stage", stage_must_not_run)
    cfg = _cfg3(tmp_path, reg, monkeypatch)
    seed_strong(reg, ALS)
    r, q = FakeRedis(), FakeQdrantClient()

    assert run_main(monkeypatch, cfg, r, q) == 0

    assert registered_models(reg) == {ALS}
    assert outcome(reg, ALS, newest(reg, ALS)) == "rejected"
    assert not r.store and not q.collections


@pytest.mark.parametrize(
    ("stage_path", "label"),
    [
        ("recsys.ranker.stage.run_stage", "gbdt"),
        ("recsys.two_tower.stage.run_stage", "two-tower"),
    ],
)
def test_stage_failure_marks_rejected_and_exits_1(tmp_path, reg, monkeypatch, stage_path, label):
    # RAT-11
    def boom(*_a, **_k):
        raise RuntimeError("stage exploded")

    cfg = _cfg3(tmp_path, reg, monkeypatch)
    seed_weak(reg, *ALL)
    before_aliases = aliases_of(reg, *ALL)
    r, q = FakeRedis(), FakeQdrantClient()
    r.store["recs:v1:serving"] = "g0"
    before_store = dict(r.store)
    monkeypatch.setattr(stage_path, boom)

    assert run_main(monkeypatch, cfg, r, q) == 1

    v = newest(reg, ALS)
    assert outcome(reg, ALS, v) == "rejected"
    assert reg.tags(ALS, v)["gate.reason"].startswith(f"{label} stage failed")
    assert aliases_of(reg, *ALL) == before_aliases
    assert r.store == before_store and not q.collections


def test_publish_failure_marks_rejected_aliases_untouched(tmp_path, reg, monkeypatch):
    # RAT-17: the Qdrant write fails after every candidate was evaluated promotable
    applied = []
    monkeypatch.setattr(modelregistry, "apply", lambda *a, **k: applied.append(a))
    cfg = _cfg3(tmp_path, reg, monkeypatch)
    seed_weak(reg, *ALL)
    before_aliases = aliases_of(reg, *ALL)
    r, q = FakeRedis(), _QdrantFailing(RuntimeError("qdrant unreachable"))
    r.store["recs:v1:serving"] = "g0"
    before_pointers = pointers(r)

    assert run_main(monkeypatch, cfg, r, q) == 1

    assert not applied
    for m in ALL:
        v = newest(reg, m)
        assert outcome(reg, m, v) == "rejected", m
        assert reg.tags(m, v)["gate.reason"] == "publish failed: RuntimeError"
    assert aliases_of(reg, *ALL) == before_aliases
    assert pointers(r) == before_pointers


def test_crash_after_evaluate_leaves_aliases(tmp_path, reg):
    # RAT-45: killed after evaluate returned promotable and before the pointer swap
    cfg = _cfg_als(tmp_path / "d", reg, "g1")
    old = seed_weak(reg, ALS)
    r = FakeRedis()
    r.store["recs:v1:serving"] = "g0"
    before_pointers = pointers(r)

    with pytest.raises(_Killed):
        run(cfg, redis_client=r, qdrant_client=_QdrantFailing(_Killed()))

    stale = newest(reg, ALS)
    assert stale != old[ALS] and outcome(reg, ALS, stale) == "promotable"
    assert reg.aliases(ALS) == {"champion": old[ALS]}
    assert pointers(r) == before_pointers

    # the next run is judged against the old champion and ignores the stale promotable version
    again = run(
        _cfg_als(tmp_path / "d2", reg, "g2"), redis_client=FakeRedis(), qdrant_client=FakeQdrantClient()
    )
    assert again["decision"] == "promoted"
    fresh = reg.aliases(ALS)["champion"]
    assert fresh not in (old[ALS], stale)
    assert reg.aliases(ALS)["previous"] == old[ALS]
    assert outcome(reg, ALS, stale) == "promotable"


def test_apply_failure_exits_1_and_logs_gap(tmp_path, reg, monkeypatch, caplog):
    # RAT-46: the pointer swap succeeded, then the registry is down at apply time
    def down(*_a, **_k):
        raise RegistryUnavailable("registry unavailable: down")

    monkeypatch.setattr(modelregistry, "apply", down)
    cfg = _cfg3(tmp_path, reg, monkeypatch)
    old = seed_weak(reg, *ALL)
    r, q = FakeRedis(), FakeQdrantClient()

    with caplog.at_level(logging.INFO):
        assert run_main(monkeypatch, cfg, r, q) == 1

    assert r.store["recs:v1:serving"] == "g1"  # the generation is already serving
    for m in ALL:
        assert reg.aliases(m) == {"champion": old[m]}
        assert outcome(reg, m, newest(reg, m)) == "promotable"
        assert m in caplog.text, f"the log must name {m} among the models whose champion was not moved"


def test_rejected_run_refreshes_ttl(tmp_path, reg):
    # RAT-18
    d = tmp_path / "d"
    write_dataset(d, als_rows())
    r, q = FakeRedis(), FakeQdrantClient()
    for gen in ("g1", "g2"):
        seed_weak(reg, ALS)
        assert run(settings(d, gen, reg), redis_client=r, qdrant_client=q)["decision"] == "promoted"
    assert gens(r) == {"g1", "g2"}
    for k in [k for k in r.store if k.startswith("recs:v1:gen:")]:
        r.ttl_set[k] = 5
    seed_strong(reg, ALS)
    cfg = settings(d, "g3", reg)

    summary = run(cfg, redis_client=r, qdrant_client=q)

    assert summary["decision"] == "rejected"
    keys = [k for k in r.store if k.startswith("recs:v1:gen:")]
    assert keys and gens(r) == {"g1", "g2"}
    assert {r.ttl(k) for k in keys} == {cfg.cache_ttl_seconds}


def test_unexpected_exception_exit_code(tmp_path, reg, monkeypatch):
    # RAT-47: the ALS trainer raises -> exit 1, no version registered, nothing published
    def boom(*_a, **_k):
        raise RuntimeError("trainer exploded")

    monkeypatch.setattr("recsys.pipeline.train_als", boom)
    monkeypatch.setattr("recsys.train.train_als", boom)
    cfg = _cfg_als(tmp_path / "d", reg)
    r, q = FakeRedis(), FakeQdrantClient()

    assert run_main(monkeypatch, cfg, r, q) == 1

    assert versions(reg, ALS) == [] and not registered_models(reg) & set(ALL)
    assert not r.store and not q.collections


def _every_stored_string(reg) -> list[str]:
    """Every model-version tag, run tag, run param and run name of every recsys-* model."""
    out: list[str] = []
    for m in reg.client.search_registered_models():
        if not m.name.startswith("recsys-"):
            continue
        for mv in reg.client.search_model_versions(f"name='{m.name}'"):
            out += [f"{k}={v}" for k, v in mv.tags.items()]
            run = reg.client.get_run(mv.run_id)
            out += [f"{k}={v}" for k, v in run.data.tags.items()]
            out += [f"{k}={v}" for k, v in run.data.params.items()]
            out.append(run.info.run_name or "")
    for exp in reg.client.search_experiments():
        for run in reg.client.search_runs([exp.experiment_id]):
            out += [f"{k}={v}" for k, v in run.data.tags.items()]
            out += [f"{k}={v}" for k, v in run.data.params.items()]
            out.append(run.info.run_name or "")
    return out


@pytest.mark.parametrize(
    ("stage_path", "label"),
    [
        ("recsys.ranker.stage.run_stage", "gbdt"),
        ("recsys.two_tower.stage.run_stage", "two-tower"),
    ],
)
def test_stage_failure_reason_carries_the_class_name_only_never_a_user_key(
    tmp_path, reg, monkeypatch, stage_path, label
):
    # RAT-11 / RAT-26 (privacy fix F1): str(exc) may embed a user key and must never reach the registry
    def boom(*_a, **_k):
        raise KeyError("u-0001")

    cfg = _cfg3(tmp_path, reg, monkeypatch)
    seed_weak(reg, *ALL)
    r, q = FakeRedis(), FakeQdrantClient()
    monkeypatch.setattr(stage_path, boom)

    assert run_main(monkeypatch, cfg, r, q) == 1

    v = newest(reg, ALS)
    assert outcome(reg, ALS, v) == "rejected"
    assert reg.tags(ALS, v)["gate.reason"] == f"{label} stage failed: KeyError"
    leaked = [x for x in _every_stored_string(reg) if "u-0001" in x]
    assert leaked == []
