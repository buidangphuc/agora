"""``python -m recsys migrate-registry``: the one-shot seed of the model registry from the Redis champions.

RAT-27..RAT-29 and RAT-51..RAT-53 (recsys-auto-train, decision D7). State is prepared with the raw MLflow
client / the library (``reg`` fixture) and the in-memory Redis fake; the outcome is read back from the
file-store registry and from the Redis keys. Nothing asserts that a mock was called.

INTERFACE ASSUMPTIONS (group C):
  - ``recsys.__main__.main(["migrate-registry"]) -> int`` (0 done, 1 registry unavailable), reading Redis
    through ``recsys.load.redis_cache.connect(settings)`` and the registry through the library (env
    ``MLFLOW_TRACKING_URI`` / ``Settings.mlflow_tracking_uri``).
  - the old Redis layout is read as it was written by the removed ``recsys.registry``:
    ``recs:model:champion`` (ALS) and ``recs:model:champion:gbdt`` hold a model version, and
    ``recs:model:meta:<version>`` holds JSON ``{"model_version", "model_type", "metrics": {<floats>,
    "eval_protocol": <str>}, ...}``; the ALS champion seeds ``recsys-als`` and the GBDT champion
    ``recsys-gbdt``; two-tower has no Redis champion and is never seeded.
  - the seeded v1 carries the metrics as stored (floats only; ``eval_protocol`` goes to the lineage tag),
    ``lineage.eval_protocol`` when the meta has one, and no artifact.
"""

from __future__ import annotations

import json
import socket
import time

import pytest

from recsys import __main__ as entry
from recsys.config import Settings
from recsys.load import redis_cache
from tests.fakes import FakeRedis
from tests.registry_fixture import enc

ALS = "recsys-als"
GBDT = "recsys-gbdt"
TWO_TOWER = "recsys-two-tower"
PROTOCOL = "leave-last-new-item-v1"


def _champion(r: FakeRedis, version: str, metrics: dict, protocol: str | None, gbdt: bool = False) -> None:
    stored = dict(metrics)
    if protocol is not None:
        stored["eval_protocol"] = protocol
    meta = {
        "model_name": GBDT if gbdt else ALS,
        "model_version": version,
        "model_type": "gbdt" if gbdt else "als",
        "status": "champion",
        "metrics": stored,
        "parameters": {},
        "git_commit": "",
        "created_at": "2026-10-12T00:00:00+00:00",
        "artifact_paths": {},
    }
    r.set(f"recs:model:meta:{version}", json.dumps(meta))
    r.set("recs:model:champion:gbdt" if gbdt else "recs:model:champion", version)


def _migrate(monkeypatch, reg, r: FakeRedis, tracking_uri: str | None = None) -> int:
    monkeypatch.setattr(redis_cache, "connect", lambda settings: r)
    cfg = Settings(mlflow_tracking_uri=tracking_uri or reg.tracking_uri)
    monkeypatch.setattr(entry, "load_settings", lambda: cfg)
    return entry.main(["migrate-registry"])


def _versions(reg, model: str) -> list[int]:
    return sorted(int(v.version) for v in reg.client.search_model_versions(f"name='{model}'"))


def _stored_metrics(reg, model: str, version: int) -> dict[str, float]:
    run_id = reg.client.get_model_version(model, str(version)).run_id
    return dict(reg.client.get_run(run_id).data.metrics)


def test_seeds_v1_champion(monkeypatch, reg):
    # RAT-27: the Redis champion becomes recsys-als v1 (@champion, promoted, "seed:" reason, protocol kept)
    r = FakeRedis()
    _champion(r, "als-2026-10-12", {"ndcg@10": 0.100, "coverage@10": 0.5}, PROTOCOL)
    r.set("recs:v1:serving", "g7")
    r.set("recs:v1:previous", "g6")

    assert _migrate(monkeypatch, reg, r) == 0

    assert _versions(reg, ALS) == [1]
    assert reg.aliases(ALS) == {"champion": 1}
    tags = reg.tags(ALS, 1)
    assert tags["lineage.eval_protocol"] == PROTOCOL
    assert tags["gate.outcome"] == "promoted"
    assert tags["gate.reason"].startswith("seed:")
    metrics = _stored_metrics(reg, ALS, 1)
    assert metrics[enc("ndcg@10")] == pytest.approx(0.100)
    assert metrics[enc("coverage@10")] == pytest.approx(0.5)
    # seeding never touches the serving pointer, and the Redis keys are left as they were
    assert r.store["recs:v1:serving"] == "g7" and r.store["recs:v1:previous"] == "g6"
    assert r.store["recs:model:champion"] == "als-2026-10-12"
    # the next nightly candidate is compared against v1 instead of auto-promoting
    weaker = reg.evaluate(ALS, {"ndcg@10": 0.05, "coverage@10": 0.5}, protocol=PROTOCOL)
    assert weaker.outcome == "rejected"


def test_gbdt_champion_seeds_recsys_gbdt_and_two_tower_is_never_seeded(monkeypatch, reg):
    # RAT-27 (second model): recs:model:champion:gbdt seeds recsys-gbdt; two-tower has no Redis champion
    r = FakeRedis()
    _champion(r, "als-1", {"ndcg@10": 0.1, "coverage@10": 0.5}, PROTOCOL)
    _champion(r, "gbdt-1", {"ndcg@10": 0.2, "recall@10": 0.4}, "gbdt-protocol-v1", gbdt=True)

    assert _migrate(monkeypatch, reg, r) == 0

    assert _versions(reg, GBDT) == [1]
    assert reg.aliases(GBDT) == {"champion": 1}
    assert reg.tags(GBDT, 1)["lineage.eval_protocol"] == "gbdt-protocol-v1"
    assert _stored_metrics(reg, GBDT, 1)[enc("recall@10")] == pytest.approx(0.4)
    assert _versions(reg, TWO_TOWER) == []


def test_nothing_to_seed_exits_0(monkeypatch, reg):
    # RAT-27 (no Redis champion at all): nothing is created, the command still succeeds
    assert _migrate(monkeypatch, reg, FakeRedis()) == 0
    assert _versions(reg, ALS) == [] and _versions(reg, GBDT) == []


def test_second_run_is_noop(monkeypatch, reg):
    # RAT-28: a second run leaves exactly one version, the alias unchanged, exit 0
    r = FakeRedis()
    _champion(r, "als-2026-10-12", {"ndcg@10": 0.100, "coverage@10": 0.5}, PROTOCOL)
    assert _migrate(monkeypatch, reg, r) == 0
    tags_before = reg.tags(ALS, 1)

    assert _migrate(monkeypatch, reg, r) == 0

    assert _versions(reg, ALS) == [1]
    assert reg.aliases(ALS) == {"champion": 1}
    assert reg.tags(ALS, 1) == tags_before


def test_registry_down_exit_nonzero(monkeypatch, reg):
    # RAT-29: MLflow unreachable -> exit 1, nothing seeded, the Redis keys are not deleted
    s = socket.socket()
    s.bind(("127.0.0.1", 0))
    port = s.getsockname()[1]
    s.close()
    dead = f"http://127.0.0.1:{port}"
    r = FakeRedis()
    _champion(r, "als-2026-10-12", {"ndcg@10": 0.100, "coverage@10": 0.5}, PROTOCOL)
    before = dict(r.store)
    live_uri = reg.tracking_uri
    monkeypatch.setenv("MLFLOW_TRACKING_URI", dead)

    t0 = time.monotonic()
    code = _migrate(monkeypatch, reg, r, tracking_uri=dead)

    assert code == 1
    assert time.monotonic() - t0 < 20
    assert r.store == before
    monkeypatch.setenv("MLFLOW_TRACKING_URI", live_uri)
    assert _versions(reg, ALS) == []


def test_seed_without_guard_metric(monkeypatch, reg):
    # RAT-51: the champion has ndcg@10 but no coverage@10: v1 is seeded as stored, and the next candidate's
    # gate.reason notes the skipped coverage@10 guard
    r = FakeRedis()
    _champion(r, "als-1", {"ndcg@10": 0.100}, PROTOCOL)
    assert _migrate(monkeypatch, reg, r) == 0
    assert _versions(reg, ALS) == [1]
    assert enc("coverage@10") not in _stored_metrics(reg, ALS, 1)

    decision = reg.evaluate(ALS, {"ndcg@10": 0.2, "coverage@10": 0.5}, protocol=PROTOCOL)

    assert decision.outcome == "promotable"
    assert "coverage@10" in decision.reason and "skipped" in decision.reason


def test_seed_without_protocol_floors_first_run(monkeypatch, reg):
    # RAT-52: no eval_protocol in the Redis meta -> v1 has no lineage.eval_protocol and the first candidate
    # is judged on floors only ("eval_protocol changed:")
    r = FakeRedis()
    _champion(r, "als-1", {"ndcg@10": 0.100, "coverage@10": 0.5}, None)
    assert _migrate(monkeypatch, reg, r) == 0
    assert "lineage.eval_protocol" not in reg.tags(ALS, 1)

    decision = reg.evaluate(ALS, {"ndcg@10": 0.0001, "coverage@10": 0.5}, protocol=PROTOCOL)

    assert decision.outcome == "promotable"
    assert decision.reason.startswith("eval_protocol changed:")
    assert reg.tags(ALS, decision.version)["gate.reason"].startswith("eval_protocol changed:")


def test_existing_versions_left_alone(monkeypatch, reg):
    # RAT-53: recsys-gbdt already has a version -> it is skipped and unchanged, ALS is still seeded, exit 0
    existing = reg.add_version(GBDT, {"ndcg@10": 0.9, "recall@10": 0.9}, protocol="gbdt-protocol-v1")
    gbdt_tags = reg.tags(GBDT, existing)
    r = FakeRedis()
    _champion(r, "als-1", {"ndcg@10": 0.1, "coverage@10": 0.5}, PROTOCOL)
    _champion(r, "gbdt-1", {"ndcg@10": 0.2, "recall@10": 0.4}, "gbdt-protocol-v1", gbdt=True)

    assert _migrate(monkeypatch, reg, r) == 0

    assert _versions(reg, GBDT) == [existing]
    assert reg.aliases(GBDT) == {"champion": existing}
    assert reg.tags(GBDT, existing) == gbdt_tags
    assert _stored_metrics(reg, GBDT, existing)[enc("ndcg@10")] == pytest.approx(0.9)
    assert _versions(reg, ALS) == [1]
    assert reg.aliases(ALS) == {"champion": 1}


def test_corrupt_meta_is_skipped_with_a_warning(monkeypatch, reg, caplog):
    # RAT-27 (robustness): a champion whose meta JSON is corrupt is skipped with a warning, the other model is
    # still seeded, exit 0, the Redis keys stay
    r = FakeRedis()
    _champion(r, "als-1", {"ndcg@10": 0.1, "coverage@10": 0.5}, PROTOCOL)
    r.set("recs:model:meta:als-1", "{not json")
    _champion(r, "gbdt-1", {"ndcg@10": 0.2, "recall@10": 0.4}, "gbdt-protocol-v1", gbdt=True)

    with caplog.at_level("WARNING"):
        assert _migrate(monkeypatch, reg, r) == 0

    assert _versions(reg, ALS) == []
    assert _versions(reg, GBDT) == [1]
    assert any(rec.levelname == "WARNING" and "recsys-als" in rec.getMessage() for rec in caplog.records)
    assert r.store["recs:model:champion"] == "als-1"


def test_unexpected_seed_error_exits_1(monkeypatch, reg):
    # RAT-29 (generic registry error): a registry error that is not a refusal makes the command exit 1 and
    # leaves the Redis keys alone
    from mlflow import MlflowClient
    from mlflow.exceptions import MlflowException

    r = FakeRedis()
    _champion(r, "als-1", {"ndcg@10": 0.1, "coverage@10": 0.5}, PROTOCOL)
    before = dict(r.store)

    def broken(self, *_a, **_k):
        raise MlflowException("internal server error while creating the run")

    monkeypatch.setattr(MlflowClient, "create_run", broken)

    assert _migrate(monkeypatch, reg, r) == 1

    assert r.store == before


def _every_seeded_string(reg, model: str) -> list[str]:
    from tests.registry_fixture import dec

    out: list[str] = []
    for v in _versions(reg, model):
        mv = reg.client.get_model_version(model, str(v))
        out += [f"{k}={x}" for k, x in mv.tags.items()]
        run = reg.client.get_run(mv.run_id)
        out += [f"{k}={x}" for k, x in run.data.tags.items()]
        out += [f"{k}={x}" for k, x in run.data.params.items()]
        out += list(run.data.metrics)
        out += [dec(k) for k in run.data.metrics]
    return out


def test_seed_does_not_send_a_non_allow_listed_metric_key(monkeypatch, reg):
    # RAT-27 / RAT-26 (privacy fix): seeding goes through the registry_client checks
    r = FakeRedis()
    _champion(r, "als-x", {"ndcg@10": 0.100, "u-0001": 0.9, "weird key!": 0.3}, PROTOCOL)

    code = _migrate(monkeypatch, reg, r)

    assert code == 0
    strings = _every_seeded_string(reg, ALS)
    assert not [s for s in strings if "u-0001" in s or "weird key" in s]
    if _versions(reg, ALS):  # seeded (sanitised) rather than skipped: the legitimate metric survives
        assert _stored_metrics(reg, ALS, 1)[enc("ndcg@10")] == pytest.approx(0.100)


@pytest.mark.parametrize("protocol", ["leave-last u-0001", "p\nrotocol", "x" * 500, "<b>p</b>"], ids=range(4))
def test_seed_does_not_send_an_eval_protocol_with_disallowed_characters(monkeypatch, reg, protocol):
    # RAT-27 / RAT-26 (privacy fix)
    r = FakeRedis()
    _champion(r, "als-x", {"ndcg@10": 0.100}, protocol)

    code = _migrate(monkeypatch, reg, r)

    assert code == 0
    for s in _every_seeded_string(reg, ALS):
        assert protocol not in s and "u-0001" not in s
