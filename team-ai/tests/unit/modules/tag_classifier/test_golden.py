"""Golden replay: the new service reproduces what team-ai's TagClassifierService recorded."""

import json
from pathlib import Path
from typing import Any

import pytest
from pydantic import BaseModel

from app.modules.business.tag_classifier.schemas import (
    ClassifySkuHierarchyRequest,
    ClassifyTagsRequest,
    ExploreTagsRequest,
    ListTagsRequest,
    PromoteTagRequest,
)
from app.modules.business.tag_classifier.service import TagClassifierService

FIXTURE = Path(__file__).parents[3] / "fixtures" / "tag_classifier_golden.json"
GOLDEN: dict[str, Any] = json.loads(FIXTURE.read_text(encoding="utf-8"))
CASES: list[dict[str, Any]] = GOLDEN["cases"]

REQUESTS: dict[str, type[BaseModel]] = {
    "classify_tags": ClassifyTagsRequest,
    "classify_sku_hierarchy": ClassifySkuHierarchyRequest,
    "list_tags": ListTagsRequest,
    "explore_tags": ExploreTagsRequest,
    "promote_tags": PromoteTagRequest,
}
ASYNC_OPS = {"explore_tags", "promote_tags"}


async def _run(service: TagClassifierService, step: dict[str, Any]) -> dict[str, Any]:
    op = step["op"]
    request = REQUESTS[op](**step["request"])
    call = getattr(service, op)(request)
    response = await call if op in ASYNC_OPS else call
    dumped = response.model_dump(mode="json")
    if "execution_time_ms" in dumped:
        dumped["execution_time_ms"] = 0.0  # latency is not part of the contract
    return dumped


def test_fixture_names_its_source():
    assert GOLDEN["source"].startswith("team-ai@")


def test_fixture_covers_the_e2e_paths():
    names = {c["name"] for c in CASES}
    assert len(names) == len(CASES)
    assert {
        "classify_e2e_spu",
        "classify_category_hint_discount",
        "classify_short_synonym_false_positive",
        "classify_emergent_patterns",
        "classify_material_keywords",
        "sku_two_variants_color_capacity",
        "sku_variant_without_name",
        "sku_non_positive_price_and_stock",
        "explore_candidate_reaching_min_frequency",
        "explore_lone_wattage_not_registered",
        "promote_exploring_with_synonyms_and_facet_override",
        "promote_existing_canonical_adds_synonyms",
        "promote_unknown_slug",
        "list_by_category",
        "list_by_facet_group",
        "list_by_status_promoted",
    } <= names
    e2e = next(c for c in CASES if c["name"] == "classify_e2e_spu")
    slugs = {t["slug"] for t in e2e["steps"][0]["response"]["canonical_tags"]}
    assert slugs == {"bluetooth-5-3", "chong-on-chu-dong-anc", "sac-nhanh-65w-gan"}


@pytest.mark.parametrize("case", CASES, ids=lambda c: c["name"])
async def test_replay_matches_team_ai(case: dict[str, Any]):
    service = TagClassifierService(store=None)
    for step in case["steps"]:
        assert await _run(service, step) == step["response"], step["op"]
