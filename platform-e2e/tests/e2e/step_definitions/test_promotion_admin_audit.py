"""Binds promo/promotion_admin_audit.feature (change promotion-admin-audit, initiative q3-audit-log).

Black box through the gateway (Connect JSON). Audit assertions go through support/audit_support.
"""

from __future__ import annotations

import base64
import json
import uuid

import httpx
from pytest_bdd import given, parsers, scenarios, then, when

from config.settings import get_settings
from src.models import Listing
from tests.e2e.step_definitions.flash_sale_ownership_steps import _campaign_response, _register_seller
from tests.e2e.support import audit_support as aud
from tests.e2e.support.world import World

scenarios("promo/promotion_admin_audit.feature")

_VOUCHER = "/platform.promotion.v1.VoucherService/"
_FLASH = "/platform.promotion.v1.FlashSaleService/"
_SPONSORED = "/platform.promotion.v1.SponsoredService/"


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


def _voucher_body(code: str, scope: str) -> dict:
    return {
        "code": code,
        "scope": scope,
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


# ── Givens ───────────────────────────────────────────────────────────────
@given("the seeded admin for the promotion audit check")
def seeded_admin(world: World) -> None:
    token = aud.admin_token(world)
    _x(world).update(pa_admin=token, pa_admin_id=_sub(token))


@given("a promotion seller with a published listing")
def seller_with_listing(world: World) -> None:
    token = _register_seller(world, "promo_aud")
    listing = Listing(
        title=f"[E2E][PromoAudit] {uuid.uuid4().hex[:8]}",
        category_id="cat-electronics",
        price=5_000_000,
        stock=100,
        status="published",
        description="Seed listing for the promotion admin audit E2E.",
    )
    world.service_factory.listing.create_listing(listing)
    _x(world).update(pa_seller=token, pa_seller_id=_sub(token), pa_listing=listing.listing_id)


@given("team-audit is stopped for the promotion audit check")
def audit_stopped(world: World) -> None:
    aud.stop_audit(world)


# ── Whens ────────────────────────────────────────────────────────────────
@when("the admin creates a PLATFORM voucher with a fresh code, discount type percent and value 10")
def admin_platform_voucher(world: World) -> None:
    st = _x(world)
    st["pa_code"] = _fresh_code("PAUD")
    st["pa_resp"] = _call(
        st["pa_admin"], _VOUCHER + "CreateVoucher", _voucher_body(st["pa_code"], "VOUCHER_SCOPE_PLATFORM")
    )


@when("the seller creates a SHOP voucher with a fresh code")
def seller_shop_voucher(world: World) -> None:
    st = _x(world)
    st["pa_code"] = _fresh_code("PSHOP")
    st["pa_resp"] = _call(
        st["pa_seller"], _VOUCHER + "CreateVoucher", _voucher_body(st["pa_code"], "VOUCHER_SCOPE_SHOP")
    )


@when("the admin creates a flash-sale campaign on that listing")
def admin_flash(world: World) -> None:
    st = _x(world)
    st["pa_resp"] = _campaign_response(st["pa_admin"], st["pa_listing"])


@when("the seller creates a flash-sale campaign on their own listing")
def seller_flash(world: World) -> None:
    st = _x(world)
    st["pa_resp"] = _campaign_response(st["pa_seller"], st["pa_listing"])


def _ad(token: str, listing_id: str) -> httpx.Response:
    return _call(
        token,
        _SPONSORED + "CreateAdCampaign",
        {"listingId": listing_id, "budget": 50_000, "bid": 1000},
    )


@when(parsers.parse("the admin creates an ad campaign with budget {budget:d} and bid {bid:d} on that listing"))
def admin_ad(world: World, budget: int, bid: int) -> None:
    st = _x(world)
    st["pa_resp"] = _call(
        st["pa_admin"],
        _SPONSORED + "CreateAdCampaign",
        {"listingId": st["pa_listing"], "budget": budget, "bid": bid},
    )


@when("the seller creates an ad campaign on their own listing")
def seller_ad(world: World) -> None:
    st = _x(world)
    st["pa_resp"] = _ad(st["pa_seller"], st["pa_listing"])


# ── Thens ────────────────────────────────────────────────────────────────
@then("the promotion call succeeds")
def call_ok(world: World) -> None:
    resp = _x(world)["pa_resp"]
    assert resp.status_code == 200, f"{resp.status_code} {resp.text}"


@then(parsers.parse('the promotion call fails with code "{code}"'))
def call_fails(world: World, code: str) -> None:
    resp = _x(world)["pa_resp"]
    assert resp.status_code >= 400, f"expected failure, got {resp.status_code} {resp.text}"
    assert resp.json().get("code") == code, f"{resp.status_code} {resp.text}"


def _get_voucher(world: World, code: str) -> httpx.Response:
    return _call(_x(world)["pa_admin"], _VOUCHER + "GetVoucher", {"code": code})


@then("the voucher exists")
def voucher_exists(world: World) -> None:
    code = _x(world)["pa_code"]
    resp = _get_voucher(world, code)
    assert resp.status_code == 200, f"GetVoucher {code}: {resp.status_code} {resp.text}"
    assert (resp.json().get("voucher") or {}).get("code") == code, resp.text


@then("the voucher is not found")
def voucher_missing(world: World) -> None:
    code = _x(world)["pa_code"]
    resp = _get_voucher(world, code)
    assert resp.status_code == 404 or resp.json().get("code") == "not_found", (
        f"voucher {code} should not exist: {resp.status_code} {resp.text}"
    )


def _campaign(world: World) -> dict:
    resp = _x(world)["pa_resp"]
    campaign = resp.json().get("campaign") or {}
    assert campaign.get("id"), f"no campaign in response: {resp.text}"
    return campaign


@then(
    parsers.parse(
        'the audit log holds exactly one "{action}" event for the voucher by the admin '
        'with discount type "{dtype}" and value "{value}"'
    )
)
def voucher_event(world: World, action: str, dtype: str, value: str) -> None:
    st = _x(world)
    aud.assert_one_event(
        world,
        target_type="voucher",
        target_id=st["pa_code"],
        action=action,
        actor_id=st["pa_admin_id"],
        metadata={"discount_type": dtype, "value": value},
    )


@then(
    parsers.parse(
        'the audit log holds exactly one "{action}" event for the returned campaign by the admin with the listing id'
    )
)
def flash_event(world: World, action: str) -> None:
    st = _x(world)
    aud.assert_one_event(
        world,
        target_type="flash_sale",
        target_id=_campaign(world)["id"],
        action=action,
        actor_id=st["pa_admin_id"],
        metadata={"listing_id": st["pa_listing"]},
    )


@then(
    parsers.parse(
        'the audit log holds exactly one "{action}" event for the returned campaign by the admin '
        'with the listing id, the stored seller id and budget "{budget}"'
    )
)
def ad_event(world: World, action: str, budget: str) -> None:
    st = _x(world)
    campaign = _campaign(world)
    aud.assert_one_event(
        world,
        target_type="ad_campaign",
        target_id=campaign["id"],
        action=action,
        actor_id=st["pa_admin_id"],
        metadata={"seller_id": campaign["sellerId"], "listing_id": st["pa_listing"], "budget": budget},
    )


@then(parsers.parse('the audit log holds no "{action}" event for the returned campaign after team-audit is started again'))
def no_campaign_event(world: World, action: str) -> None:
    aud.start_audit(world)
    target_type = "flash_sale" if action.startswith("flash_sale") else "ad_campaign"
    aud.assert_no_event(world, target_type=target_type, target_id=_campaign(world)["id"], action=action)


@then(parsers.parse('the audit log holds no "{action}" event for the voucher after team-audit is started again'))
def no_voucher_event(world: World, action: str) -> None:
    aud.start_audit(world)
    aud.assert_no_event(world, target_type="voucher", target_id=_x(world)["pa_code"], action=action)


@then("the listing has no active flash sale")
def no_active_sale(world: World) -> None:
    st = _x(world)
    aud.start_audit(world)
    resp = _call(
        st["pa_admin"], _FLASH + "GetActiveFlashSale", {"listingId": st["pa_listing"]}
    )
    assert resp.status_code == 200, f"{resp.status_code} {resp.text}"
    assert not resp.json().get("active"), f"unexpected active flash sale: {resp.text}"


@then("the listing is absent from the sponsored slots")
def not_in_slots(world: World) -> None:
    st = _x(world)
    aud.start_audit(world)
    resp = _call(st["pa_admin"], _SPONSORED + "ListSponsoredSlots", {"contextStr": ""})
    assert resp.status_code == 200, f"{resp.status_code} {resp.text}"
    assert st["pa_listing"] not in (resp.json().get("listingIds") or []), resp.text
