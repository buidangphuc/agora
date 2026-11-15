"""Copilot groundedness eval set, gated by run_eval.

Interfaces these tests pin (implementer creates them):
  evals/copilot_grounding.jsonl cases use metadata.evaluator "copilot_grounded"; input has
      message, seller_id, listing_id and facts (ListingFacts fields, or null for unknown)
  scripts.run_eval._build_copilot(listing) -> object with async suggest(ChatCopilotRequest);
      the default builds the mock ChatCopilotService over the case listing port
  the checker fails a case whose drafts claim availability unless the facts are published,
      owned by input.seller_id and have stock above 0, or that out-of-stock-claim the opposite
"""

import json
from argparse import Namespace
from pathlib import Path

from app.modules.business.ai_assistant.schemas import ChatCopilotResponse
from scripts import run_eval
from tests.factories import build_test_settings

_DATASET = Path(__file__).parents[2] / "evals" / "copilot_grounding.jsonl"


def _cases() -> list[dict]:
    return [
        json.loads(line)
        for line in _DATASET.read_text(encoding="utf-8").splitlines()
        if line.strip()
    ]


async def _gate(capsys) -> tuple[int, dict]:
    code = await run_eval._run(
        Namespace(cases=str(_DATASET), min_score=0.8), settings=build_test_settings()
    )
    return code, json.loads(capsys.readouterr().out.strip().splitlines()[-1])


def test_copilot_dataset_covers_the_required_situations():
    """Scenario: Eval passes on the grounded copilot (dataset shape)"""
    cases = _cases()

    assert len(cases) >= 5
    assert len({c["id"] for c in cases}) == len(cases)
    assert {c["metadata"]["evaluator"] for c in cases} == {"copilot_grounded"}
    stocks = {(c["input"]["facts"] or {}).get("stock") for c in cases}
    assert {None, 0} <= stocks
    assert any(s and s > 0 for s in stocks)


async def test_copilot_groundedness_set_passes(capsys):
    """Scenario: Eval passes on the grounded copilot"""
    code, report = await _gate(capsys)

    assert code == 0
    assert report["cases"] == len(_cases())
    assert report["scored"] == len(_cases())
    assert report["pass_rate"] == 1.0
    assert report["failures"] == []


class _ClaimsStockCopilot:
    async def suggest(self, request) -> ChatCopilotResponse:
        return ChatCopilotResponse(quick_replies=["Dạ shop còn hàng ạ!"] * 3)


async def test_copilot_groundedness_set_fails_on_contradiction(capsys, monkeypatch):
    """Scenario: Eval fails on a stock-contradicting draft"""
    monkeypatch.setattr(
        run_eval, "_build_copilot", lambda listing: _ClaimsStockCopilot()
    )

    code, report = await _gate(capsys)

    assert code != 0
    assert report["pass_rate"] < 0.8
    assert any("copilot-stock-zero" in f for f in report["failures"])
