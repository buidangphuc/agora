"""Scenarios: shared publish gate for finetune-classifier, lineage, fail-closed submit.

State is prepared and read with the raw MLflow client against a local file store (conftest `reg`);
the unit under test is the shared gate with the shipped policy plus the two register sites
(`pipeline.stages.run_evaluate`, `tune.promote.promote`). No network, GPU or dataset.
"""

import ast
import json
import re
import time

import pytest
import yaml
from conftest import MODEL, PROJECT, submit_candidate
from modelregistry import GateError, RegistryUnavailable, apply, evaluate

from datascience.config.configuration import ConfigurationManager
from datascience.constants import paths
from datascience.perf import hw

HEX12 = re.compile(r"[0-9a-f]{12}")


# --- promotion through the shared gate ------------------------------------------------------

def test_pass_moves_champion(reg):
    # Scenario: Candidate passing floor and improvement becomes champion
    submit_candidate(reg, {"macro_f1": 0.80}, "aaaaaaaaaaaa")
    d = submit_candidate(reg, {"macro_f1": 0.90}, "aaaaaaaaaaaa")
    assert reg.aliases() == {"champion": 2, "previous": 1}
    assert reg.tags(2)["gate.outcome"] == "promoted"
    assert d.version == 2 and d.outcome == "promoted"


def test_first_candidate_meets_floor(reg):
    # Scenario: First candidate at or above the floor becomes the first champion
    submit_candidate(reg, {"macro_f1": 0.55})
    assert reg.aliases() == {"champion": 1}
    assert reg.tags(1)["gate.outcome"] == "promoted"


def test_boundary_at_floor_passes(reg):
    # Scenario: Exactly at the floor passes
    submit_candidate(reg, {"macro_f1": 0.5})
    assert reg.aliases() == {"champion": 1}
    assert reg.tags(1)["gate.outcome"] == "promoted"


def test_boundary_one_percent_passes(reg):
    # Scenario: One percent better passes per the library's comparison rule
    reg.add_version({"macro_f1": 0.5}, protocol="aaaaaaaaaaaa", alias="champion")
    submit_candidate(reg, {"macro_f1": 0.5 * 1.01}, "aaaaaaaaaaaa")
    assert reg.aliases()["champion"] == 2


def test_apply_refused_for_rejected_version(reg):
    # Scenario: A candidate that is not promotable never moves the alias
    reg.add_version({"macro_f1": 0.8}, alias="champion")
    d = evaluate(MODEL, reg.new_run(), {"macro_f1": 0.4}, None, {"data_version": "d", "eval_protocol": "aaaaaaaaaaaa"})
    assert d.outcome == "rejected"
    before = reg.snapshot()
    with pytest.raises(GateError):
        apply(MODEL, d.version)
    assert reg.snapshot() == before
    assert reg.aliases()["champion"] == 1


# --- rejection never raises, never moves aliases ---------------------------------------------

def test_below_floor_rejected_champion_unchanged(reg, stage_env):
    # Scenario: Below the floor is rejected and champion is unchanged (through the register site)
    reg.add_version({"macro_f1": 0.8}, alias="champion")
    stage_env.run_evaluate({"macro_f1": 0.40, "accuracy": 0.4})
    tags = reg.tags(2)
    assert tags["gate.outcome"] == "rejected"
    assert "macro_f1" in tags["gate.reason"]
    assert reg.aliases() == {"champion": 1}
    assert paths.MODEL_DIR.exists() and any(paths.MODEL_DIR.iterdir())


def test_first_candidate_below_floor_no_champion(reg, stage_env):
    # Scenario: First candidate below the floor with no champion is rejected
    stage_env.run_evaluate({"macro_f1": 0.40})
    assert reg.versions() == [1]
    assert reg.tags(1)["gate.outcome"] == "rejected"
    assert "champion" not in reg.aliases()
    assert paths.MODEL_DIR.exists() and any(paths.MODEL_DIR.iterdir())


def test_insufficient_improvement_rejected(reg):
    # Scenario: Not improving enough is rejected
    reg.add_version({"macro_f1": 0.5}, protocol="aaaaaaaaaaaa", alias="champion")
    submit_candidate(reg, {"macro_f1": 0.5 * 1.005}, "aaaaaaaaaaaa")
    assert reg.tags(2)["gate.outcome"] == "rejected"
    assert reg.aliases() == {"champion": 1}


@pytest.mark.parametrize("metrics", [{"accuracy": 0.9}, {"macro_f1": float("nan")}],
                         ids=["missing", "nan"])
def test_missing_or_nan_primary_rejected(reg, metrics):
    # Scenario: Missing or NaN macro_f1 is rejected, not raised
    reg.add_version({"macro_f1": 0.8}, alias="champion")
    d = submit_candidate(reg, metrics)
    assert d.outcome == "rejected"
    assert reg.tags(d.version)["gate.outcome"] == "rejected"
    assert "macro_f1" in reg.tags(d.version)["gate.reason"]
    assert reg.aliases() == {"champion": 1}


# --- eval protocol (D2) -----------------------------------------------------------------------

def test_eval_protocol_changed_floor_only(reg):
    # Scenario: A changed eval protocol is judged on the floor only
    reg.add_version({"macro_f1": 0.95}, protocol="aaaaaaaaaaaa", alias="champion")
    submit_candidate(reg, {"macro_f1": 0.90}, "bbbbbbbbbbbb")
    assert reg.aliases() == {"champion": 2, "previous": 1}
    tags = reg.tags(2)
    assert tags["gate.outcome"] == "promoted"
    assert tags["gate.reason"].startswith("eval_protocol changed")


def test_eval_protocol_changed_below_floor_rejected(reg):
    # Scenario: A changed eval protocol below the floor is rejected
    reg.add_version({"macro_f1": 0.95}, protocol="aaaaaaaaaaaa", alias="champion")
    submit_candidate(reg, {"macro_f1": 0.40}, "bbbbbbbbbbbb")
    tags = reg.tags(2)
    assert tags["gate.outcome"] == "rejected"
    assert "macro_f1" in tags["gate.reason"]
    assert reg.aliases() == {"champion": 1}


# --- register sites write to the registry -----------------------------------------------------

def _src_imports():
    """(file, resolved module, imported names) for every import in src/datascience."""
    root = PROJECT / "src"
    for f in (root / "datascience").rglob("*.py"):
        pkg = list(f.relative_to(root).with_suffix("").parts)
        pkg_ctx = pkg if f.name == "__init__.py" else pkg[:-1]
        for node in ast.walk(ast.parse(f.read_text())):
            if isinstance(node, ast.Import):
                for a in node.names:
                    yield f, a.name, []
            elif isinstance(node, ast.ImportFrom):
                base = pkg_ctx[: len(pkg_ctx) - (node.level - 1)] if node.level else []
                mod = ".".join(base + ([node.module] if node.module else []))
                yield f, mod, [a.name for a in node.names]


def test_register_sites_write_to_registry(reg, stage_env, tmp_artifacts, monkeypatch):
    # Scenario: Both register sites write to the registry at runtime
    from datascience.tune.promote import promote

    # site 1: run_evaluate
    run1 = stage_env.run_evaluate({"macro_f1": 0.9})
    assert reg.versions() == [1]
    assert reg.run_id(1) == run1
    assert reg.source(1).startswith(f"runs:/{run1}") or run1 in reg.source(1)

    # site 2: tune.promote.promote (variant trained by the tuning loop, run id in results.json)
    run2 = reg.new_run()
    vdir = paths.TUNE_DIR / "v1"
    (vdir / "model").mkdir(parents=True)
    (vdir / "model" / "model.safetensors").write_text("w")
    (vdir / "metrics.json").write_text(json.dumps({"macro_f1": 0.95, "accuracy": 0.9}))
    (vdir / "config.yaml").write_text(paths.PARAMS_FILE.read_text())
    paths.TUNE_RESULTS.write_text(json.dumps(
        {"experiment": "e", "runs": [{"variant": "v1", "run_id": run2, "passed": True}]}))
    promote("v1")
    assert reg.versions() == [1, 2]
    assert reg.run_id(2) == run2

    # no remnant of the removed local registry / gate in the source tree
    banned = {"datascience.registry.base", "datascience.registry.local",
              "datascience.registry.s3", "datascience.registry.stages", "datascience.gate"}
    bad = []
    for f, mod, names in _src_imports():
        full = {mod} | {f"{mod}.{n}" for n in names}
        if full & banned:
            bad.append((f.name, sorted(full & banned)))
    assert not bad, bad


# --- fail closed -----------------------------------------------------------------------------

def test_registry_unreachable_fails_closed(reg, stage_env, monkeypatch, closed_port_url):
    # Scenario: Submit against a closed port fails closed
    reg.add_version({"macro_f1": 0.8}, alias="champion")
    before = reg.snapshot()
    run_id = reg.new_run()
    monkeypatch.setenv("MLFLOW_TRACKING_URI", closed_port_url)
    t0 = time.monotonic()
    with pytest.raises(RegistryUnavailable):
        stage_env.run_evaluate({"macro_f1": 0.9}, run_id=run_id)
    assert time.monotonic() - t0 <= 15
    assert paths.MODEL_DIR.exists() and any(paths.MODEL_DIR.iterdir())
    assert reg.snapshot() == before  # read back through the file store directly


# --- lineage ---------------------------------------------------------------------------------

def test_lineage_tags_on_every_version(reg, stage_env):
    # Scenario: Lineage tags are present on promoted and rejected versions
    run_pass = stage_env.run_evaluate({"macro_f1": 0.9})
    run_fail = stage_env.run_evaluate({"macro_f1": 0.3})
    assert reg.tags(1)["gate.outcome"] == "promoted"
    assert reg.tags(2)["gate.outcome"] == "rejected"
    for v, run in ((1, run_pass), (2, run_fail)):
        tags = reg.tags(v)
        assert tags["lineage.data_version"]
        assert tags["lineage.train_hw"]
        assert HEX12.fullmatch(tags["lineage.eval_protocol"])
        assert reg.run_id(v) == run


def test_train_hw_cpu_fallback(reg, stage_env, monkeypatch):
    # Scenario: A CPU-only host records cpu
    monkeypatch.setattr(hw, "_gpus", lambda: ([], {}))
    stage_env.run_evaluate({"macro_f1": 0.9})
    assert reg.tags(1)["lineage.train_hw"] == "cpu"


def test_train_hw_gpu_records_hw_id(reg, stage_env, monkeypatch):
    # Scenario: Lineage tags are present (train_hw is the _hw_id string on a GPU host)
    gpu = {"name": "NVIDIA A100-SXM4-80GB", "compute_capability": "8.0", "vram_gb": 79}
    monkeypatch.setattr(hw, "_gpus", lambda: ([gpu], {"driver": "550"}))
    stage_env.run_evaluate({"macro_f1": 0.9})
    assert reg.tags(1)["lineage.train_hw"] == hw._hw_id([gpu])


def _cm_with(tmp_path, edit):
    params = yaml.safe_load(paths.PARAMS_FILE.read_text())
    edit(params)
    f = tmp_path / "params_variant.yaml"
    f.write_text(yaml.safe_dump(params))
    return ConfigurationManager(params_filepath=f)


def test_eval_protocol_id_stable_and_sensitive(reg, stage_env, tmp_artifacts):
    # Scenario: The eval protocol id is stable and sensitive
    from conftest import write_data_version

    def proto(version):
        return reg.tags(version)["lineage.eval_protocol"]

    same = _cm_with(tmp_artifacts, lambda p: None)
    stage_env.run_evaluate({"macro_f1": 0.9}, cm=same)
    stage_env.run_evaluate({"macro_f1": 0.9}, cm=same)
    changed_metrics = _cm_with(tmp_artifacts, lambda p: p["metrics"].update(compute=["accuracy", "macro_f1"]))
    stage_env.run_evaluate({"macro_f1": 0.9}, cm=changed_metrics)
    write_data_version(paths.DATA_VERSION, sha="fedcba654321")
    stage_env.run_evaluate({"macro_f1": 0.9}, cm=same)
    changed_dataset = _cm_with(tmp_artifacts, lambda p: p["ingestion"].update(dataset_name="other/ds"))
    stage_env.run_evaluate({"macro_f1": 0.9}, cm=changed_dataset)

    ids = [proto(v) for v in reg.versions()]
    assert len(ids) == 5 and all(HEX12.fullmatch(i) for i in ids)
    assert ids[0] == ids[1]                      # same inputs -> same id
    assert ids[2] != ids[0]                      # metrics.compute changed
    assert ids[3] != ids[0]                      # eval-set data_version changed
    assert ids[4] != ids[0]                      # dataset name changed


# --- direct library use must not hide a policy gap -------------------------------------------

def test_policy_entry_is_resolvable_by_the_library(reg):
    # Scenario: The entry has the agreed values (library resolves the shipped policy at submit)
    d = evaluate(MODEL, reg.new_run(), {"macro_f1": 0.6}, None, {"data_version": "d", "eval_protocol": "aaaaaaaaaaaa"})
    assert d.outcome == "promotable"
