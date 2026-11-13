"""Golden replay: the new services reproduce what team-ai's AIAssistantService recorded."""

import json
from pathlib import Path
from typing import Any

import pytest
from app.modules.business.ai_assistant.factory import (
    AIAssistantServices,
    build_ai_assistant_services,
)
from app.modules.business.ai_assistant.schemas import (
    ChatCopilotRequest,
    MagicListingRequest,
    SummarizeReviewsRequest,
)

FIXTURE = Path(__file__).parents[3] / "fixtures" / "ai_assistant_golden.json"
GOLDEN: dict[str, Any] = json.loads(FIXTURE.read_text(encoding="utf-8"))
CASES: list[dict[str, Any]] = [
    c for c in GOLDEN["cases"] if c["flow"] != "shopping"
]


def _services(rag: dict[str, Any] | None) -> AIAssistantServices:
    return build_ai_assistant_services()


async def _replay(case: dict[str, Any]) -> dict[str, Any]:
    services = _services(case["rag"])
    request = case["request"]
    match case["flow"]:
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
    assert flows == {"listing", "copilot", "reviews"}
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


@pytest.mark.parametrize("case", CASES, ids=lambda c: f"{c['flow']}-{c['name']}")
async def test_replay_matches_team_ai(case: dict[str, Any]):
    assert await _replay(case) == case["response"]
