from __future__ import annotations

import re
import time
import unicodedata
from collections import Counter
from typing import Any

from loguru import logger

from app.modules.business.tag_classifier.schemas import (
    ClassifySkuHierarchyRequest,
    ClassifySkuHierarchyResponse,
    ClassifyTagsRequest,
    ClassifyTagsResponse,
    ExploreTagsRequest,
    ExploreTagsResponse,
    FacetGroup,
    ListTagsRequest,
    ListTagsResponse,
    PromoteTagRequest,
    PromoteTagResponse,
    RawListingItem,
    SkuClassificationResult,
    SkuVariantInput,
    TagItem,
    TagStatus,
)


def _strip_accents(s: str) -> str:
    """Normalize Vietnamese accents and lowercase for robust fuzzy matching."""
    s = unicodedata.normalize("NFD", s)
    s = "".join(c for c in s if unicodedata.category(c) != "Mn")
    return s.replace("đ", "d").replace("Đ", "d").lower().strip()


def _slugify(s: str) -> str:
    cleaned = _strip_accents(s)
    cleaned = re.sub(r"[^a-z0-9]+", "-", cleaned)
    return cleaned.strip("-")


# Default Seed Canonical Taxonomy for Agora Marketplace (SPU & SKU Levels)
SEED_CANONICAL_TAGS: list[dict[str, Any]] = [
    # ── Electronics SPU Features ──
    {
        "tag_id": "tag-bt-53",
        "name": "Bluetooth 5.3",
        "slug": "bluetooth-5-3",
        "facet_group": FacetGroup.CONNECTIVITY,
        "category_id": "cat-electronics",
        "status": TagStatus.PROMOTED,
        "confidence": 1.0,
        "is_canonical": True,
        "synonyms": ["bluetooth 5.3", "bt 5.3", "bt5.3", "bluetooth v5.3"],
        "occurrence_count": 84,
        "search_volume": 1250,
        "conversion_lift": 0.14,
    },
    {
        "tag_id": "tag-anc",
        "name": "Chống ồn chủ động (ANC)",
        "slug": "chong-on-chu-dong-anc",
        "facet_group": FacetGroup.FEATURE,
        "category_id": "cat-electronics",
        "status": TagStatus.PROMOTED,
        "confidence": 1.0,
        "is_canonical": True,
        "synonyms": ["anc", "chong on", "active noise cancelling", "khu tieng on"],
        "occurrence_count": 62,
        "search_volume": 2100,
        "conversion_lift": 0.18,
    },
    {
        "tag_id": "tag-waterproof-ipx7",
        "name": "Chống nước IPX7",
        "slug": "chong-nuoc-ipx7",
        "facet_group": FacetGroup.FEATURE,
        "category_id": "cat-electronics",
        "status": TagStatus.PROMOTED,
        "confidence": 1.0,
        "is_canonical": True,
        "synonyms": ["ipx7", "ipx-7", "chong nuoc ipx7", "khang nuoc ipx7"],
        "occurrence_count": 45,
        "search_volume": 890,
        "conversion_lift": 0.12,
    },
    {
        "tag_id": "tag-fastcharge-65w",
        "name": "Sạc nhanh 65W GaN",
        "slug": "sac-nhanh-65w-gan",
        "facet_group": FacetGroup.POWER,
        "category_id": "cat-electronics",
        "status": TagStatus.PROMOTED,
        "confidence": 1.0,
        "is_canonical": True,
        "synonyms": ["65w", "gan 65w", "sac 65w", "power delivery 65w", "pd 65w"],
        "occurrence_count": 51,
        "search_volume": 1640,
        "conversion_lift": 0.15,
    },
    {
        "tag_id": "tag-display-144hz",
        "name": "Tần số quét 144Hz",
        "slug": "tan-so-quet-144hz",
        "facet_group": FacetGroup.DISPLAY,
        "category_id": "cat-electronics",
        "status": TagStatus.PROMOTED,
        "confidence": 1.0,
        "is_canonical": True,
        "synonyms": ["144hz", "144 hz", "man hinh 144hz", "tan so quet 144"],
        "occurrence_count": 39,
        "search_volume": 1820,
        "conversion_lift": 0.22,
    },
    {
        "tag_id": "tag-gaming",
        "name": "Chuyên Gaming",
        "slug": "chuyen-gaming",
        "facet_group": FacetGroup.USAGE,
        "category_id": "cat-electronics",
        "status": TagStatus.PROMOTED,
        "confidence": 0.98,
        "is_canonical": True,
        "synonyms": ["gaming", "choi game", "game thu", "esports", "esport"],
        "occurrence_count": 78,
        "search_volume": 3400,
        "conversion_lift": 0.19,
    },
    # ── SKU Level: Storage & Memory Variants ──
    {
        "tag_id": "tag-cap-128gb",
        "name": "Bộ nhớ 128GB",
        "slug": "128gb",
        "facet_group": FacetGroup.CAPACITY,
        "category_id": "cat-electronics",
        "status": TagStatus.PROMOTED,
        "confidence": 1.0,
        "is_canonical": True,
        "synonyms": ["128gb", "128g", "128 gb", "rom 128gb"],
        "occurrence_count": 210,
        "search_volume": 4200,
        "conversion_lift": 0.15,
    },
    {
        "tag_id": "tag-cap-256gb",
        "name": "Bộ nhớ 256GB",
        "slug": "256gb",
        "facet_group": FacetGroup.CAPACITY,
        "category_id": "cat-electronics",
        "status": TagStatus.PROMOTED,
        "confidence": 1.0,
        "is_canonical": True,
        "synonyms": ["256gb", "256g", "256 gb", "rom 256gb"],
        "occurrence_count": 340,
        "search_volume": 6800,
        "conversion_lift": 0.24,
    },
    {
        "tag_id": "tag-cap-512gb",
        "name": "Bộ nhớ 512GB",
        "slug": "512gb",
        "facet_group": FacetGroup.CAPACITY,
        "category_id": "cat-electronics",
        "status": TagStatus.PROMOTED,
        "confidence": 1.0,
        "is_canonical": True,
        "synonyms": ["512gb", "512g", "512 gb", "rom 512gb"],
        "occurrence_count": 180,
        "search_volume": 3900,
        "conversion_lift": 0.20,
    },
    {
        "tag_id": "tag-cap-1tb",
        "name": "Bộ nhớ 1TB",
        "slug": "1tb",
        "facet_group": FacetGroup.CAPACITY,
        "category_id": "cat-electronics",
        "status": TagStatus.PROMOTED,
        "confidence": 1.0,
        "is_canonical": True,
        "synonyms": ["1tb", "1 tb", "1024gb", "rom 1tb"],
        "occurrence_count": 95,
        "search_volume": 2100,
        "conversion_lift": 0.18,
    },
    # ── SKU Level: Colors ──
    {
        "tag_id": "tag-col-titan-natural",
        "name": "Màu Titan Tự Nhiên",
        "slug": "titan-tu-nhien",
        "facet_group": FacetGroup.COLOR,
        "category_id": "cat-electronics",
        "status": TagStatus.PROMOTED,
        "confidence": 1.0,
        "is_canonical": True,
        "synonyms": ["titan tu nhien", "natural titanium", "mau titan", "titan xam"],
        "occurrence_count": 120,
        "search_volume": 5600,
        "conversion_lift": 0.28,
    },
    {
        "tag_id": "tag-col-navy",
        "name": "Màu Xanh Navy",
        "slug": "xanh-navy",
        "facet_group": FacetGroup.COLOR,
        "category_id": "all",
        "status": TagStatus.PROMOTED,
        "confidence": 1.0,
        "is_canonical": True,
        "synonyms": ["xanh navy", "navy", "xanh dam", "xanh bien dam", "blue navy"],
        "occurrence_count": 150,
        "search_volume": 3100,
        "conversion_lift": 0.14,
    },
    {
        "tag_id": "tag-col-black-matte",
        "name": "Màu Đen Nhám",
        "slug": "den-nham",
        "facet_group": FacetGroup.COLOR,
        "category_id": "all",
        "status": TagStatus.PROMOTED,
        "confidence": 1.0,
        "is_canonical": True,
        "synonyms": ["den nham", "matte black", "den", "mau den", "black"],
        "occurrence_count": 280,
        "search_volume": 7200,
        "conversion_lift": 0.19,
    },
    {
        "tag_id": "tag-col-white-pearl",
        "name": "Màu Trắng Ngọc Trai",
        "slug": "trang-ngoc-trai",
        "facet_group": FacetGroup.COLOR,
        "category_id": "all",
        "status": TagStatus.PROMOTED,
        "confidence": 1.0,
        "is_canonical": True,
        "synonyms": ["trang ngoc trai", "trang", "pearl white", "mau trang", "white"],
        "occurrence_count": 190,
        "search_volume": 4400,
        "conversion_lift": 0.16,
    },
    # ── SKU Level: Clothing Sizes ──
    {
        "tag_id": "tag-size-s",
        "name": "Size S",
        "slug": "size-s",
        "facet_group": FacetGroup.SIZE,
        "category_id": "cat-fashion",
        "status": TagStatus.PROMOTED,
        "confidence": 1.0,
        "is_canonical": True,
        "synonyms": ["size s", "s", "co s"],
        "occurrence_count": 220,
        "search_volume": 2500,
        "conversion_lift": 0.12,
    },
    {
        "tag_id": "tag-size-m",
        "name": "Size M",
        "slug": "size-m",
        "facet_group": FacetGroup.SIZE,
        "category_id": "cat-fashion",
        "status": TagStatus.PROMOTED,
        "confidence": 1.0,
        "is_canonical": True,
        "synonyms": ["size m", "m", "co m"],
        "occurrence_count": 310,
        "search_volume": 4600,
        "conversion_lift": 0.20,
    },
    {
        "tag_id": "tag-size-l",
        "name": "Size L",
        "slug": "size-l",
        "facet_group": FacetGroup.SIZE,
        "category_id": "cat-fashion",
        "status": TagStatus.PROMOTED,
        "confidence": 1.0,
        "is_canonical": True,
        "synonyms": ["size l", "l", "co l"],
        "occurrence_count": 350,
        "search_volume": 5100,
        "conversion_lift": 0.22,
    },
    {
        "tag_id": "tag-size-xl",
        "name": "Size XL",
        "slug": "size-xl",
        "facet_group": FacetGroup.SIZE,
        "category_id": "cat-fashion",
        "status": TagStatus.PROMOTED,
        "confidence": 1.0,
        "is_canonical": True,
        "synonyms": ["size xl", "xl", "co xl"],
        "occurrence_count": 280,
        "search_volume": 3800,
        "conversion_lift": 0.18,
    },
    # ── Fashion SPU Features & Materials ──
    {
        "tag_id": "tag-cotton-100",
        "name": "100% Cotton Premium",
        "slug": "100-cotton-premium",
        "facet_group": FacetGroup.MATERIAL,
        "category_id": "cat-fashion",
        "status": TagStatus.PROMOTED,
        "confidence": 1.0,
        "is_canonical": True,
        "synonyms": ["100% cotton", "cotton 100", "cotton 100%", "pure cotton", "cotton tu nhien"],
        "occurrence_count": 112,
        "search_volume": 2900,
        "conversion_lift": 0.16,
    },
    {
        "tag_id": "tag-oversize",
        "name": "Form Rộng Oversize",
        "slug": "form-rong-oversize",
        "facet_group": FacetGroup.STYLE,
        "category_id": "cat-fashion",
        "status": TagStatus.PROMOTED,
        "confidence": 0.95,
        "is_canonical": True,
        "synonyms": ["oversize", "form rong", "dang rong", "loose fit", "form thung"],
        "occurrence_count": 98,
        "search_volume": 4100,
        "conversion_lift": 0.21,
    },
    {
        "tag_id": "tag-anti-uv",
        "name": "Chống tia UV UPF50+",
        "slug": "chong-tia-uv-upf50",
        "facet_group": FacetGroup.FEATURE,
        "category_id": "cat-fashion",
        "status": TagStatus.PROMOTED,
        "confidence": 1.0,
        "is_canonical": True,
        "synonyms": ["upf50+", "chong uv", "chong tia uv", "tia uv", "chong nang", "chong tia cuc tim", "upf 50"],
        "occurrence_count": 42,
        "search_volume": 1500,
        "conversion_lift": 0.17,
    },
    {
        "tag_id": "tag-co-gian-4-chieu",
        "name": "Co giãn 4 chiều",
        "slug": "co-gian-4-chieu",
        "facet_group": FacetGroup.FEATURE,
        "category_id": "cat-fashion",
        "status": TagStatus.PROMOTED,
        "confidence": 0.95,
        "is_canonical": True,
        "synonyms": ["co gian 4 chieu", "4-way stretch", "co gian tot", "dan hoi cao"],
        "occurrence_count": 65,
        "search_volume": 1200,
        "conversion_lift": 0.11,
    },
    # ── Home & Appliances SPU Features ──
    {
        "tag_id": "tag-inverter-saving",
        "name": "Công nghệ Inverter Tiết Kiệm Điện",
        "slug": "cong-nghe-inverter-tiet-kiem-dien",
        "facet_group": FacetGroup.FEATURE,
        "category_id": "cat-appliances",
        "status": TagStatus.PROMOTED,
        "confidence": 1.0,
        "is_canonical": True,
        "synonyms": ["inverter", "tiet kiem dien", "bien tan inverter", "ecobubble inverter"],
        "occurrence_count": 87,
        "search_volume": 3200,
        "conversion_lift": 0.25,
    },
    {
        "tag_id": "tag-stainless-steel-304",
        "name": "Inox 304 Cao Cấp",
        "slug": "inox-304-cao-cap",
        "facet_group": FacetGroup.MATERIAL,
        "category_id": "cat-appliances",
        "status": TagStatus.PROMOTED,
        "confidence": 1.0,
        "is_canonical": True,
        "synonyms": ["inox 304", "thep khong gi 304", "sus 304", "stainless steel 304"],
        "occurrence_count": 55,
        "search_volume": 1100,
        "conversion_lift": 0.13,
    },
    {
        "tag_id": "tag-air-fryer-oilfree",
        "name": "Không dầu Rapid Air",
        "slug": "khong-dau-rapid-air",
        "facet_group": FacetGroup.FEATURE,
        "category_id": "cat-appliances",
        "status": TagStatus.PROMOTED,
        "confidence": 0.96,
        "is_canonical": True,
        "synonyms": ["rapid air", "khong dau", "it dau mo", "giam 80% dau"],
        "occurrence_count": 48,
        "search_volume": 2800,
        "conversion_lift": 0.20,
    },
]


class TagClassifierService:
    """Service providing:

    1. Fast online tag classification for SPU & child SKU variants (`classify_tags`, `classify_sku_hierarchy`).
    2. Offline candidate tag exploration & extraction over batch listings (`explore_tags`).
    3. Gating and promotion pipeline to elevate candidate tags to canonical filter facets (`promote_tags`).
    """

    def __init__(self) -> None:
        self._canonical_tags: dict[str, TagItem] = {}
        self._candidate_tags: dict[str, TagItem] = {}
        self._synonym_index: dict[str, str] = {}  # normalized_synonym -> tag_slug

        # Bootstrap seed canonical taxonomy
        for item in SEED_CANONICAL_TAGS:
            tag = TagItem(**item)
            self._register_canonical_tag(tag)

    def _register_canonical_tag(self, tag: TagItem) -> None:
        self._canonical_tags[tag.slug] = tag
        self._synonym_index[_strip_accents(tag.name)] = tag.slug
        self._synonym_index[_strip_accents(tag.slug.replace("-", " "))] = tag.slug
        for syn in tag.synonyms:
            self._synonym_index[_strip_accents(syn)] = tag.slug

    def _register_candidate_tag(self, tag: TagItem) -> None:
        if tag.slug in self._canonical_tags:
            return  # Already canonical
        if tag.slug in self._candidate_tags:
            existing = self._candidate_tags[tag.slug]
            existing.occurrence_count += tag.occurrence_count
            existing.confidence = max(existing.confidence, tag.confidence)
        else:
            self._candidate_tags[tag.slug] = tag

    # ──────────────────────────────────────────────────────────────────────────
    # 1. SPU Level Online Classification
    # ──────────────────────────────────────────────────────────────────────────
    async def classify_tags(self, request: ClassifyTagsRequest) -> ClassifyTagsResponse:
        """Classifies product text (title + description) and returns Top-K

        promoted canonical tags and discovered candidate tags.
        """
        start_time = time.perf_counter()
        combined_text = f"{request.title} {request.description}"
        norm_text = _strip_accents(combined_text)

        matched_canonical: list[tuple[TagItem, float]] = []
        matched_candidates: list[tuple[TagItem, float]] = []

        seen_canonical_slugs: set[str] = set()
        for syn_norm, slug in self._synonym_index.items():
            if slug in seen_canonical_slugs:
                continue
            pattern = r"\b" + re.escape(syn_norm) + r"\b"
            if re.search(pattern, norm_text):
                tag = self._canonical_tags[slug]
                cat_boost = 1.0
                if request.category_id and tag.category_id not in ("all", request.category_id):
                    cat_boost = 0.6
                score = tag.confidence * cat_boost
                matched_canonical.append((tag, score))
                seen_canonical_slugs.add(slug)

        if request.include_candidates:
            seen_cand_slugs: set[str] = set()
            for slug, cand in self._candidate_tags.items():
                if slug in seen_canonical_slugs or slug in seen_cand_slugs:
                    continue
                cand_name_norm = _strip_accents(cand.name)
                pattern = r"\b" + re.escape(cand_name_norm) + r"\b"
                if re.search(pattern, norm_text):
                    score = cand.confidence * 0.85
                    matched_candidates.append((cand, score))
                    seen_cand_slugs.add(slug)

        emergent_candidates = self._extract_emergent_patterns(norm_text, request.category_id)
        for cand in emergent_candidates:
            if cand.slug not in seen_canonical_slugs and cand.slug not in [c.slug for c, _ in matched_candidates]:
                self._register_candidate_tag(cand)
                if request.include_candidates:
                    matched_candidates.append((cand, cand.confidence * 0.8))

        matched_canonical.sort(key=lambda x: x[1], reverse=True)
        matched_candidates.sort(key=lambda x: x[1], reverse=True)

        final_canonical = [item[0] for item in matched_canonical[: request.top_k]]
        final_candidates = [item[0] for item in matched_candidates[: request.top_k]]

        facet_filters: dict[str, list[str]] = {}
        for tag in final_canonical:
            group_key = tag.facet_group.value
            if group_key not in facet_filters:
                facet_filters[group_key] = []
            facet_filters[group_key].append(tag.slug)

        latency_ms = (time.perf_counter() - start_time) * 1000.0

        return ClassifyTagsResponse(
            canonical_tags=final_canonical,
            candidate_tags=final_candidates,
            suggested_facet_filters=facet_filters,
            category_id=request.category_id or "cat-general",
            execution_time_ms=round(latency_ms, 2),
        )

    # ──────────────────────────────────────────────────────────────────────────
    # 2. SKU-Level Hierarchical Classification (SPU -> Granular SKU)
    # ──────────────────────────────────────────────────────────────────────────
    async def classify_sku_hierarchy(
        self, request: ClassifySkuHierarchyRequest
    ) -> ClassifySkuHierarchyResponse:
        """Scales classification down to each child SKU variant.

        - Extracts SPU common tags once.
        - For each variant, extracts SKU-specific facets (color, size, storage, ram, power, edition).
        - Computes union effective tags (SPU + SKU) and builds OpenSearch nested document structure.
        """
        start_time = time.perf_counter()

        # Step 1: Classify Parent SPU Common Tags
        spu_res = await self.classify_tags(
            ClassifyTagsRequest(
                title=request.spu_title,
                description=request.spu_description,
                category_id=request.category_id,
                top_k=10,
                include_candidates=False,
            )
        )
        spu_tags = spu_res.canonical_tags
        spu_facet_map: dict[str, set[str]] = {
            k: set(v) for k, v in spu_res.suggested_facet_filters.items()
        }

        # Step 2: Granular SKU Level Classification
        sku_results: list[SkuClassificationResult] = []
        opensearch_variants_payload: list[dict[str, Any]] = []

        for v in request.variants:
            # Combine variant title, options dict, and SKU code
            variant_options_text = " ".join(f"{k} {val}" for k, val in v.options.items())
            variant_raw_text = f"{v.name} {variant_options_text} {v.sku_code}"
            norm_v_text = _strip_accents(variant_raw_text)

            # Match variant-specific tags
            sku_specific_tags: list[TagItem] = []
            variant_facets: dict[str, str] = {}
            seen_sku_slugs: set[str] = set()

            for syn_norm, slug in self._synonym_index.items():
                if slug in seen_sku_slugs:
                    continue
                pattern = r"\b" + re.escape(syn_norm) + r"\b"
                if re.search(pattern, norm_v_text):
                    tag = self._canonical_tags[slug]
                    # Check if this tag is a variant-specific dimension
                    if tag.facet_group in (
                        FacetGroup.COLOR,
                        FacetGroup.SIZE,
                        FacetGroup.CAPACITY,
                        FacetGroup.RAM,
                        FacetGroup.POWER,
                        FacetGroup.MATERIAL,
                        FacetGroup.EDITION,
                    ):
                        sku_specific_tags.append(tag)
                        variant_facets[tag.facet_group.value] = tag.slug
                        seen_sku_slugs.add(slug)

                        # Accumulate into SPU facet coverage
                        fg = tag.facet_group.value
                        if fg not in spu_facet_map:
                            spu_facet_map[fg] = set()
                        spu_facet_map[fg].add(tag.slug)

            # Build all effective tags (SPU union SKU)
            all_effective = list({t.slug: t for t in (spu_tags + sku_specific_tags)}.values())

            sku_res = SkuClassificationResult(
                variant_id=v.variant_id or f"var-{len(sku_results)+1}",
                sku_code=v.sku_code,
                name=v.name,
                price=v.price,
                stock=v.stock,
                is_in_stock=(v.stock > 0),
                sku_specific_tags=sku_specific_tags,
                inherited_spu_tags=spu_tags,
                all_effective_tags=all_effective,
                variant_facets=variant_facets,
            )
            sku_results.append(sku_res)

            # Prepare OpenSearch nested document representation
            opensearch_variants_payload.append(
                {
                    "variant_id": sku_res.variant_id,
                    "sku_code": sku_res.sku_code,
                    "name": sku_res.name,
                    "price": sku_res.price,
                    "stock": sku_res.stock,
                    "is_in_stock": sku_res.is_in_stock,
                    "facets": variant_facets,
                    "tags": [t.slug for t in sku_res.all_effective_tags],
                }
            )

        # Finalize SPU Aggregated Facet Filters
        final_spu_facets = {k: sorted(list(v)) for k, v in spu_facet_map.items()}

        # OpenSearch Full Document Shape
        prices = [v.price for v in request.variants if v.price > 0]
        total_stock = sum(v.stock for v in request.variants)
        nested_doc = {
            "title": request.spu_title,
            "category_id": request.category_id,
            "price_min": min(prices) if prices else 0,
            "price_max": max(prices) if prices else 0,
            "total_stock": total_stock,
            "is_in_stock": (total_stock > 0),
            "spu_tags": [t.slug for t in spu_tags],
            "spu_facets": final_spu_facets,
            "variants": opensearch_variants_payload,
        }

        latency_ms = (time.perf_counter() - start_time) * 1000.0

        return ClassifySkuHierarchyResponse(
            spu_title=request.spu_title,
            category_id=request.category_id or "cat-general",
            spu_canonical_tags=spu_tags,
            sku_results=sku_results,
            spu_facet_filters=final_spu_facets,
            nested_opensearch_doc=nested_doc,
            total_skus_processed=len(request.variants),
            execution_time_ms=round(latency_ms, 2),
        )

    # ──────────────────────────────────────────────────────────────────────────
    # 3. Emergent Pattern Extraction (SPU & SKU)
    # ──────────────────────────────────────────────────────────────────────────
    def _extract_emergent_patterns(self, norm_text: str, category_id: str) -> list[TagItem]:
        candidates: list[TagItem] = []

        # Patterns: Wattage (e.g. 100w, 120w, 240w)
        for match in re.finditer(r"\b(\d{2,3})\s*w\b", norm_text):
            watt = match.group(1)
            name = f"Công suất {watt}W"
            slug = f"cong-suat-{watt}w"
            candidates.append(
                TagItem(
                    tag_id=f"cand-{slug}",
                    name=name,
                    slug=slug,
                    facet_group=FacetGroup.POWER,
                    category_id=category_id or "cat-electronics",
                    status=TagStatus.EXPLORING,
                    confidence=0.88,
                    is_canonical=False,
                    occurrence_count=1,
                )
            )

        # Patterns: Battery capacity (e.g. 10000mah, 20000mah, 5000mah)
        for match in re.finditer(r"\b(\d{4,5})\s*mah\b", norm_text):
            cap = match.group(1)
            name = f"Dung lượng {cap}mAh"
            slug = f"dung-luong-{cap}mah"
            candidates.append(
                TagItem(
                    tag_id=f"cand-{slug}",
                    name=name,
                    slug=slug,
                    facet_group=FacetGroup.CAPACITY,
                    category_id=category_id or "cat-electronics",
                    status=TagStatus.EXPLORING,
                    confidence=0.90,
                    is_canonical=False,
                    occurrence_count=1,
                )
            )

        # Patterns: Storage (e.g. 128gb, 256gb, 512gb, 1tb)
        for match in re.finditer(r"\b(\d{1,3})\s*(gb|tb)\b", norm_text):
            size, unit = match.group(1), match.group(2).upper()
            name = f"Bộ nhớ {size}{unit}"
            slug = f"bo-nho-{size.lower()}{unit.lower()}"
            candidates.append(
                TagItem(
                    tag_id=f"cand-{slug}",
                    name=name,
                    slug=slug,
                    facet_group=FacetGroup.CAPACITY,
                    category_id=category_id or "cat-electronics",
                    status=TagStatus.EXPLORING,
                    confidence=0.92,
                    is_canonical=False,
                    occurrence_count=1,
                )
            )

        # Patterns: RAM Memory (e.g. 8gb ram, 16gb ram, 32gb ram)
        for match in re.finditer(r"\b(\d{1,2})\s*gb\s*ram\b", norm_text):
            size = match.group(1)
            name = f"RAM {size}GB"
            slug = f"ram-{size}gb"
            candidates.append(
                TagItem(
                    tag_id=f"cand-{slug}",
                    name=name,
                    slug=slug,
                    facet_group=FacetGroup.RAM,
                    category_id=category_id or "cat-electronics",
                    status=TagStatus.EXPLORING,
                    confidence=0.94,
                    is_canonical=False,
                    occurrence_count=1,
                )
            )

        # Patterns: Fabric / Materials / Finishes
        material_keywords = [
            ("linen", "Vải Linen Tự Nhiên", "vai-linen-tu-nhien", FacetGroup.MATERIAL, "cat-fashion"),
            ("satin", "Lụa Satin Cao Cấp", "lua-satin-cao-cap", FacetGroup.MATERIAL, "cat-fashion"),
            ("denim", "Chất liệu Denim Bền", "chat-lieu-denim-ben", FacetGroup.MATERIAL, "cat-fashion"),
            ("titan", "Khung Titanium", "khung-titanium", FacetGroup.MATERIAL, "cat-electronics"),
            ("ceramic", "Gốm Sứ Ceramic", "gom-su-ceramic", FacetGroup.MATERIAL, "cat-home"),
        ]
        for kw, name, slug, fg, cat in material_keywords:
            if re.search(r"\b" + kw + r"\b", norm_text):
                candidates.append(
                    TagItem(
                        tag_id=f"cand-{slug}",
                        name=name,
                        slug=slug,
                        facet_group=fg,
                        category_id=category_id or cat,
                        status=TagStatus.EXPLORING,
                        confidence=0.85,
                        is_canonical=False,
                        occurrence_count=1,
                    )
                )

        return candidates

    # ──────────────────────────────────────────────────────────────────────────
    # 4. Offline Exploration Pipeline (SPU & SKU Batches)
    # ──────────────────────────────────────────────────────────────────────────
    async def explore_tags(self, request: ExploreTagsRequest) -> ExploreTagsResponse:
        """Processes raw batch listings and child SKU variants, discovering

        emergent specs, clustering them into candidate pools.
        """
        initial_candidate_count = len(self._candidate_tags)
        extracted_slugs: Counter[str] = Counter()
        candidate_map: dict[str, TagItem] = {}

        for item in request.batch_listings:
            # Mine from parent listing
            combined = f"{item.title} {item.description}"
            norm = _strip_accents(combined)
            discovered = self._extract_emergent_patterns(norm, item.category_id)
            for tag in discovered:
                extracted_slugs[tag.slug] += 1
                if tag.slug not in candidate_map:
                    candidate_map[tag.slug] = tag
                else:
                    candidate_map[tag.slug].occurrence_count += 1

            # Mine from child SKU variants
            for var in item.variants:
                var_text = f"{var.name} {' '.join(var.options.values())} {var.sku_code}"
                var_norm = _strip_accents(var_text)
                var_discovered = self._extract_emergent_patterns(var_norm, item.category_id)
                for tag in var_discovered:
                    extracted_slugs[tag.slug] += 1
                    if tag.slug not in candidate_map:
                        candidate_map[tag.slug] = tag
                    else:
                        candidate_map[tag.slug].occurrence_count += 1

        for slug, freq in extracted_slugs.items():
            if freq >= request.min_frequency:
                tag = candidate_map[slug]
                if tag.confidence >= request.min_confidence:
                    self._register_candidate_tag(tag)

        new_candidates = len(self._candidate_tags) - initial_candidate_count
        discovered_list = [
            self._candidate_tags[slug]
            for slug in extracted_slugs.keys()
            if slug in self._candidate_tags
        ]

        logger.info(
            f"Explore tags completed: processed {len(request.batch_listings)} listings, "
            f"found {len(discovered_list)} candidates (new: {new_candidates})"
        )

        return ExploreTagsResponse(
            total_processed=len(request.batch_listings),
            discovered_candidates=discovered_list,
            clusters_found=len(extracted_slugs),
            new_candidate_count=new_candidates,
        )

    # ──────────────────────────────────────────────────────────────────────────
    # 5. Promotion Gating & Registry
    # ──────────────────────────────────────────────────────────────────────────
    async def promote_tags(self, request: PromoteTagRequest) -> PromoteTagResponse:
        """Promotes candidate tag(s) to official canonical filter facets."""
        promoted_list: list[TagItem] = []

        for slug in request.tag_slugs:
            if slug in self._candidate_tags:
                tag = self._candidate_tags.pop(slug)
                tag.status = TagStatus.PROMOTED
                tag.is_canonical = True
                tag.confidence = 1.0
                if request.target_category_id:
                    tag.category_id = request.target_category_id
                if request.target_facet_group:
                    tag.facet_group = request.target_facet_group
                if request.add_synonyms:
                    tag.synonyms.extend(request.add_synonyms)

                self._register_canonical_tag(tag)
                promoted_list.append(tag)
                logger.info(f"Promoted candidate tag '{slug}' to canonical facet ({tag.facet_group.value})")
            elif slug in self._canonical_tags:
                tag = self._canonical_tags[slug]
                if request.add_synonyms:
                    tag.synonyms.extend(request.add_synonyms)
                    for s in request.add_synonyms:
                        self._synonym_index[_strip_accents(s)] = slug
                promoted_list.append(tag)
            else:
                new_tag = TagItem(
                    tag_id=f"tag-{slug}",
                    name=slug.replace("-", " ").title(),
                    slug=slug,
                    facet_group=request.target_facet_group or FacetGroup.FEATURE,
                    category_id=request.target_category_id or "all",
                    status=TagStatus.PROMOTED,
                    confidence=1.0,
                    is_canonical=True,
                    synonyms=request.add_synonyms,
                )
                self._register_canonical_tag(new_tag)
                promoted_list.append(new_tag)

        return PromoteTagResponse(
            promoted_tags=promoted_list,
            total_promoted=len(promoted_list),
            active_canonical_count=len(self._canonical_tags),
        )

    # ──────────────────────────────────────────────────────────────────────────
    # 6. Query Tags
    # ──────────────────────────────────────────────────────────────────────────
    async def list_tags(self, request: ListTagsRequest) -> ListTagsResponse:
        all_tags: list[TagItem] = list(self._canonical_tags.values()) + list(self._candidate_tags.values())

        filtered = all_tags
        if request.category_id:
            filtered = [t for t in filtered if t.category_id in ("all", request.category_id)]
        if request.facet_group:
            filtered = [t for t in filtered if t.facet_group == request.facet_group]
        if request.status:
            filtered = [t for t in filtered if t.status == request.status]

        return ListTagsResponse(
            tags=filtered,
            total=len(filtered),
            canonical_count=len(self._canonical_tags),
            candidate_count=len(self._candidate_tags),
        )
