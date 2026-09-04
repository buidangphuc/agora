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
    def create_listing(self, listing: Listing) -> str:
        """Create a listing (requires a seller bearer token). Returns listing id.

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
        data = self.post(ep.LISTING_CREATE, payload)
        listing_id = (data.get("listing") or {}).get("id", "")
        listing.listing_id = listing_id
        return listing_id

    def delete_listing(self, listing_id: str) -> dict[str, Any]:
        return self.post("/platform.listing.v1.ListingService/DeleteListing", {"id": listing_id})

    def list_categories(self) -> list[dict]:
        data = self.post(ep.LISTING_CATEGORIES, {})
        result = data.get("result") or data
        return result.get("categories", [])
