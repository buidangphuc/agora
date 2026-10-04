"""Listing service: create/list/delete listings + categories (used for API seeding)."""

from __future__ import annotations

from typing import Any

from src.constants import gateway_endpoints as ep
from src.models import Listing

from .base_service import BaseService

# Frontend/proto status enum names (a bare "published" is ignored -> defaults DRAFT).
_STATUS_ENUM = {
    "published": "LISTING_STATUS_PUBLISHED",
    "draft": "LISTING_STATUS_DRAFT",
}


class ListingService(BaseService):
    def create_listing(self, listing: Listing, variants: list[dict[str, Any]] | None = None) -> str:
        """Create a listing (requires a seller bearer token). Returns listing id.

        `variants` (optional) are Variant dicts (`name`, `sku`, `price`, `stock`,
        `imageUrl`); the server assigns their ids.

        The RPC wraps the entity: request `{"listing": {...}}`, response
        `{"listing": {"id": ...}}`.
        """
        payload: dict[str, Any] = {
            "listing": {
                "title": listing.title,
                "categoryId": listing.category_id,
                "price": listing.price,
                "stock": listing.stock,
                "status": _STATUS_ENUM.get(listing.status, listing.status),
                "currency": listing.currency,
                "description": listing.description,
            }
        }
        if variants:
            payload["listing"]["variants"] = variants
        data = self.post(ep.LISTING_CREATE, payload)
        listing_id = (data.get("listing") or {}).get("id", "")
        listing.listing_id = listing_id
        return listing_id

    def get_listing(self, listing_id: str) -> dict[str, Any]:
        """Read a listing back (public scope); used to learn server-assigned variant ids."""
        data = self.post("/platform.listing.v1.ListingService/GetListing", {"id": listing_id})
        return data.get("listing") or {}

    def delete_listing(self, listing_id: str) -> dict[str, Any]:
        return self.post("/platform.listing.v1.ListingService/DeleteListing", {"id": listing_id})

    def list_categories(self) -> list[dict]:
        data = self.post(ep.LISTING_CATEGORIES, {})
        result = data.get("result") or data
        return result.get("categories", [])

    # ── Storefront / shop display name ───────────────────────────────────
    def upsert_storefront(self, slug: str, display_name: str = "") -> dict[str, Any]:
        """Create/replace the calling seller's storefront (seller bearer token).

        The seller id is forced from the principal server-side.
        """
        data = self.post(
            ep.LISTING_UPSERT_STOREFRONT,
            {"storefront": {"slug": slug, "displayName": display_name}},
        )
        return data.get("storefront") or {}

    def get_storefront(self, seller_id: str) -> dict[str, Any]:
        data = self.post(ep.LISTING_GET_STOREFRONT, {"sellerId": seller_id})
        return data.get("storefront") or {}

    def batch_get_storefronts(self, seller_ids: list[str]) -> dict[str, str]:
        """BatchGetStorefronts -> {sellerId: displayName} (unknown sellers omitted)."""
        data = self.post(ep.LISTING_BATCH_GET_STOREFRONTS, {"sellerIds": seller_ids})
        return {s.get("sellerId", ""): s.get("displayName", "") for s in data.get("shops", [])}
