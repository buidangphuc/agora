"""Gateway helpers for order/multishop_checkout.feature (change order-multishop-checkout)."""

from __future__ import annotations

import uuid
from typing import Any

from tests.e2e.support import oic_order_support as s

PREVIEW = "PreviewCheckout"
SHIP_FEE_DA_NANG = 35_000


def fresh_code(prefix: str) -> str:
    return f"{prefix}{uuid.uuid4().hex[:8].upper()}"


def create_address(w: s.OicWorld, buyer: s.Actor, city: str) -> None:
    s.ok(
        s.post(
            w,
            buyer,
            s.ADDRESS,
            "CreateAddress",
            {
                "recipientName": "Nguyen Van A",
                "phone": "0912345678",
                "street": "29 Lieu Giai",
                "city": city,
                "ward": "Phuong Lieu Giai",
                "district": "Quan Ba Dinh",
                "isDefault": True,
            },
        )
    )


def seed_voucher(
    w: s.OicWorld,
    owner: s.Actor,
    scope: str,
    code: str,
    *,
    percent: int = 0,
    fixed: int = 0,
    min_spend: int = 0,
) -> str:
    body: dict[str, Any] = {
        "code": code,
        "scope": scope,
        "discountType": "DISCOUNT_TYPE_PERCENT" if percent else "DISCOUNT_TYPE_FIXED",
        "discountValue": str(percent or fixed),
        "minSpend": str(min_spend),
        "maxDiscount": "0",
        "quota": "100",
        "startsAt": "2026-01-01T00:00:00Z",
        "endsAt": "2030-12-31T23:59:59Z",
    }
    resp = s.post(w, owner, s.PROMO, "CreateVoucher", body)
    created = s.ok(resp).get("voucher") or {}
    assert created.get("code") == code, resp.text
    return code


def voucher_used(w: s.OicWorld, admin: s.Actor, code: str) -> int:
    resp = s.post(w, admin, s.PROMO, "GetVoucher", {"code": code})
    return int((s.ok(resp).get("voucher") or {}).get("used", 0) or 0)


def request_body(req: dict[str, Any]) -> dict[str, Any]:
    body: dict[str, Any] = {}
    if req.get("shop_vouchers"):
        body["shopVouchers"] = [
            {"sellerId": sid, "code": code} for sid, code in req["shop_vouchers"]
        ]
    if req.get("platform"):
        body["platformVoucherCode"] = req["platform"]
    if req.get("item_ids") is not None:
        body["itemIds"] = req["item_ids"]
    return body


def preview(w: s.OicWorld, buyer: s.Actor | None, req: dict[str, Any]):
    return s.post(w, buyer, s.ORDER, PREVIEW, request_body(req))


def place(w: s.OicWorld, buyer: s.Actor, req: dict[str, Any]):
    body = {"paymentMethod": "PAYMENT_METHOD_COD", **request_body(req)}
    body.pop("itemIds", None)
    return s.post(w, buyer, s.ORDER, "CreateOrder", body)


def groups_by_seller(resp: dict[str, Any]) -> dict[str, dict[str, Any]]:
    return {g["sellerId"]: g for g in resp.get("groups", [])}


def n(value: Any) -> int:
    return int(value or 0)
