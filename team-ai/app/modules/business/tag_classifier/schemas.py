from __future__ import annotations

from enum import Enum
from typing import Any
from pydantic import BaseModel, Field


class TagStatus(str, Enum):
    EXPLORING = "exploring"    # Discovered candidate in offline pool
    PROMOTED = "promoted"      # Promoted to canonical label / filter facet
    REJECTED = "rejected"      # Filtered out by gating or human curator
    ARCHIVED = "archived"


class FacetGroup(str, Enum):
    CONNECTIVITY = "connectivity"      # bluetooth, wifi, usb-c, lightning, 5g
    FEATURE = "feature"                # chong-nuoc, chong-on-anc, tiet-kiem-dien, chong-tia-uv
    MATERIAL = "material"              # cotton, da-that, thep-khong-gi, nhom-nguyen-khoi
    DISPLAY = "display"                # oled, amoled, 144hz, 4k-uhd
    CAPACITY = "capacity"              # 128gb, 256gb, 512gb, 1tb, 10000mah, 20000mah, 5-lit
    RAM = "ram"                        # 8gb-ram, 16gb-ram, 32gb-ram
    POWER = "power"                    # 20w, 65w, 100w, 120w, sac-nhanh-pd
    COLOR = "color"                    # titan-tu-nhien, xanh-navy, den-nham, trang-ngoc-trai, hong-pastel
    SIZE = "size"                      # size-s, size-m, size-l, size-xl, 6-7-inch, 14-inch
    STYLE = "style"                    # oversize, slimfit, vintage, toi-gian
    USAGE = "usage"                    # the-thao, van-phong, du-lich, hoc-tap, gaming
    EDITION = "edition"                # ban-tieu-chuan, combo-phu-kien, ban-pro
    GENERAL = "general"


class TagItem(BaseModel):
    tag_id: str = Field(..., description="Unique tag identifier, e.g. 'tag-bt-53'")
    name: str = Field(..., description="Display name, e.g. 'Bluetooth 5.3'")
    slug: str = Field(..., description="URL-safe canonical slug, e.g. 'bluetooth-5-3'")
    facet_group: FacetGroup = Field(default=FacetGroup.FEATURE, description="Facet classification group")
    category_id: str = Field(..., description="Target category or 'all' for cross-category")
    status: TagStatus = Field(default=TagStatus.PROMOTED, description="Lifecycle status")
    confidence: float = Field(default=1.0, ge=0.0, le=1.0, description="Model extraction confidence")
    is_canonical: bool = Field(default=True, description="Whether this tag is an official filter facet")
    synonyms: list[str] = Field(default_factory=list, description="Alternative keywords/aliases")
    occurrence_count: int = Field(default=1, ge=0, description="Number of matching listings/SKUs in catalog")
    search_volume: int = Field(default=0, ge=0, description="Monthly search query frequency")
    conversion_lift: float = Field(default=0.0, description="Historical CVR lift when tag is present")


# ──────────────────────────────────────────────────────────────────────────────
# SPU / Listing Level Classification
# ──────────────────────────────────────────────────────────────────────────────

class ClassifyTagsRequest(BaseModel):
    title: str = Field(..., min_length=2, description="Product title to classify")
    description: str = Field(default="", description="Optional full product description")
    category_id: str = Field(default="", description="Optional category hint (e.g. 'cat-electronics')")
    top_k: int = Field(default=8, ge=1, le=30, description="Max tags to return")
    include_candidates: bool = Field(default=True, description="Whether to include exploring candidates")


class ClassifyTagsResponse(BaseModel):
    canonical_tags: list[TagItem] = Field(default_factory=list, description="Official promoted tags ready for search filters")
    candidate_tags: list[TagItem] = Field(default_factory=list, description="Emergent candidate tags in exploration pool")
    suggested_facet_filters: dict[str, list[str]] = Field(
        default_factory=dict, description="Structured facet key-values for OpenSearch dynamic filtering"
    )
    category_id: str = Field(default="", description="Resolved category ID")
    execution_time_ms: float = Field(default=0.0, description="Inference latency in milliseconds")


# ──────────────────────────────────────────────────────────────────────────────
# SKU / Variant Level Granular Classification
# ──────────────────────────────────────────────────────────────────────────────

class SkuVariantInput(BaseModel):
    variant_id: str = Field(default="", description="Unique Variant/SKU ID (e.g. 'var-01')")
    name: str = Field(..., description="Variant display title, e.g. 'Xanh Titan / 512GB'")
    sku_code: str = Field(default="", description="SKU Code, e.g. 'IP15PM-BLU-512'")
    price: int = Field(default=0, ge=0, description="SKU price in VND")
    stock: int = Field(default=0, ge=0, description="Available inventory count")
    options: dict[str, str] = Field(default_factory=dict, description="Key-value options e.g. {'color': 'Xanh', 'size': 'XL'}")


class SkuClassificationResult(BaseModel):
    variant_id: str
    sku_code: str
    name: str
    price: int
    stock: int
    is_in_stock: bool
    sku_specific_tags: list[TagItem] = Field(default_factory=list, description="Tags specific only to this variant (e.g. 512GB, Xanh Titan)")
    inherited_spu_tags: list[TagItem] = Field(default_factory=list, description="Common parent tags inherited from SPU (e.g. Apple, ANC)")
    all_effective_tags: list[TagItem] = Field(default_factory=list, description="Union of inherited SPU tags and SKU-specific tags")
    variant_facets: dict[str, str] = Field(default_factory=dict, description="Exact facet key-values (e.g. {'capacity': '512gb', 'color': 'xanh-titan'})")


class ClassifySkuHierarchyRequest(BaseModel):
    spu_title: str = Field(..., min_length=2, description="Parent product title")
    spu_description: str = Field(default="", description="Parent product description")
    category_id: str = Field(default="", description="Category hint")
    variants: list[SkuVariantInput] = Field(..., min_length=1, description="List of child SKU variants")


class ClassifySkuHierarchyResponse(BaseModel):
    spu_title: str
    category_id: str
    spu_canonical_tags: list[TagItem] = Field(default_factory=list, description="Common parent listing tags")
    sku_results: list[SkuClassificationResult] = Field(default_factory=list, description="Granular per-SKU classification")
    spu_facet_filters: dict[str, list[str]] = Field(default_factory=dict, description="Aggregated SPU-level facet coverage")
    nested_opensearch_doc: dict[str, Any] = Field(default_factory=dict, description="Ready-to-index OpenSearch nested document payload")
    total_skus_processed: int
    execution_time_ms: float


# ──────────────────────────────────────────────────────────────────────────────
# Exploration & Promotion Schemas
# ──────────────────────────────────────────────────────────────────────────────

class RawListingItem(BaseModel):
    listing_id: str
    title: str
    description: str = ""
    category_id: str = "cat-general"
    attributes: dict[str, Any] = Field(default_factory=dict)
    variants: list[SkuVariantInput] = Field(default_factory=list)


class ExploreTagsRequest(BaseModel):
    batch_listings: list[RawListingItem] = Field(..., description="Listings to mine candidate tags from")
    min_confidence: float = Field(default=0.65, ge=0.0, le=1.0, description="Minimum extraction confidence threshold")
    min_frequency: int = Field(default=2, ge=1, description="Minimum occurrence frequency to register candidate")
    extract_attributes: bool = Field(default=True, description="Whether to extract structured key-values")


class ExploreTagsResponse(BaseModel):
    total_processed: int
    discovered_candidates: list[TagItem] = Field(default_factory=list)
    clusters_found: int
    new_candidate_count: int


class PromoteTagRequest(BaseModel):
    tag_slugs: list[str] = Field(..., min_length=1, description="List of candidate slugs to promote")
    target_category_id: str = Field(default="", description="Override category ID or keep original")
    target_facet_group: FacetGroup | None = Field(default=None, description="Override facet group")
    add_synonyms: list[str] = Field(default_factory=list, description="Additional synonyms to bind")


class PromoteTagResponse(BaseModel):
    promoted_tags: list[TagItem] = Field(default_factory=list)
    total_promoted: int
    active_canonical_count: int


class ListTagsRequest(BaseModel):
    category_id: str = Field(default="", description="Filter by category")
    facet_group: FacetGroup | None = Field(default=None, description="Filter by facet group")
    status: TagStatus | None = Field(default=None, description="Filter by lifecycle status")


class ListTagsResponse(BaseModel):
    tags: list[TagItem] = Field(default_factory=list)
    total: int
    canonical_count: int
    candidate_count: int
