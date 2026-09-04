"""Cart service: seed/inspect a buyer's cart via the gateway."""

from __future__ import annotations

from src.constants import gateway_endpoints as ep

from .base_service import BaseService


class CartService(BaseService):
    def add_to_cart(self, listing_id: str, quantity: int = 1) -> dict:
        return self.post(ep.CART_ADD, {"listingId": listing_id, "quantity": quantity})

    def get_cart(self) -> dict:
        return self.post(ep.CART_GET, {})

    def clear_cart(self) -> dict:
        return self.post(ep.CART_CLEAR, {})
