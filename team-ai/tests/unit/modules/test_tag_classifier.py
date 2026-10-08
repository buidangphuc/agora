from __future__ import annotations

import pytest

from app.modules.business.tag_classifier.schemas import (
    ClassifySkuHierarchyRequest,
    ClassifyTagsRequest,
    ExploreTagsRequest,
    ListTagsRequest,
    PromoteTagRequest,
    RawListingItem,
    SkuVariantInput,
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
async def test_classify_sku_hierarchy_electronics(tag_service: TagClassifierService):
    req = ClassifySkuHierarchyRequest(
        spu_title="Điện thoại iPhone 15 Pro Max Khung Titanium Chống nước IPX7",
        spu_description="Siêu phẩm camera tiềm vọng zoom 5x chip A17 Pro mạnh mẽ chuyên gaming",
        category_id="cat-electronics",
        variants=[
            SkuVariantInput(
                variant_id="var-101",
                name="Titan Tự Nhiên / 256GB",
                sku_code="IP15PM-NAT-256",
                price=29990000,
                stock=15,
                options={"color": "Titan Tự Nhiên", "capacity": "256GB"},
            ),
            SkuVariantInput(
                variant_id="var-102",
                name="Xanh Navy / 512GB",
                sku_code="IP15PM-BLU-512",
                price=34990000,
                stock=8,
                options={"color": "Xanh Navy", "capacity": "512GB"},
            ),
            SkuVariantInput(
                variant_id="var-103",
                name="Đen Nhám / 1TB",
                sku_code="IP15PM-BLK-1TB",
                price=41990000,
                stock=0,
                options={"color": "Đen Nhám", "capacity": "1TB"},
            ),
        ],
    )
    res = await tag_service.classify_sku_hierarchy(req)

    assert res.total_skus_processed == 3
    spu_slugs = [t.slug for t in res.spu_canonical_tags]
    assert "chong-nuoc-ipx7" in spu_slugs
    assert "chuyen-gaming" in spu_slugs

    # Verify SKU 1: Titan Natural / 256GB
    sku1 = res.sku_results[0]
    assert sku1.is_in_stock is True
    assert sku1.variant_facets["color"] == "titan-tu-nhien"
    assert sku1.variant_facets["capacity"] == "256gb"
    effective1_slugs = [t.slug for t in sku1.all_effective_tags]
    assert "chong-nuoc-ipx7" in effective1_slugs  # inherited from SPU
    assert "256gb" in effective1_slugs  # SKU-specific

    # Verify SKU 2: Blue Navy / 512GB
    sku2 = res.sku_results[1]
    assert sku2.variant_facets["color"] == "xanh-navy"
    assert sku2.variant_facets["capacity"] == "512gb"

    # Verify SKU 3: Black Matte / 1TB (Out of stock)
    sku3 = res.sku_results[2]
    assert sku3.is_in_stock is False
    assert sku3.variant_facets["color"] == "den-nham"
    assert sku3.variant_facets["capacity"] == "1tb"

    # Verify OpenSearch nested doc
    nested_doc = res.nested_opensearch_doc
    assert nested_doc["price_min"] == 29990000
    assert nested_doc["price_max"] == 41990000
    assert nested_doc["total_stock"] == 23
    assert len(nested_doc["variants"]) == 3
    assert "capacity" in nested_doc["spu_facets"]
    assert "256gb" in nested_doc["spu_facets"]["capacity"]
    assert "512gb" in nested_doc["spu_facets"]["capacity"]
    assert "1tb" in nested_doc["spu_facets"]["capacity"]


@pytest.mark.asyncio
async def test_offline_exploration_and_promotion_pipeline(
    tag_service: TagClassifierService,
):
    raw_listings = [
        RawListingItem(
            listing_id="list-001",
            title="Sạc dự phòng Anker 20000mAh công suất 100W siêu nhanh",
            description="Pin sạc dung lượng 20000mAh hỗ trợ PD 100W cho laptop và điện thoại",
            category_id="cat-electronics",
            variants=[
                SkuVariantInput(
                    variant_id="sku-1",
                    name="Bản 100W Màu Đen",
                    options={"power": "100W", "color": "Đen"},
                )
            ],
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
    list_cand_res = await tag_service.list_tags(
        ListTagsRequest(status=TagStatus.EXPLORING)
    )
    cand_slugs = [t.slug for t in list_cand_res.tags]
    assert "dung-luong-20000mah" in cand_slugs

    # Promote candidates
    promote_res = await tag_service.promote_tags(
        PromoteTagRequest(
            tag_slugs=["dung-luong-20000mah", "cong-suat-100w", "vai-linen-tu-nhien"],
            add_synonyms=["pin 20000mah", "sac 100w", "chat lieu linen"],
        )
    )
    assert promote_res.total_promoted == 3
