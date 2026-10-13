"""Golden replay: the new services reproduce what team-ai's AIAssistantService recorded."""

import json
from collections.abc import Sequence
from pathlib import Path
from typing import Any

import pytest
from llama_index.core.schema import NodeWithScore, TextNode

from app.modules.business.ai_assistant.factory import (
    AIAssistantServices,
    build_ai_assistant_services,
)
from app.modules.business.ai_assistant.schemas import (
    ChatCopilotRequest,
    MagicListingRequest,
    ShoppingAssistantRequest,
    SummarizeReviewsRequest,
)

FIXTURE = Path(__file__).parents[3] / "fixtures" / "ai_assistant_golden.json"
GOLDEN: dict[str, Any] = json.loads(FIXTURE.read_text(encoding="utf-8"))
CASES: list[dict[str, Any]] = GOLDEN["cases"]


class RecordedSearch:
    def __init__(self, spec: dict[str, Any]) -> None:
        self._spec = spec

    async def search(
        self, query: str, *, top_k: int | None = None
    ) -> Sequence[NodeWithScore]:
        if self._spec["raises"]:
            raise RuntimeError("redis down")
        return [
            NodeWithScore(
                node=TextNode(
                    id_=f"{n['listing_id']}:chunk:{i}",
                    text="x",
                    metadata=n["metadata"],
                ),
                score=n["score"],
            )
            for i, n in enumerate(self._spec["nodes"])
        ]


def _services(rag: dict[str, Any] | None) -> AIAssistantServices:
    if rag is None:
        return build_ai_assistant_services(None, rag_min_score=0.0)
    return build_ai_assistant_services(
        RecordedSearch(rag), rag_min_score=rag["min_score"]
    )


async def _replay(case: dict[str, Any]) -> dict[str, Any]:
    services = _services(case["rag"])
    request = case["request"]
    match case["flow"]:
        case "shopping":
            shopping = await services.shopping.answer(
                ShoppingAssistantRequest(**request)
            )
            return shopping.model_dump()
        case "listing":
            return services.listing.generate(
                MagicListingRequest(**request)
            ).model_dump()
        case "copilot":
            return services.copilot.suggest(ChatCopilotRequest(**request)).model_dump()
        case "reviews":
            return services.reviews.summarize(
                SummarizeReviewsRequest(**request)
            ).model_dump()
        case flow:
            raise AssertionError(flow)


def test_fixture_names_its_source():
    assert GOLDEN["source"].startswith("team-ai@")


def test_fixture_covers_every_flow_and_branch():
    names = {(c["flow"], c["name"]) for c in CASES}
    assert len(names) == len(CASES)
    flows = {c["flow"] for c in CASES}
    assert flows == {"shopping", "listing", "copilot", "reviews"}
    rag = [c["rag"] for c in CASES if c["flow"] == "shopping" and c["rag"]]
    assert any(r["raises"] for r in rag)
    assert any(not r["raises"] and not r["nodes"] for r in rag)
    assert any(r["min_score"] > 0 for r in rag)
    sentiments = {c["response"]["sentiment"] for c in CASES if c["flow"] == "reviews"}
    assert {"unknown", "positive", "negative", "mixed", "neutral"} <= sentiments
    categories = {
        c["response"]["suggested_category_id"] for c in CASES if c["flow"] == "listing"
    }
    assert {
        "cat-fashion",
        "cat-electronics",
        "cat-home-living",
        "cat-beauty",
        "cat-sports",
        "cat-general",
    } <= categories
    assert (
        len(
            {
                tuple(c["response"]["quick_replies"])
                for c in CASES
                if c["flow"] == "copilot"
            }
        )
        == 6
    )
    assert (
        len(
            {
                tuple(c["response"]["suggested_followups"])
                for c in CASES
                if c["flow"] == "shopping" and c["rag"] is None
            }
        )
        == 4
    )


@pytest.mark.parametrize("case", CASES, ids=lambda c: f"{c['flow']}-{c['name']}")
async def test_replay_matches_team_ai(case: dict[str, Any]):
    assert await _replay(case) == case["response"]
