"""Step definitions for Product Tag Classifier & Taxonomy Filter Enrichment BDD tests."""

from __future__ import annotations

import asyncio
import concurrent.futures
import pytest
from pytest_bdd import given, parsers, scenarios, then, when

import sys
sys.path.insert(0, "team-ai")

from app.modules.business.tag_classifier.schemas import (
    ClassifySkuHierarchyRequest,
    ClassifyTagsRequest,
    ExploreTagsRequest,
    PromoteTagRequest,
    RawListingItem,
    SkuVariantInput,
    TagStatus,
)
from app.modules.business.tag_classifier.service import TagClassifierService

scenarios("../features/ai/tag_classifier_taxonomy.feature")


def _run_sync(coro):
    try:
        loop = asyncio.get_running_loop()
    except RuntimeError:
        loop = None

    if loop and loop.is_running():
        with concurrent.futures.ThreadPoolExecutor(max_workers=1) as pool:
            return pool.submit(asyncio.run, coro).result()
    return asyncio.run(coro)


@pytest.fixture
def tag_world():
    return {
        "service": TagClassifierService(),
        "input_title": "",
        "category_id": "",
        "classify_res": None,
        "sku_req": None,
        "sku_res": None,
        "explore_res": None,
        "promote_res": None,
    }


# Scenario 1: SPU Level Tag Classification
@given(parsers.parse('a seller provides product title "{title}"'))
def seller_provides_title(tag_world, title: str):
    tag_world["input_title"] = title


@when(parsers.parse('the seller requests tag classification for category "{category_id}"'))
def request_tag_classification(tag_world, category_id: str):
    service: TagClassifierService = tag_world["service"]
    res = _run_sync(
        service.classify_tags(
            ClassifyTagsRequest(
                title=tag_world["input_title"],
                category_id=category_id,
            )
        )
    )
    tag_world["classify_res"] = res


@then(parsers.parse('the system returns canonical tags "{t1}", "{t2}", "{t3}"'))
def verify_canonical_tags(tag_world, t1: str, t2: str, t3: str):
    res = tag_world["classify_res"]
    slugs = [t.slug for t in res.canonical_tags]
    assert t1 in slugs
    assert t2 in slugs
    assert t3 in slugs


@then(parsers.parse('the tags are mapped to facet groups "{g1}", "{g2}", "{g3}"'))
def verify_facet_groups(tag_world, g1: str, g2: str, g3: str):
    res = tag_world["classify_res"]
    assert g1 in res.suggested_facet_filters
    assert g2 in res.suggested_facet_filters
    assert g3 in res.suggested_facet_filters


# Scenario 2: Granular SKU Level Hierarchical Classification
@given(parsers.parse('a parent listing "{spu_title}"'))
def parent_listing(tag_world, spu_title: str):
    tag_world["spu_title"] = spu_title


@given(parsers.parse('child SKU variants "{v1_name}" and "{v2_name}"'))
def child_sku_variants(tag_world, v1_name: str, v2_name: str):
    tag_world["variants"] = [
        SkuVariantInput(
            variant_id="sku-1",
            name=v1_name,
            price=29990000,
            stock=15,
            options={"color": "Titan Tự Nhiên", "capacity": "256GB"},
        ),
        SkuVariantInput(
            variant_id="sku-2",
            name=v2_name,
            price=34990000,
            stock=10,
            options={"color": "Xanh Navy", "capacity": "512GB"},
        ),
    ]


@when("hierarchical SKU classification is executed")
def execute_sku_hierarchy(tag_world):
    service: TagClassifierService = tag_world["service"]
    req = ClassifySkuHierarchyRequest(
        spu_title=tag_world["spu_title"],
        category_id="cat-electronics",
        variants=tag_world["variants"],
    )
    res = _run_sync(service.classify_sku_hierarchy(req))
    tag_world["sku_res"] = res


@then("each SKU inherits common tags and receives specific variant facets for color and capacity")
def verify_sku_facets(tag_world):
    res = tag_world["sku_res"]
    assert res.total_skus_processed == 2
    sku1 = res.sku_results[0]
    sku2 = res.sku_results[1]
    assert sku1.variant_facets["color"] == "titan-tu-nhien"
    assert sku1.variant_facets["capacity"] == "256gb"
    assert sku2.variant_facets["color"] == "xanh-navy"
    assert sku2.variant_facets["capacity"] == "512gb"


@then("an OpenSearch nested document payload is generated")
def verify_opensearch_nested_doc(tag_world):
    res = tag_world["sku_res"]
    doc = res.nested_opensearch_doc
    assert "variants" in doc
    assert len(doc["variants"]) == 2
    assert "spu_facets" in doc


# Scenario 3: Candidate Tag Discovery from Raw Batch
@given("a batch of unclassified listings with emergent specs")
def batch_unclassified_listings(tag_world):
    tag_world["raw_batch"] = [
        RawListingItem(
            listing_id="item-1",
            title="Sạc nhanh GaN công suất 100W Anker",
            category_id="cat-electronics",
        ),
        RawListingItem(
            listing_id="item-2",
            title="Củ sạc laptop công suất 100W Baseus",
            category_id="cat-electronics",
        ),
    ]


@when(parsers.parse("offline exploration is executed with frequency threshold {min_freq:d}"))
def execute_offline_exploration(tag_world, min_freq: int):
    service: TagClassifierService = tag_world["service"]
    req = ExploreTagsRequest(
        batch_listings=tag_world["raw_batch"],
        min_frequency=min_freq,
        min_confidence=0.80,
    )
    res = _run_sync(service.explore_tags(req))
    tag_world["explore_res"] = res


@then("candidate tags are discovered and registered with status EXPLORING")
def verify_candidate_discovery(tag_world):
    res = tag_world["explore_res"]
    discovered = [c.slug for c in res.discovered_candidates]
    assert "cong-suat-100w" in discovered


# Scenario 4: Candidate Tag Promoted to Canonical Filter Facet
@given(parsers.parse('an exploring candidate tag "{slug}"'))
def exploring_candidate_tag(tag_world, slug: str):
    tag_world["candidate_slug"] = slug


@when(parsers.parse('promotion is executed with synonyms "{syn}"'))
def execute_promotion(tag_world, syn: str):
    service: TagClassifierService = tag_world["service"]
    req = PromoteTagRequest(
        tag_slugs=[tag_world["candidate_slug"]],
        add_synonyms=[syn],
    )
    res = _run_sync(service.promote_tags(req))
    tag_world["promote_res"] = res


@then("the tag status becomes PROMOTED and is active for search filter facets")
def verify_promotion_active(tag_world):
    res = tag_world["promote_res"]
    assert res.total_promoted == 1
    promoted = res.promoted_tags[0]
    assert promoted.status == TagStatus.PROMOTED
    assert promoted.is_canonical is True
