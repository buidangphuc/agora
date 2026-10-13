"""The job fails closed when the registry is unreachable (RAT-19, RAT-20) and maps errors to exit codes (D4).

The Spark-driven cases need PySpark and a JVM and are skipped without one.
"""

from __future__ import annotations

import socket
import time

import pytest
from modelregistry import GateError, PolicyError, RegistryUnavailable, RollbackError

from recsys.config import ConfigError, Settings
from tests.dataset_fixture import write_dataset
from tests.fakes import FakeQdrantClient, FakeRedis
from tests.pipeline_fixture import (
    ALS,
    GBDT,
    aliases_of,
    als_rows,
    forbid_spark,
    newest,
    outcome,
    requires_spark,
    run_main,
    seed_weak,
    settings,
    three_model_world,
)


@pytest.fixture
def dead_registry_url(monkeypatch) -> str:
    s = socket.socket()
    s.bind(("127.0.0.1", 0))
    port = s.getsockname()[1]
    s.close()
    url = f"http://127.0.0.1:{port}"
    monkeypatch.setenv("MLFLOW_TRACKING_URI", url)
    return url


def test_registry_unreachable_at_start_exits_1_before_spark(tmp_path, monkeypatch, dead_registry_url):
    # RAT-19 (job level): pre-flight fails -> exit 1 well inside 20 s, Spark never starts, nothing written
    write_dataset(tmp_path, als_rows())
    forbid_spark(monkeypatch)
    r, q = FakeRedis(), FakeQdrantClient()
    r.store["recs:v1:serving"] = "g0"
    r.store["recs:v1:previous"] = "g-1"
    before = dict(r.store)
    cfg = Settings(
        spark_master="local[1]",
        dataset_dir=str(tmp_path),
        model_version="g1",
        mlflow_tracking_uri=dead_registry_url,
    )

    t0 = time.monotonic()
    code = run_main(monkeypatch, cfg, r, q)

    assert code == 1
    assert time.monotonic() - t0 < 20
    assert r.store == before and not q.collections


@requires_spark
def test_outage_after_als_evaluate_publishes_nothing(tmp_path, reg, monkeypatch):
    # RAT-20: the registry stops answering during the GBDT evaluation, after the ALS candidate was promotable
    import modelregistry

    real_evaluate = modelregistry.evaluate
    calls = []

    def flaky_evaluate(model, *a, **k):
        calls.append(model)
        if model != ALS:
            raise RegistryUnavailable("registry unavailable: connection refused")
        return real_evaluate(model, *a, **k)

    def down(*_a, **_k):
        raise RegistryUnavailable("registry unavailable: connection refused")

    world = three_model_world(tmp_path, monkeypatch)
    data = world.pop("dataset_dir")
    cfg = settings(data, "g1", reg, enable_gbdt=True, gbdt_trees=20, gbdt_ctr_min_impressions=10**6, **world)
    old = seed_weak(reg, ALS, GBDT)
    before = aliases_of(reg, ALS, GBDT)
    monkeypatch.setattr(modelregistry, "evaluate", flaky_evaluate)
    for name in ("apply", "mark_rejected", "register_rejected"):
        monkeypatch.setattr(modelregistry, name, down)
    r, q = FakeRedis(), FakeQdrantClient()
    r.store["recs:v1:serving"] = "g0"
    before_store = dict(r.store)

    assert run_main(monkeypatch, cfg, r, q) == 1

    assert calls[:2] == [ALS, GBDT]
    assert r.store == before_store and not q.collections
    assert aliases_of(reg, ALS, GBDT) == before
    # the ALS version may stay promotable (mark_rejected cannot run while the registry is down)
    assert outcome(reg, ALS, newest(reg, ALS)) == "promotable" and newest(reg, ALS) != old[ALS]


@pytest.mark.parametrize(
    ("exc", "code"),
    [
        (RegistryUnavailable("down"), 1),
        (RuntimeError("unexpected stage exception"), 1),
        (ConfigError("bad input"), 2),
        (PolicyError("no policy"), 2),
        (GateError("not promotable"), 2),
        (RollbackError("no previous"), 2),
    ],
)
def test_error_to_exit_code_mapping(reg, monkeypatch, exc, code):
    # D4: 1 = registry unavailable or unexpected; 2 = usage/config, policy, gate, rollback errors
    from recsys import __main__ as entry

    def boom(*_a, **_k):
        raise exc

    monkeypatch.setattr(entry, "run", boom)
    assert entry.main([]) == code


def test_a_completed_run_exits_0_even_when_rejected(reg, monkeypatch):
    # D4: a rejected decision is a completed run
    from recsys import __main__ as entry

    monkeypatch.setattr(entry, "run", lambda *_a, **_k: {"decision": "rejected", "reason": "ndcg@10 ..."})
    assert entry.main([]) == 0
