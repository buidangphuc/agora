"""Ready-to-index OpenSearch documents for a classified SPU and its variants."""

from typing import Any

from app.modules.business.tag_classifier.schemas import (
    ClassifySkuHierarchyRequest,
    SkuClassificationResult,
    TagItem,
)


def variant_document(sku: SkuClassificationResult) -> dict[str, Any]:
    return {
        "variant_id": sku.variant_id,
        "sku_code": sku.sku_code,
        "name": sku.name,
        "price": sku.price,
        "stock": sku.stock,
        "is_in_stock": sku.is_in_stock,
        "facets": sku.variant_facets,
        "tags": [t.slug for t in sku.all_effective_tags],
    }


def spu_document(
    request: ClassifySkuHierarchyRequest,
    spu_tags: list[TagItem],
    spu_facets: dict[str, list[str]],
    variants: list[dict[str, Any]],
) -> dict[str, Any]:
    prices = [v.price for v in request.variants if v.price > 0]
    total_stock = sum(v.stock for v in request.variants)
    return {
        "title": request.spu_title,
        "category_id": request.category_id,
        "price_min": min(prices) if prices else 0,
        "price_max": max(prices) if prices else 0,
        "total_stock": total_stock,
        "is_in_stock": total_stock > 0,
        "spu_tags": [t.slug for t in spu_tags],
        "spu_facets": spu_facets,
        "variants": variants,
    }
