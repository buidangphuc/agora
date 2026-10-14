"""The Redis model registry is gone (RAT-30), and no stand-in for the registry policy remains in the job.

The static checks read source text only. The run test needs PySpark and a JVM (skipped without one).
"""

from __future__ import annotations

import ast
import importlib
from pathlib import Path

import pytest

from recsys.pipeline import run
from tests.dataset_fixture import write_dataset
from tests.fakes import FakeQdrantClient, FakeRedis
from tests.pipeline_fixture import als_rows, requires_spark, settings

PKG = Path(__file__).resolve().parents[1] / "recsys"


def test_registry_package_is_deleted():
    # RAT-30 / task 1.10: recsys/registry/ holds no module any more and cannot be imported
    assert not list((PKG / "registry").rglob("*.py"))
    for name in ("recsys.registry.registry", "recsys.registry.metadata"):
        with pytest.raises(ImportError):
            importlib.import_module(name)


def test_no_source_names_the_redis_registry_keys():
    # RAT-30: no code path reads or writes recs:model:champion, recs:model:champion:gbdt or recs:model:meta:*
    offenders = [p.relative_to(PKG).as_posix() for p in PKG.rglob("*.py") if "recs:model" in p.read_text()]
    assert not offenders, offenders


def test_nothing_imports_the_old_registry():
    offenders = []
    for p in PKG.rglob("*.py"):
        for node in ast.walk(ast.parse(p.read_text())):
            if isinstance(node, ast.ImportFrom):
                modules = [node.module or ""]
                symbols = [a.name for a in node.names]
            elif isinstance(node, ast.Import):
                modules = [a.name for a in node.names]
                symbols = []
            else:
                continue
            old = {"registry", "registry.registry", "registry.metadata"}
            if any(m in old or m.startswith("recsys.registry") for m in modules):
                offenders.append(f"{p.name}:{node.lineno}")
            if {"ModelRegistry", "ModelMetadata"} & set(symbols):
                offenders.append(f"{p.name}:{node.lineno} (symbol)")
    assert not offenders, offenders


@pytest.mark.parametrize("module", ["pipeline.py", "ranker/stage.py"])
def test_no_standin_policy_constants_remain(module):
    # group-A follow-up: the policy numbers live in policies.yaml; the stage/pipeline keep no copy of them
    tree = ast.parse((PKG / module).read_text())
    assigned = []
    for node in tree.body:
        targets = (
            node.targets
            if isinstance(node, ast.Assign)
            else [node.target] if isinstance(node, ast.AnnAssign) else []
        )
        assigned += [t.id for t in targets if isinstance(t, ast.Name)]
    stale = [n for n in assigned if n == "_PRIMARY_METRIC" or n.startswith("_MIN_")]
    assert not stale, f"{module}: {stale}"


@requires_spark
def test_no_model_keys_written(tmp_path, reg):
    # RAT-30: a full in-memory run writes no key beginning recs:model:
    write_dataset(tmp_path, als_rows())
    r, q = FakeRedis(), FakeQdrantClient()

    summary = run(settings(tmp_path, "g1", reg), redis_client=r, qdrant_client=q)

    assert summary["decision"] == "promoted" and r.store
    assert not [k for k in r.store if k.startswith("recs:model:")]
