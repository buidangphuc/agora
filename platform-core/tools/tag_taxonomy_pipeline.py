#!/usr/bin/env python3
"""Agora MLOps — Product Tag Classifier, Discovery & Filter Taxonomy Enrichment Pipeline.

Demonstrates the 2-stage lifecycle:
1. Online Inference: Fast tag classification (<10ms) for seller listing forms & OpenSearch facets.
2. Offline Exploration: Candidate tag extraction & frequency clustering over uncataloged listings.
3. Gating & Promotion: Statistical threshold evaluation & promotion to official Canonical Labels.

Usage:
    python3 platform-core/tools/tag_taxonomy_pipeline.py [--benchmark] [--explore] [--promote]
"""

from __future__ import annotations

import argparse
import asyncio
import json
import sys
import time
from typing import Any

# Ensure project python modules are discoverable
sys.path.insert(0, "team-ai")

from app.modules.business.tag_classifier.schemas import (
    ClassifyTagsRequest,
    ExploreTagsRequest,
    FacetGroup,
    ListTagsRequest,
    PromoteTagRequest,
    RawListingItem,
    TagStatus,
)
from app.modules.business.tag_classifier.service import TagClassifierService


SAMPLE_SELLER_INPUTS = [
    {
        "title": "Tai nghe Bluetooth 5.3 chụp tai chống ồn chủ động ANC pin 50h sạc nhanh 65W GaN",
        "description": "Tai nghe không dây chống nước chuẩn IPX7, đệm tai êm ái, chuyên gaming độ trễ cực thấp.",
        "category_id": "cat-electronics",
    },
    {
        "title": "Áo polo nam 100% Cotton cao cấp form rộng oversize co giãn 4 chiều",
        "description": "Chất liệu cotton tự nhiên thoáng mát, công nghệ chống tia UV bảo vệ da khi đi nắng.",
        "category_id": "cat-fashion",
    },
    {
        "title": "Nồi chiên không dầu Rapid Air công nghệ Inverter tiết kiệm điện lòng nồi Inox 304",
        "description": "Dung tích 6L nướng gà nguyên con không dầu mỡ, thép không gỉ 304 bền bỉ dễ vệ sinh.",
        "category_id": "cat-appliances",
    },
]

SAMPLE_UNCATALOGED_BATCH = [
    RawListingItem(
        listing_id="raw-01",
        title="Pin sạc dự phòng dung lượng 20000mAh công suất 100W siêu nhanh",
        description="Pin sạc 20000mAh chuẩn PD 100W sạc được cho Macbook và laptop gaming",
        category_id="cat-electronics",
    ),
    RawListingItem(
        listing_id="raw-02",
        title="Củ sạc đa cổng GaN 100W 3 cổng Type-C nhỏ gọn",
        description="Hỗ trợ công suất 100W sạc đồng thời 3 thiết bị an toàn",
        category_id="cat-electronics",
    ),
    RawListingItem(
        listing_id="raw-03",
        title="Áo sơ mi đũi nam cộc tay vải linen tự nhiên vintage",
        description="Chất liệu vải linen tự nhiên nhẹ mát thấm hút mồ hôi tốt",
        category_id="cat-fashion",
    ),
    RawListingItem(
        listing_id="raw-04",
        title="Váy maxi đi biển cao cấp vải linen tự nhiên mềm mại",
        description="May từ vải linen tự nhiên form dáng xòe trẻ trung",
        category_id="cat-fashion",
    ),
    RawListingItem(
        listing_id="raw-05",
        title="Chảo chống dính gốm sứ ceramic đáy từ cao cấp",
        description="Lớp phủ gốm sứ ceramic an toàn cho sức khỏe không chứa PFOA",
        category_id="cat-appliances",
    ),
    RawListingItem(
        listing_id="raw-06",
        title="Bộ nồi tráng men gốm sứ ceramic dùng cho mọi loại bếp",
        description="Chất liệu gốm sứ ceramic giữ nhiệt lâu và kháng khuẩn",
        category_id="cat-appliances",
    ),
]


async def run_pipeline() -> None:
    print("=" * 80)
    print(" Agora MLOps — Product Tag Classifier & Filter Taxonomy Pipeline")
    print("=" * 80)

    service = TagClassifierService()

    # -------------------------------------------------------------------------
    # Phase 1: Online Tag Classification (Fast Inference)
    # -------------------------------------------------------------------------
    print("\n[Phase 1] Online Fast Tag Classification for Seller Postings")
    print("-" * 80)

    for i, sample in enumerate(SAMPLE_SELLER_INPUTS, start=1):
        req = ClassifyTagsRequest(
            title=sample["title"],
            description=sample["description"],
            category_id=sample["category_id"],
            top_k=6,
        )
        res = await service.classify_tags(req)

        print(f"\nProduct #{i}: {sample['title']}")
        print(f"  Category: {res.category_id} | Inference Latency: {res.execution_time_ms}ms")
        print("  Predicted Canonical Tags:")
        for tag in res.canonical_tags:
            print(f"    - [{tag.facet_group.value.upper()}] {tag.name} (slug: '{tag.slug}', conf: {tag.confidence:.2f})")
        print("  OpenSearch Filter Facet Mapping:")
        for group, slugs in res.suggested_facet_filters.items():
            print(f"    - filter.{group}: {slugs}")

    # -------------------------------------------------------------------------
    # Phase 2: Offline Exploration Pipeline (Candidate Tag Discovery)
    # -------------------------------------------------------------------------
    print("\n\n[Phase 2] Offline Exploration — Mining & Clustering Raw Listings")
    print("-" * 80)
    print(f"Ingesting batch of {len(SAMPLE_UNCATALOGED_BATCH)} unclassified listings into exploration pool...")

    explore_req = ExploreTagsRequest(
        batch_listings=SAMPLE_UNCATALOGED_BATCH,
        min_frequency=2,
        min_confidence=0.80,
    )
    explore_res = await service.explore_tags(explore_req)

    print(f"Extraction complete! Discovered {len(explore_res.discovered_candidates)} candidate clusters:")
    for cand in explore_res.discovered_candidates:
        print(
            f"  * Candidate: '{cand.name}' (slug: '{cand.slug}') "
            f"| Group: {cand.facet_group.value} | Occurrences: {cand.occurrence_count} "
            f"| Confidence: {cand.confidence:.2f} | Status: {cand.status.value.upper()}"
        )

    # -------------------------------------------------------------------------
    # Phase 3: Gating & Promotion Pipeline (Candidate -> Canonical Label)
    # -------------------------------------------------------------------------
    print("\n\n[Phase 3] Promotion Gating — Elevating High-Confidence Candidates")
    print("-" * 80)

    candidates_to_promote = ["cong-suat-100w", "dung-luong-20000mah", "vai-linen-tu-nhien", "gom-su-ceramic"]
    print(f"Evaluating promotion criteria (Frequency >= 2, Confidence >= 0.80) for: {candidates_to_promote}")

    promote_req = PromoteTagRequest(
        tag_slugs=candidates_to_promote,
        add_synonyms=["sac 100w", "pin 20000mah", "chat lieu linen", "men ceramic"],
    )
    promote_res = await service.promote_tags(promote_req)

    print(f"Successfully promoted {promote_res.total_promoted} tags to Canonical Filter Facets!")
    for tag in promote_res.promoted_tags:
        print(f"  [PROMOTED] {tag.name} -> Canonical Slug: '{tag.slug}' (Facet: {tag.facet_group.value})")

    # -------------------------------------------------------------------------
    # Phase 4: Verification & Re-Classification with Enriched Taxonomy
    # -------------------------------------------------------------------------
    print("\n\n[Phase 4] Post-Promotion Classification & Dynamic Filter Enrichment")
    print("-" * 80)

    test_listing = {
        "title": "Củ sạc đa năng Baseus công suất 100W sạc nhanh laptop pin dự phòng 20000mAh",
        "description": "Cổng Type-C GaN 100W công suất lớn sạc nhanh tiện lợi đi du lịch",
        "category_id": "cat-electronics",
    }
    req = ClassifyTagsRequest(
        title=test_listing["title"],
        description=test_listing["description"],
        category_id=test_listing["category_id"],
    )
    res = await service.classify_tags(req)

    print(f"New Listing: {test_listing['title']}")
    print(f"  Inference Latency: {res.execution_time_ms}ms")
    print("  Matched Canonical Tags (including newly promoted):")
    for tag in res.canonical_tags:
        print(f"    - [{tag.facet_group.value.upper()}] {tag.name} (slug: '{tag.slug}')")
    print("  Updated OpenSearch Dynamic Facets:")
    for group, slugs in res.suggested_facet_filters.items():
        print(f"    - filter.{group}: {slugs}")

    # Summary Statistics
    list_res = await service.list_tags(ListTagsRequest())
    print("\n" + "=" * 80)
    print(f" Marketplace Taxonomy Status: {list_res.canonical_count} Canonical Facets | {list_res.candidate_count} Exploring Candidates")
    print("=" * 80 + "\n")


def main() -> None:
    asyncio.run(run_pipeline())


if __name__ == "__main__":
    main()
