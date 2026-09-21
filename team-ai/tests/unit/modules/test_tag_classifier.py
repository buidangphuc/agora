from __future__ import annotations

import pytest

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


@pytest.fixture
def tag_service() -> TagClassifierService:
    return TagClassifierService()


@pytest.mark.asyncio
async def test_classify_tags_electronics(tag_service: TagClassifierService):
    req = ClassifyTagsRequest(
        title="Tai nghe Bluetooth 5.3 TWS chống ồn chủ động ANC pin 40h sạc 65W GaN",
        description="Tai nghe không dây cao cấp chuẩn chống nước IPX7, âm bass sâu, chuyên gaming độ trễ thấp.",
        category_id="cat-electronics",
        top_k=8,
    )
    res = await tag_service.classify_tags(req)

    canonical_slugs = [t.slug for t in res.canonical_tags]
    assert "bluetooth-5-3" in canonical_slugs
    assert "chong-on-chu-dong-anc" in canonical_slugs
    assert "chong-nuoc-ipx7" in canonical_slugs
    assert "sac-nhanh-65w-gan" in canonical_slugs
    assert "chuyen-gaming" in canonical_slugs

    # Verify facet grouping for OpenSearch
    assert "connectivity" in res.suggested_facet_filters
    assert "bluetooth-5-3" in res.suggested_facet_filters["connectivity"]
    assert "feature" in res.suggested_facet_filters
    assert res.execution_time_ms >= 0


@pytest.mark.asyncio
async def test_classify_tags_fashion(tag_service: TagClassifierService):
    req = ClassifyTagsRequest(
        title="Áo thun nam 100% Cotton tự nhiên dáng rộng oversize co giãn 4 chiều",
        description="Áo phông cộc tay phong cách hàn quốc thoáng mát, chống tia UV bảo vệ da.",
        category_id="cat-fashion",
    )
    res = await tag_service.classify_tags(req)

    canonical_slugs = [t.slug for t in res.canonical_tags]
    assert "100-cotton-premium" in canonical_slugs
    assert "form-rong-oversize" in canonical_slugs
    assert "co-gian-4-chieu" in canonical_slugs
    assert "chong-tia-uv-upf50" in canonical_slugs


@pytest.mark.asyncio
async def test_offline_exploration_and_promotion_pipeline(tag_service: TagClassifierService):
    # Step 1: Batch of raw unclassified listings with emergent specs
    raw_listings = [
        RawListingItem(
            listing_id="list-001",
            title="Sạc dự phòng Anker 20000mAh công suất 100W siêu nhanh",
            description="Pin sạc dung lượng 20000mAh hỗ trợ PD 100W cho laptop và điện thoại",
            category_id="cat-electronics",
        ),
        RawListingItem(
            listing_id="list-002",
            title="Pin sạc dự phòng Baseus 20000mAh 100W vỏ nhôm",
            description="Dung lượng 20000mAh cổng type-c sạc nhanh công suất 100W",
            category_id="cat-electronics",
        ),
        RawListingItem(
            listing_id="list-003",
            title="Áo sơ mi đũi nam vải linen tự nhiên mát mẻ mùa hè",
            description="Chất liệu vải linen tự nhiên mềm mại thoáng khí form basic",
            category_id="cat-fashion",
        ),
        RawListingItem(
            listing_id="list-004",
            title="Đầm suông nữ thời trang vải linen tự nhiên cao cấp",
            description="Vải linen tự nhiên may tỉ mỉ dáng suông thanh lịch",
            category_id="cat-fashion",
        ),
    ]

    # Step 2: Run Offline Exploration
    explore_req = ExploreTagsRequest(
        batch_listings=raw_listings,
        min_frequency=2,
        min_confidence=0.80,
    )
    explore_res = await tag_service.explore_tags(explore_req)

    assert explore_res.total_processed == 4
    assert explore_res.clusters_found > 0
    discovered_slugs = [c.slug for c in explore_res.discovered_candidates]
    assert "dung-luong-20000mah" in discovered_slugs
    assert "cong-suat-100w" in discovered_slugs
    assert "vai-linen-tu-nhien" in discovered_slugs

    # Verify candidates are in exploring state
    list_cand_res = await tag_service.list_tags(ListTagsRequest(status=TagStatus.EXPLORING))
    cand_slugs = [t.slug for t in list_cand_res.tags]
    assert "dung-luong-20000mah" in cand_slugs

    # Step 3: Online classification BEFORE promotion should identify it as candidate tag
    classify_before = await tag_service.classify_tags(
        ClassifyTagsRequest(
            title="Pin dự phòng 20000mAh công suất 100W",
            category_id="cat-electronics",
            include_candidates=True,
        )
    )
    cand_matched_slugs = [t.slug for t in classify_before.candidate_tags]
    assert "dung-luong-20000mah" in cand_matched_slugs
    assert "cong-suat-100w" in cand_matched_slugs

    # Step 4: Run Promotion Gate (Promote candidates to official canonical filter facets)
    promote_res = await tag_service.promote_tags(
        PromoteTagRequest(
            tag_slugs=["dung-luong-20000mah", "cong-suat-100w", "vai-linen-tu-nhien"],
            add_synonyms=["pin 20000mah", "sac 100w", "chat lieu linen"],
        )
    )
    assert promote_res.total_promoted == 3

    # Step 5: Online classification AFTER promotion should immediately produce canonical tags
    classify_after = await tag_service.classify_tags(
        ClassifyTagsRequest(
            title="Củ sạc đa năng công suất 100W công nghệ mới",
            category_id="cat-electronics",
        )
    )
    promoted_canonical_slugs = [t.slug for t in classify_after.canonical_tags]
    assert "cong-suat-100w" in promoted_canonical_slugs
    assert "power" in classify_after.suggested_facet_filters
    assert "cong-suat-100w" in classify_after.suggested_facet_filters["power"]
