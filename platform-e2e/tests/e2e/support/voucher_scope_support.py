"""Black-box access to VoucherService.ValidateAndReserve (change promotion-voucher-scope).

ValidateAndReserve is not routed by the gateway and team-promotion publishes no host port, so it is
driven with `grpcurl` from a throwaway container on the stack network, with the `x-principal-*`
headers the interceptor reads (same pattern as `flash_hold_support`). Vouchers are seeded through the
gateway `VoucherService/CreateVoucher`: shop vouchers as a seller, platform vouchers as admin.
"""

from __future__ import annotations

import base64
import json
import re
import uuid
from typing import Any

import httpx

from config.settings import get_settings
from tests.e2e.support import flash_hold_support as fh
from tests.e2e.support import plp_stack as stack

SERVICE = "platform.promotion.v1.VoucherService"
GATEWAY_VOUCHER = "/platform.promotion.v1.VoucherService/"
ORDER_PRINCIPAL = "service-team-order"
RESERVE_SCOPE = "promotion.reserve"
UNSPECIFIED = "VOUCHER_SCOPE_UNSPECIFIED"


def fresh_code(prefix: str) -> str:
    return f"{prefix}{uuid.uuid4().hex[:8].upper()}"


def fresh_reservation(prefix: str = "vs") -> str:
    return f"e2e-{prefix}-{uuid.uuid4().hex[:12]}"


def token_sub(token: str) -> str:
    payload = token.split(".")[1]
    payload += "=" * (-len(payload) % 4)
    return str(json.loads(base64.urlsafe_b64decode(payload))["sub"])


def validate_and_reserve(
    code: str,
    reservation_id: str,
    subtotal: int,
    seller_id: str = "",
    principal: str = "service",
    buyer_id: str = "e2e-buyer",
) -> tuple[str, Any]:
    """(connect-style code, response dict or error text). `principal` is `service`,
    `service-no-scope`, `none` or `user:<id>`."""
    pdir = fh.proto_dir()
    assert (pdir / fh.PROTO_FILE).is_file(), f"promotion proto not found under {pdir}"
    cmd = [
        "run", "--rm", "--network", stack.stack_network(), "-v", f"{pdir}:/proto:ro",
        fh.grpcurl_image(), "-plaintext", "-import-path", "/proto", "-proto", fh.PROTO_FILE,
    ]  # fmt: skip
    if principal == "service":
        cmd += ["-H", f"x-principal-id: {ORDER_PRINCIPAL}", "-H", "x-principal-type: service",
                "-H", f"x-principal-scopes: {RESERVE_SCOPE}"]  # fmt: skip
    elif principal == "service-no-scope":
        cmd += ["-H", "x-principal-id: service-other", "-H", "x-principal-type: service"]
    elif principal.startswith("user:"):
        cmd += ["-H", f"x-principal-id: {principal[5:]}", "-H", "x-principal-type: user"]
    body = {
        "reservationId": reservation_id,
        "code": code,
        "buyerId": buyer_id,
        "cartSubtotal": subtotal,
        "sellerId": seller_id,
    }
    cmd += ["-d", json.dumps(body), fh.promotion_addr(), f"{SERVICE}/ValidateAndReserve"]
    proc = stack.docker(*cmd, check=False, timeout=120)
    if proc.returncode == 0:
        return "ok", json.loads(proc.stdout or "{}")
    err = (proc.stderr + proc.stdout).strip()
    m = re.search(r"Code:\s*(\w+)", err)
    return (fh._snake(m.group(1)) if m else f"grpcurl_exit_{proc.returncode}"), err


def scope_of(body: dict[str, Any]) -> str:
    return str(body.get("scope") or UNSPECIFIED)


def seller_of(body: dict[str, Any]) -> str:
    return str(body.get("sellerId") or body.get("seller_id") or "")


def _post(token: str, path: str, body: dict[str, Any]) -> httpx.Response:
    url = f"{get_settings().gateway_url.rstrip('/')}{path}"
    headers = {"Content-Type": "application/json", "Authorization": f"bearer {token}"}
    return httpx.post(url, json=body, headers=headers, timeout=30)


def _voucher_body(code: str, scope: str, min_spend: int, quota: int) -> dict[str, Any]:
    return {
        "code": code,
        "scope": scope,
        "discountType": "DISCOUNT_TYPE_PERCENT",
        "discountValue": "10",
        "minSpend": str(min_spend),
        "maxDiscount": "0",
        "quota": str(quota),
        "startsAt": "2026-01-01T00:00:00Z",
        "endsAt": "2030-12-31T23:59:59Z",
    }


def _create(token: str, body: dict[str, Any]) -> str:
    resp = _post(token, GATEWAY_VOUCHER + "CreateVoucher", body)
    assert resp.status_code == 200, f"CreateVoucher {body['code']}: {resp.status_code} {resp.text}"
    assert (resp.json().get("voucher") or {}).get("code") == body["code"], resp.text
    return str(body["code"])


def seed_shop_voucher(seller_token: str, code: str, min_spend: int = 0, quota: int = 100) -> str:
    return _create(seller_token, _voucher_body(code, "VOUCHER_SCOPE_SHOP", min_spend, quota))


def seed_platform_voucher(admin_token: str, code: str, min_spend: int = 0, quota: int = 100) -> str:
    return _create(admin_token, _voucher_body(code, "VOUCHER_SCOPE_PLATFORM", min_spend, quota))


def reservation_rows(reservation_id: str) -> list[list[str]]:
    out = fh.psql(
        "SELECT reservation_id, buyer_id, status FROM voucher_reservations "
        f"WHERE reservation_id = {fh.sql_lit(reservation_id)}"
    ).stdout.strip()
    return [line.split("|") for line in out.splitlines() if line]
