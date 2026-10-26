from __future__ import annotations

from typing import Any

from src.api.services import AddressService

_UPDATE = "/platform.identity.v1.AddressService/UpdateAddress"

DELIVERY_FIELDS = ("recipientName", "phone", "street", "ward", "district", "city")


def create_address(world, buyer, city: str, is_default: bool = True) -> dict[str, Any]:
    svc = AddressService(token=buyer.token)
    world.add_cleanup(svc.close)
    res = svc.create_address(
        recipient_name="Nguyen Van A",
        phone="0912345678",
        street="29 Lieu Giai",
        city=city,
        ward="Phuong Lieu Giai",
        district="Quan Ba Dinh",
        is_default=is_default,
    )
    address = res.get("address", {})
    assert address.get("id"), f"CreateAddress returned no address: {res}"
    return address


def edit_address(world, buyer, address: dict[str, Any], **fields: Any) -> dict[str, Any]:
    svc = AddressService(token=buyer.token)
    world.add_cleanup(svc.close)
    body = {k: address.get(k, "") for k in DELIVERY_FIELDS}
    body["isDefault"] = bool(address.get("isDefault", False))
    body["id"] = address["id"]
    body.update(fields)
    res = svc.post(_UPDATE, body)
    return res.get("address", {})


def list_addresses(world, buyer) -> list[dict[str, Any]]:
    svc = AddressService(token=buyer.token)
    world.add_cleanup(svc.close)
    return svc.list_addresses().get("addresses", [])
