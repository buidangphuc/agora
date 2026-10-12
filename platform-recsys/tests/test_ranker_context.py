"""Session acquisition context as an evaluated ranker training feature (no Spark).

Scenarios (recommendations spec, "Session acquisition context is an evaluated ranker training feature"):
  - Context kept when it does not regress
  - Context dropped on regression
  - Legacy input without context columns

Interface assumptions (the implementer must provide them):
  - ``recsys.ranker.stage.evaluate_variant(settings, matrix, split, *, with_context: bool) -> dict`` is the
    single seam that trains on the time-based train rows and scores the holdout; it returns at least
    ``{"ndcg@10": float, "recall@10": float}``. Tests stub it so the comparison is deterministic.
  - ``stage.run_stage(...).summary`` carries ``context_features`` ("enabled"|"disabled"), ``context_reason``
    and ``context_metrics`` = {"with_context": {...}, "without_context": {...}}.
  - The published artifact ALWAYS has exactly contract.RANKING_FEATURES (enabled or disabled).
  - Context columns in the rank dataset: ``session_source``, ``session_medium``, ``session_campaign``; a
    context feature in the artifact's ``features`` list has a name containing ``session_``.
"""

from __future__ import annotations

import hashlib
import json
from pathlib import Path

import pandas as pd
import pytest

from recsys.config import Settings
from recsys.ranker import contract, stage
from recsys.registry.registry import ModelRegistry
from tests import ranker_fixture as fx

CONTEXT_COLS = ("session_source", "session_medium", "session_campaign")


def _add_context(root: Path) -> None:
    parquet = next((root / "rank").glob("as_of=*.parquet"))
    df = pd.read_parquet(parquet)
    df["session_source"] = ["google", "direct"] * (len(df) // 2) + ["google"] * (len(df) % 2)
    df["session_medium"] = "organic"
    df["session_campaign"] = "(none)"
    df.to_parquet(parquet, index=False, coerce_timestamps="ms", allow_truncated_timestamps=True)
    manifest = parquet.with_name(parquet.name.replace(".parquet", ".manifest.json"))
    m = json.loads(manifest.read_text())
    m["file_sha256"] = hashlib.sha256(parquet.read_bytes()).hexdigest()
    manifest.write_text(json.dumps(m))


def _settings(tmp_path, context=True) -> Settings:
    values = fx.write_all(tmp_path)
    if context:
        _add_context(tmp_path)
    values.update(gbdt_trees=10, gbdt_min_leaf=10, enable_gbdt=True, gbdt_ctr_min_impressions=10**6)
    return Settings(**values)


def _stub(monkeypatch, without: dict, with_ctx: dict):
    calls = []

    def fake(settings, matrix, split, *, with_context):
        calls.append(with_context)
        return dict(with_ctx if with_context else without)

    monkeypatch.setattr(stage, "evaluate_variant", fake)
    return calls


def _run(settings):
    return stage.run_stage(settings, stage.resolve_inputs(settings), "gen-1", ModelRegistry())


def _context_features(artifact) -> list[str]:
    return [f for f in artifact["features"] if "session_" in f]


def test_context_kept_without_regression(tmp_path, monkeypatch):
    """Scenario: Context kept when it does not regress (ndcg equal, recall better: both >= baseline)."""
    without = {"ndcg@10": 0.90, "recall@10": 0.60}
    with_ctx = {"ndcg@10": 0.90, "recall@10": 0.65}
    calls = _stub(monkeypatch, without, with_ctx)
    result = _run(_settings(tmp_path))
    assert set(calls) == {True, False}
    s = result.summary
    assert s["context_features"] == "enabled"
    assert s["context_metrics"]["with_context"]["ndcg@10"] == 0.90
    assert s["context_metrics"]["with_context"]["recall@10"] == 0.65
    assert s["context_metrics"]["without_context"]["ndcg@10"] == 0.90
    assert s["context_metrics"]["without_context"]["recall@10"] == 0.60
    assert result.promoted, result.reason
    # team-ai accepts only RANKING_FEATURES: context is evaluate-and-report, never published
    assert list(result.artifact["features"]) == list(contract.RANKING_FEATURES)


@pytest.mark.parametrize(
    "with_ctx",
    [
        {"ndcg@10": 0.95, "recall@10": 0.55},  # ndcg up, recall down
        {"ndcg@10": 0.85, "recall@10": 0.70},  # recall up, ndcg down
    ],
)
def test_context_dropped_on_regression(tmp_path, monkeypatch, with_ctx):
    """Scenario: Context dropped on regression (either metric below the without-context baseline)."""
    without = {"ndcg@10": 0.90, "recall@10": 0.60}
    _stub(monkeypatch, without, with_ctx)
    result = _run(_settings(tmp_path))
    s = result.summary
    assert s["context_features"] == "disabled"
    assert s["context_metrics"]["with_context"] == {**with_ctx, **s["context_metrics"]["with_context"]}
    assert s["context_metrics"]["with_context"]["recall@10"] == with_ctx["recall@10"]
    assert s["context_metrics"]["without_context"]["ndcg@10"] == 0.90
    assert result.promoted, result.reason  # the run does not fail: it ships the context-free model
    assert _context_features(result.artifact) == []
    assert list(result.artifact["features"]) == list(contract.RANKING_FEATURES)


def test_legacy_input_without_context_columns(tmp_path, monkeypatch):
    """Scenario: Legacy input without context columns: disabled, reason columns_absent, no failure."""
    calls = _stub(monkeypatch, {"ndcg@10": 0.9, "recall@10": 0.6}, {"ndcg@10": 0.99, "recall@10": 0.99})
    result = _run(_settings(tmp_path, context=False))
    s = result.summary
    assert s["context_features"] == "disabled"
    assert s["context_reason"] == "columns_absent"
    assert True not in calls  # nothing to evaluate with context
    assert result.promoted, result.reason
    assert list(result.artifact["features"]) == list(contract.RANKING_FEATURES)
