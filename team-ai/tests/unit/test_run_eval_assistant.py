"""Groundedness eval set for the shopping assistant, gated by run_eval."""

import json
from argparse import Namespace
from pathlib import Path

from scripts import run_eval
from tests.factories import build_test_settings

_ROOT = Path(__file__).parents[2]
_DATASET = _ROOT / "evals" / "assistant_grounding.jsonl"


def _cases() -> list[dict]:
    return [
        json.loads(line)
        for line in _DATASET.read_text(encoding="utf-8").splitlines()
        if line.strip()
    ]


def test_grounding_dataset_has_varied_cases():
    """Scenario: Groundedness eval set passes."""
    cases = _cases()

    assert len(cases) >= 6
    assert len({c["id"] for c in cases}) == len(cases)
    assert all(c["input"].get("message") for c in cases)


async def test_run_eval_gate_passes_on_the_grounding_set(capsys):
    """Scenario: Groundedness eval set passes."""
    code = await run_eval._run(
        Namespace(cases=str(_DATASET), min_score=0.8), settings=build_test_settings()
    )
    report = json.loads(capsys.readouterr().out.strip().splitlines()[-1])

    assert code == 0
    assert report["cases"] == len(_cases())
    assert report["scored"] >= len(_cases())
    assert report["pass_rate"] >= 0.8
    assert report["failures"] == []
