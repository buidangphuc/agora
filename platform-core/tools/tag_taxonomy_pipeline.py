#!/usr/bin/env python3
"""Agora MLOps — Product & SKU Tag Classifier, Discovery & Filter Taxonomy Enrichment Pipeline.

Demonstrates:
1. Online SPU & Granular SKU-Level Inference (<5ms for parent + N child variants).
2. Offline Exploration: Candidate tag extraction & frequency clustering over uncataloged listings.
3. Gating & Promotion: Statistical threshold evaluation & promotion to official Canonical Labels.
4. OpenSearch Nested Document payload generation for high-scale SKU-level filtering.

Usage:
    python3 platform-core/tools/tag_taxonomy_pipeline.py
"""

from __future__ import annotations

import asyncio
import json
import sys
import time
from typing import Any

# Ensure project python modules are discoverable
sys.path.insert(0, "team-ai")

from app.modules.business.tag_classifier.schemas import (
    ClassifySkuHierarchyRequest,
    ClassifyTagsRequest,
    ExploreTagsRequest,
    FacetGroup,
    ListTagsRequest,
    PromoteTagRequest,
    RawListingItem,
    SkuVariantInput,
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
]

SAMPLE_HIERARCHICAL_PRODUCT = ClassifySkuHierarchyRequest(
    spu_title="Điện thoại Apple iPhone 15 Pro Max Khung Titanium Chống nước IPX7 Chuyên Gaming",
    spu_description="Camera tiềm vọng 5x zoom quang học, sạc nhanh 20W PD, chip A17 Pro mạnh mẽ",
    category_id="cat-electronics",
    variants=[
        SkuVariantInput(
            variant_id="sku-101",
            name="Titan Tự Nhiên / 256GB",
            sku_code="IP15PM-NAT-256",
            price=29990000,
            stock=20,
            options={"color": "Titan Tự Nhiên", "capacity": "256GB"},
        ),
        SkuVariantInput(
            variant_id="sku-102",
            name="Xanh Navy / 512GB",
            sku_code="IP15PM-BLU-512",
            price=34990000,
            stock=12,
            options={"color": "Xanh Navy", "capacity": "512GB"},
        ),
        SkuVariantInput(
            variant_id="sku-103",
            name="Đen Nhám / 1TB",
            sku_code="IP15PM-BLK-1TB",
            price=41990000,
            stock=5,
            options={"color": "Đen Nhám", "capacity": "1TB"},
        ),
        SkuVariantInput(
            variant_id="sku-104",
            name="Trắng Ngọc Trai / 256GB",
            sku_code="IP15PM-WHT-256",
            price=29990000,
            stock=0,
            options={"color": "Trắng Ngọc Trai", "capacity": "256GB"},
        ),
    ],
)

SAMPLE_UNCATALOGED_BATCH = [
    RawListingItem(
        listing_id="raw-01",
        title="Pin sạc dự phòng dung lượng 20000mAh công suất 100W siêu nhanh",
        description="Pin sạc 20000mAh chuẩn PD 100W sạc được cho Macbook và laptop gaming",
        category_id="cat-electronics",
        variants=[
            SkuVariantInput(variant_id="v1", name="Bản 100W Màu Đen", options={"power": "100W", "color": "Đen"})
        ],
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
    print(" Agora MLOps — Product & Granular SKU Tag Classifier Pipeline")
    print("=" * 80)

    service = TagClassifierService()

    # -------------------------------------------------------------------------
    # Phase 1: Online SPU & Granular SKU-Level Hierarchical Classification
    # -------------------------------------------------------------------------
    print("\n[Phase 1] Online Granular SKU Hierarchy Classification (<5ms)")
    print("-" * 80)

    sku_res = await service.classify_sku_hierarchy(SAMPLE_HIERARCHICAL_PRODUCT)
    print(f"Parent SPU: {sku_res.spu_title}")
    print(f"  Category: {sku_res.category_id} | Execution Time: {sku_res.execution_time_ms}ms")
    print(f"  SPU Inherited Common Tags:")
    for t in sku_res.spu_canonical_tags:
        print(f"    - [{t.facet_group.value.upper()}] {t.name} (slug: '{t.slug}')")

    print(f"\n  Child SKU Variants ({sku_res.total_skus_processed} SKUs classified):")
    for i, sku in enumerate(sku_res.sku_results, start=1):
        stock_badge = f"{sku.stock} in stock" if sku.is_in_stock else "OUT OF STOCK"
        print(f"    SKU #{i}: {sku.name} ({sku.sku_code}) — {sku.price:,.0f} VND [{stock_badge}]")
        print(f"      * Specific Facets: {sku.variant_facets}")
        print(f"      * Effective Tags ({len(sku.all_effective_tags)}): {[t.slug for t in sku.all_effective_tags]}")

    print(f"\n  Aggregated Parent OpenSearch Filter Facets:")
    for k, v in sku_res.spu_facet_filters.items():
        print(f"    - filter.{k}: {v}")

    # -------------------------------------------------------------------------
    # Phase 2: Offline Exploration Pipeline (Candidate Discovery)
    # -------------------------------------------------------------------------
    print("\n\n[Phase 2] Offline Exploration — Mining & Clustering Raw Listings & SKUs")
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
    # Phase 4: OpenSearch Ready-to-Index Document Payload Verification
    # -------------------------------------------------------------------------
    print("\n\n[Phase 4] OpenSearch 2.11 Nested Index Document Payload")
    print("-" * 80)
    print(json.dumps(sku_res.nested_opensearch_doc, indent=2, ensure_ascii=False))

    # Summary Statistics
    list_res = await service.list_tags(ListTagsRequest())
    print("\n" + "=" * 80)
    print(f" Marketplace Taxonomy Status: {list_res.canonical_count} Canonical Facets | {list_res.candidate_count} Exploring Candidates")
    print("=" * 80 + "\n")


def main() -> None:
    asyncio.run(run_pipeline())


if __name__ == "__main__":
    main()
