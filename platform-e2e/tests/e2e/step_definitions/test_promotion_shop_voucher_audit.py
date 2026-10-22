"""Binds promo/promotion_shop_voucher_audit.feature (change promotion-shop-voucher-audit).

Black box through the gateway (Connect JSON). Audit assertions go through support/audit_support.
"""

from __future__ import annotations

import base64
import json
import uuid

import httpx
from pytest_bdd import given, parsers, scenarios, then, when

from config.settings import get_settings
from tests.e2e.step_definitions.flash_sale_ownership_steps import _register_seller
from tests.e2e.support import audit_support as aud
from tests.e2e.support.world import World

scenarios("promo/promotion_shop_voucher_audit.feature")

_VOUCHER = "/platform.promotion.v1.VoucherService/"


def _x(world: World) -> dict:
    return world.state.extra


def _sub(token: str) -> str:
    payload = token.split(".")[1]
    payload += "=" * (-len(payload) % 4)
    return str(json.loads(base64.urlsafe_b64decode(payload))["sub"])


def _call(token: str | None, path: str, body: dict) -> httpx.Response:
    headers = {"Content-Type": "application/json"}
    if token:
        headers["Authorization"] = f"bearer {token}"
    url = f"{get_settings().gateway_url.rstrip('/')}{path}"
    return httpx.post(url, json=body, headers=headers, timeout=30)


def _shop_body(code: str) -> dict:
    return {
        "code": code,
        "scope": "VOUCHER_SCOPE_SHOP",
        "discountType": "DISCOUNT_TYPE_PERCENT",
        "discountValue": "10",
        "minSpend": "0",
        "maxDiscount": "50000",
        "quota": "100",
        "startsAt": "2026-01-01T00:00:00Z",
        "endsAt": "2030-01-01T00:00:00Z",
    }


def _fresh_code(prefix: str) -> str:
    return f"{prefix}{uuid.uuid4().hex[:8].upper()}"


@given("the seeded admin for the shop voucher audit check")
def seeded_admin(world: World) -> None:
    token = aud.admin_token(world)
    _x(world).update(sv_admin=token, sv_admin_id=_sub(token))


@given("a seller for the shop voucher audit check")
def seller(world: World) -> None:
    token = _register_seller(world, "shopv_aud")
    _x(world).update(sv_seller=token, sv_seller_id=_sub(token))


@given("team-audit is stopped for the shop voucher audit check")
def audit_stopped(world: World) -> None:
    aud.stop_audit(world)


@when("the admin creates a SHOP voucher with a fresh code, discount type percent and value 10")
def admin_shop_voucher(world: World) -> None:
    st = _x(world)
    st["sv_code"] = _fresh_code("SADM")
    st["sv_resp"] = _call(st["sv_admin"], _VOUCHER + "CreateVoucher", _shop_body(st["sv_code"]))


@when("the seller creates a SHOP voucher with a fresh code for the shop voucher audit check")
def seller_shop_voucher(world: World) -> None:
    st = _x(world)
    st["sv_code"] = _fresh_code("SSELL")
    st["sv_resp"] = _call(st["sv_seller"], _VOUCHER + "CreateVoucher", _shop_body(st["sv_code"]))


@then("the shop voucher call succeeds")
def call_ok(world: World) -> None:
    resp = _x(world)["sv_resp"]
    assert resp.status_code == 200, f"{resp.status_code} {resp.text}"


@then(parsers.parse('the shop voucher call fails with code "{code}"'))
def call_fails(world: World, code: str) -> None:
    resp = _x(world)["sv_resp"]
    assert resp.status_code >= 400, f"expected failure, got {resp.status_code} {resp.text}"
    assert resp.json().get("code") == code, f"{resp.status_code} {resp.text}"


def _get(world: World, code: str) -> httpx.Response:
    return _call(_x(world)["sv_admin"], _VOUCHER + "GetVoucher", {"code": code})


@then("the shop voucher exists")
def voucher_exists(world: World) -> None:
    code = _x(world)["sv_code"]
    resp = _get(world, code)
    assert resp.status_code == 200, f"GetVoucher {code}: {resp.status_code} {resp.text}"
    assert (resp.json().get("voucher") or {}).get("code") == code, resp.text


@then("the shop voucher is not found after team-audit is started again")
def voucher_missing(world: World) -> None:
    aud.start_audit(world)
    code = _x(world)["sv_code"]
    resp = _get(world, code)
    assert resp.status_code == 404 or resp.json().get("code") == "not_found", (
        f"voucher {code} should not exist: {resp.status_code} {resp.text}"
    )


@then(
    parsers.parse(
        'the audit log holds exactly one "{action}" event for the shop voucher by the admin '
        'with discount type "{dtype}", value "{value}" and the admin id as seller id'
    )
)
def admin_event(world: World, action: str, dtype: str, value: str) -> None:
    st = _x(world)
    aud.assert_one_event(
        world,
        target_type="voucher",
        target_id=st["sv_code"],
        action=action,
        actor_id=st["sv_admin_id"],
        metadata={"discount_type": dtype, "value": value, "seller_id": st["sv_admin_id"]},
    )


@then(parsers.parse('the audit log holds no "{action}" event for the shop voucher after team-audit is started again'))
def no_event(world: World, action: str) -> None:
    aud.start_audit(world)
    aud.assert_no_event(world, target_type="voucher", target_id=_x(world)["sv_code"], action=action)


@then("an admin SHOP voucher created now is audited as a positive control")
def positive_control(world: World) -> None:
    aud.start_audit(world)
    st = _x(world)
    code = _fresh_code("SCTL")
    resp = _call(st["sv_admin"], _VOUCHER + "CreateVoucher", _shop_body(code))
    assert resp.status_code == 200, f"control create: {resp.status_code} {resp.text}"
    aud.assert_one_event(
        world,
        target_type="voucher",
        target_id=code,
        action="voucher.admin_create",
        actor_id=st["sv_admin_id"],
    )
