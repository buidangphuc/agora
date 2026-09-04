"""Address service client for platform.identity.v1.AddressService."""

from __future__ import annotations

from typing import Any

from src.api.services.base_service import BaseService
from src.constants import gateway_endpoints as ep


class AddressService(BaseService):
    def create_address(
        self,
        recipient_name: str,
        phone: str,
        street: str,
        city: str,
        ward: str = "Phường Liễu Giai",
        district: str = "Quận Ba Đình",
        is_default: bool = True,
    ) -> dict[str, Any]:
        """Create a delivery address for the authenticated principal. Flat payload."""
        return self.post(
            ep.ADDRESS_CREATE,
            {
                "recipientName": recipient_name,
                "phone": phone,
                "street": street,
                "ward": ward,
                "district": district,
                "city": city,
                "isDefault": is_default,
            },
        )

    def list_addresses(self) -> dict[str, Any]:
        return self.post(ep.ADDRESS_LIST, {})
