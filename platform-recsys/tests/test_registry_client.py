"""recsys/registry_client.py: the only module that talks to py-modelregistry (RAT-19/25/26/50/58).

INTERFACE ASSUMPTIONS (the implementer may not change these without telling the test author):
  ALS_MODEL / GBDT_MODEL / TWO_TOWER_MODEL      "recsys-als" / "recsys-gbdt" / "recsys-two-tower"
  PayloadRejected(ValueError)                   raised on any payload that is not allow-listed
  preflight() -> None                           raises modelregistry.RegistryUnavailable when unreachable
  start_run(run_name, params) -> run_id         creates the MLflow run, logs allow-listed params
  evaluate(model, run_id, metrics, artifact_dir, lineage) -> modelregistry.Decision
  build_als_artifact(als, out_dir) -> Path      als.item_factors.toPandas() -> [listing_id, features];
                                                writes <out_dir>/item_factors.parquet and nothing else
  write_drift_sketches(run_id, sketches)        logs JSON artifact `drift_sketches.json` on the run
  champion_drift_baseline(model=ALS_MODEL) -> (version:int, sketches:dict) | None
Allow-listed params used here: als_rank, als_reg_param, als_alpha, als_max_iter, top_n.
"""

from __future__ import annotations

import json
import time

import pandas as pd
import pytest
from modelregistry import RegistryUnavailable

from recsys import registry_client as rc

LINEAGE = {"data_version": "als_interactions@v1", "eval_protocol": "p1"}
HYPER = {"als_rank": 8, "als_reg_param": 0.05, "als_alpha": 40.0, "als_max_iter": 5, "top_n": 50}


class _Frame:
    def __init__(self, df: pd.DataFrame | None, boom: bool = False) -> None:
        self._df, self._boom = df, boom

    def toPandas(self) -> pd.DataFrame:  # noqa: N802 (Spark API name)
        assert not self._boom, "user factors must never be read"
        return self._df  # type: ignore[return-value]


class _Als:
    def __init__(self, n_users: int = 50, n_items: int = 20) -> None:
        self.item_factors = _Frame(
            pd.DataFrame(
                {
                    "listing_id": [f"l-{i:03d}" for i in range(n_items)],
                    "features": [[0.1 * i, 0.2, 0.3] for i in range(n_items)],
                }
            )
        )
        self.user_factors = _Frame(
            pd.DataFrame(
                {
                    "user_key": [f"u-{i:04d}" for i in range(1, n_users + 1)],
                    "features": [[0.5, 0.6, 0.7]] * n_users,
                }
            ),
            boom=True,
        )
        self.rank = 3


def _all_files_text(root) -> str:
    return "".join(p.read_bytes().decode("latin-1") for p in root.rglob("*") if p.is_file())


def test_model_names():
    assert (rc.ALS_MODEL, rc.GBDT_MODEL, rc.TWO_TOWER_MODEL) == (
        "recsys-als",
        "recsys-gbdt",
        "recsys-two-tower",
    )


# --- RAT-19: reachability pre-flight --------------------------------------------------------------
def test_preflight_passes_on_reachable_registry(reg):
    # RAT-19
    rc.preflight()


def test_preflight_unreachable_registry_raises_fast(monkeypatch, closed_port_url_fx):
    # RAT-19: fail closed, well inside the 20 s budget
    monkeypatch.setenv("MLFLOW_TRACKING_URI", closed_port_url_fx)
    t0 = time.monotonic()
    with pytest.raises(RegistryUnavailable):
        rc.preflight()
    assert time.monotonic() - t0 < 20


@pytest.fixture
def closed_port_url_fx() -> str:
    import socket

    s = socket.socket()
    s.bind(("127.0.0.1", 0))
    port = s.getsockname()[1]
    s.close()
    return f"http://127.0.0.1:{port}"


# --- RAT-25 / RAT-26: nothing personal leaves the job ---------------------------------------------
def test_user_keyed_payload_refused(tmp_path, monkeypatch, closed_port_url_fx):
    # RAT-26: the adapter raises BEFORE any MLflow call. The tracking URI is a dead port, so a call
    # would surface as RegistryUnavailable instead of PayloadRejected.
    monkeypatch.setenv("MLFLOW_TRACKING_URI", closed_port_url_fx)
    with pytest.raises(rc.PayloadRejected):
        rc.start_run("recsys-als-g1", {**HYPER, "u-0001": "0.3"})  # param keyed by user id
    with pytest.raises(rc.PayloadRejected):
        rc.start_run("recsys-als-g1", {**HYPER, "top_users": {"u-0001": [1, 2]}})  # per-user value
    with pytest.raises(rc.PayloadRejected):
        rc.start_run("recsys-als-g1", {**HYPER, "user_id": "u-0001"})  # not allow-listed

    bad_dir = tmp_path / "als-save"
    (bad_dir / "userFactors").mkdir(parents=True)  # a Spark ALSModel.save layout
    (bad_dir / "userFactors" / "part-0.parquet").write_bytes(b"x")
    with pytest.raises(rc.PayloadRejected):
        rc.evaluate(rc.ALS_MODEL, "run-id", {"ndcg@10": 0.1}, bad_dir, LINEAGE)

    user_file = tmp_path / "users"
    user_file.mkdir()
    (user_file / "user_factors.parquet").write_bytes(b"x")
    with pytest.raises(rc.PayloadRejected):
        rc.evaluate(rc.ALS_MODEL, "run-id", {"ndcg@10": 0.1}, user_file, LINEAGE)


def test_non_allow_listed_artifact_file_refused(tmp_path, monkeypatch, closed_port_url_fx):
    # RAT-25: only item-side artifacts may be uploaded
    monkeypatch.setenv("MLFLOW_TRACKING_URI", closed_port_url_fx)
    d = tmp_path / "art"
    d.mkdir()
    (d / "interactions.parquet").write_bytes(b"x")
    with pytest.raises(rc.PayloadRejected):
        rc.evaluate(rc.ALS_MODEL, "run-id", {"ndcg@10": 0.1}, d, LINEAGE)


def test_metric_allow_list(tmp_path, monkeypatch, closed_port_url_fx):
    # RAT-25: only numeric aggregate metrics
    monkeypatch.setenv("MLFLOW_TRACKING_URI", closed_port_url_fx)
    d = tmp_path / "art"
    d.mkdir()
    (d / "item_factors.parquet").write_bytes(b"x")
    with pytest.raises(rc.PayloadRejected):
        rc.evaluate(rc.ALS_MODEL, "run-id", {"ndcg_u-0001": 0.1}, d, LINEAGE)
    with pytest.raises(rc.PayloadRejected):
        rc.evaluate(rc.ALS_MODEL, "run-id", {"ndcg@10": "0.1"}, d, LINEAGE)  # type: ignore[dict-item]


def test_run_name_with_pii_shaped_text_refused(monkeypatch, closed_port_url_fx):
    # RAT-25 (run name is allow-listed too)
    monkeypatch.setenv("MLFLOW_TRACKING_URI", closed_port_url_fx)
    with pytest.raises(rc.PayloadRejected):
        rc.start_run("alice@example.com", HYPER)


def test_allow_listed_run_reaches_registry_without_user_data(reg, tmp_path):
    # RAT-25: the happy path logs exactly the allow-listed params and item-side artifacts
    run_id = rc.start_run("recsys-als-g1", HYPER)
    assert reg.client.get_run(run_id).data.params["als_rank"] == "8"
    out = rc.build_als_artifact(_Als(), tmp_path / "als")
    d = rc.evaluate(rc.ALS_MODEL, run_id, {"ndcg@10": 0.1, "coverage@10": 0.5}, out, LINEAGE)
    assert d.outcome == "promotable"
    files = [f.path for f in reg.client.list_artifacts(run_id, "model")]
    assert files == ["model/item_factors.parquet"]
    run = reg.client.get_run(run_id)
    blob = json.dumps([run.data.params, run.data.tags, run.info.run_name])
    assert "u-0" not in blob


# --- RAT-50: ALS artifact = item factors only -----------------------------------------------------
def test_als_artifact_is_item_factors_only(tmp_path):
    # RAT-50: 50 users, 20 items
    out = rc.build_als_artifact(_Als(n_users=50, n_items=20), tmp_path / "als")
    files = sorted(p.name for p in out.rglob("*") if p.is_file())
    assert files == ["item_factors.parquet"]
    assert not any(p.is_dir() for p in out.rglob("*"))
    df = pd.read_parquet(out / "item_factors.parquet")
    assert len(df) == 20
    assert list(df.columns) == ["listing_id", "features"]
    text = _all_files_text(out)
    assert "u-0001" not in text and "u-0050" not in text and "user_key" not in text


# --- RAT-58: drift sketches round trip through the champion run -----------------------------------
def test_drift_sketches_artifact_round_trip(reg, tmp_path):
    # RAT-58: the first run's version is champion; the next run reads its sketches as baseline
    sketches = {
        k: [float(i) for i in range(101)] for k in ("weight", "user_items", "item_users", "top_score")
    }
    run_id = rc.start_run("recsys-als-g1", HYPER)
    rc.write_drift_sketches(run_id, sketches)
    assert [f.path for f in reg.client.list_artifacts(run_id)] == ["drift_sketches.json"]
    d = rc.evaluate(
        rc.ALS_MODEL,
        run_id,
        {"ndcg@10": 0.1, "coverage@10": 0.5},
        rc.build_als_artifact(_Als(), tmp_path / "als"),
        LINEAGE,
    )
    from modelregistry import apply

    apply(rc.ALS_MODEL, d.version)
    version, baseline = rc.champion_drift_baseline()
    assert version == d.version
    assert baseline == sketches


# --- review follow-ups from group A ---------------------------------------------------------------
SKETCH = [float(i) for i in range(101)]


@pytest.mark.parametrize(
    "bad",
    [
        {"u-0001": SKETCH},  # keyed by a user id
        {"weight": {"u-0001": 0.3}},  # a per-user mapping
        {"weight": ["u-0001", "u-0002"]},  # non-numeric values
        {"weight": [1.0, "2.0"]},
        {"weight": [1.0, float("nan")]},  # not finite: not JSON either
        {"weight": 3.0},  # a scalar, not a sketch
        {"weight": [[1.0], [2.0]]},  # nested
        {"top_users": SKETCH},  # not one of the four sketched features
    ],
)
def test_drift_sketches_non_numeric_or_user_keyed_refused(monkeypatch, closed_port_url_fx, bad):
    # RAT-26 / RAT-25 (follow-up): refused BEFORE any MLflow call: the URI is a dead port, so a call would
    # surface as RegistryUnavailable instead of PayloadRejected
    monkeypatch.setenv("MLFLOW_TRACKING_URI", closed_port_url_fx)
    with pytest.raises(rc.PayloadRejected):
        rc.write_drift_sketches("run-id", bad)


def test_numeric_drift_sketches_are_accepted(reg):
    run_id = rc.start_run("recsys-als-g1", HYPER)
    rc.write_drift_sketches(run_id, {"weight": SKETCH, "top_score": SKETCH})
    assert [f.path for f in reg.client.list_artifacts(run_id)] == ["drift_sketches.json"]


@pytest.mark.parametrize(
    "bad",
    [
        {"data_version": {"u-0001": 1}},  # a mapping keyed by a user id
        {"data_version": ["a", "b"]},  # a list
        {"data_version": ("a",)},  # a tuple
        {"data_version": {"nested": {"u-0001": 1}}},
    ],
)
def test_non_scalar_lineage_refused(tmp_path, monkeypatch, closed_port_url_fx, bad):
    # follow-up: lineage becomes MLflow tags, so only scalars may cross the adapter
    monkeypatch.setenv("MLFLOW_TRACKING_URI", closed_port_url_fx)
    d = tmp_path / "art"
    d.mkdir()
    (d / "item_factors.parquet").write_bytes(b"x")
    with pytest.raises(rc.PayloadRejected):
        rc.evaluate(rc.ALS_MODEL, "run-id", {"ndcg@10": 0.1}, d, {**LINEAGE, **bad})


def test_artifact_dir_containing_a_symlink_refused(tmp_path, monkeypatch, closed_port_url_fx):
    # follow-up: a symlink named like an allowed file would upload whatever it points at (e.g. user data)
    monkeypatch.setenv("MLFLOW_TRACKING_URI", closed_port_url_fx)
    secret = tmp_path / "users.txt"
    secret.write_text("u-0001 u-0002")
    d = tmp_path / "art"
    d.mkdir()
    (d / "item_factors.parquet").symlink_to(secret)
    with pytest.raises(rc.PayloadRejected):
        rc.evaluate(rc.ALS_MODEL, "run-id", {"ndcg@10": 0.1}, d, LINEAGE)

    other = tmp_path / "art2"
    other.mkdir()
    (other / "item_factors.parquet").write_bytes(b"x")
    (other / "linked").symlink_to(tmp_path, target_is_directory=True)  # a symlinked directory
    with pytest.raises(rc.PayloadRejected):
        rc.evaluate(rc.ALS_MODEL, "run-id", {"ndcg@10": 0.1}, other, LINEAGE)


def test_baseline_none_without_champion_or_artifact(reg):
    # RAT-58 / RAT-59: no champion -> None; seeded champion without artifact -> None
    assert rc.champion_drift_baseline() is None
    reg.add_version(rc.ALS_MODEL, {"ndcg@10": 0.1, "coverage@10": 0.5})
    assert rc.champion_drift_baseline() is None


# --- privacy fix: gate.reason is a bounded, user-key-free string -------------------------------------
FIXED_REASONS = [
    "publish failed: ConnectionError",
    "gbdt stage failed: KeyError",
    "two-tower stage failed: ValueError",
    "below fixed-weight baseline: holdout ndcg@10 0.12 < baseline 0.13 x 1.01",
]
BAD_REASONS = [
    "gbdt stage failed: KeyError: u-0001",  # a user key
    "x" * 201,  # too long
    "gbdt stage failed: KeyError('u-0001')",  # user key and characters outside the charset
    "publish failed\nu-0002",  # control character
    "publish failed: <script>",  # outside the bounded charset
    "publish failed: \u00e9\u4e2d",  # non-ASCII
]


@pytest.mark.parametrize("bad", BAD_REASONS, ids=range(len(BAD_REASONS)))
def test_mark_rejected_refuses_unbounded_or_user_keyed_reason(monkeypatch, closed_port_url_fx, bad):
    # RAT-26 (privacy fix): refused BEFORE any MLflow call (dead port would give RegistryUnavailable)
    monkeypatch.setenv("MLFLOW_TRACKING_URI", closed_port_url_fx)
    with pytest.raises(rc.PayloadRejected):
        rc.mark_rejected(rc.GBDT_MODEL, 1, bad)


@pytest.mark.parametrize("bad", BAD_REASONS, ids=range(len(BAD_REASONS)))
def test_register_rejected_refuses_unbounded_or_user_keyed_reason(monkeypatch, closed_port_url_fx, bad):
    # RAT-26 (privacy fix)
    monkeypatch.setenv("MLFLOW_TRACKING_URI", closed_port_url_fx)
    with pytest.raises(rc.PayloadRejected):
        rc.register_rejected(rc.ALS_MODEL, "run-id", bad, LINEAGE)


def test_a_reason_of_exactly_200_chars_is_not_refused_as_too_long(monkeypatch, closed_port_url_fx):
    # RAT-26 boundary: 200 passes validation, so the dead port is reached (RegistryUnavailable)
    monkeypatch.setenv("MLFLOW_TRACKING_URI", closed_port_url_fx)
    with pytest.raises(RegistryUnavailable):
        rc.mark_rejected(rc.GBDT_MODEL, 1, "a" * 200)


@pytest.mark.parametrize("ok", FIXED_REASONS)
def test_pipeline_fixed_reasons_are_accepted(monkeypatch, closed_port_url_fx, ok):
    # RAT-26: validation passes; the call reaches MLflow and fails on the dead port, not on the payload
    monkeypatch.setenv("MLFLOW_TRACKING_URI", closed_port_url_fx)
    with pytest.raises(RegistryUnavailable):
        rc.mark_rejected(rc.GBDT_MODEL, 1, ok)
    with pytest.raises(RegistryUnavailable):
        rc.register_rejected(rc.ALS_MODEL, "run-id", ok, LINEAGE)
