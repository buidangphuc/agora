"""Black-box helpers for change seller-marketing-hub (initiative seller-centre).

Everything is seeded and read back through the gateway (Connect JSON) as the owning seller; the only
direct SQL is fixture shaping (ending or starting a campaign). Fresh accounts per scenario.
"""

from __future__ import annotations

import time
import uuid
from typing import Any

from config.settings import get_settings
from src.models import Listing
from tests.e2e.flows.auth_flow import session_cookies
from tests.e2e.support import admin_campaigns_support as ac
from tests.e2e.support import admin_vouchers_support as av
from tests.e2e.support import flash_hold_support as fh
from tests.e2e.support import voucher_scope_support as vs
from tests.e2e.support.world import World

SALE_PRICE_FORM = 50000
LABEL_ACTIVE = "Đang chạy"
LABEL_SCHEDULED = "Sắp diễn ra"
LABEL_ENDED = "Đã kết thúc"
LABEL_DEACTIVATED = "Đã tắt"


def state(world: World) -> dict:
    return world.state.extra.setdefault("smh", {})


def register(world: World, role: str, prefix: str) -> tuple[str, str]:
    from src.utils import data as fake

    token = world.service_factory.auth.register(
        fake.unique_username(prefix), get_settings().seed_password, role
    )
    return token, vs.token_sub(token)


def sign_in(world: World, token: str) -> None:
    world.context.add_cookies(session_cookies(world.settings, token))


def make_listing(world: World, token: str, tag: str, title: str | None = None) -> tuple[str, str]:
    world.service_factory.set_token(token)
    listing = Listing(
        title=title or f"[E2E][Mkt] {tag} {uuid.uuid4().hex[:6]}",
        category_id="cat-electronics",
        price=ac.LISTING_PRICE,
        stock=100,
        status="published",
        description="Sản phẩm seed tự động cho seller marketing hub E2E.",
    )
    world.service_factory.listing.create_listing(listing)
    assert listing.listing_id, "listing was not created"
    return str(listing.listing_id), listing.title


def my_vouchers(token: str) -> list[dict[str, Any]]:
    found: list[dict[str, Any]] = []
    cursor = ""
    for _ in range(30):
        resp = av.gw(token, "ListMyShopVouchers", {"page": {"pageSize": 200, "cursor": cursor}})
        assert resp.status_code == 200, f"ListMyShopVouchers {resp.status_code}: {resp.text}"
        found.extend(resp.json().get("vouchers") or [])
        cursor = (resp.json().get("page") or {}).get("nextCursor") or ""
        if not cursor:
            break
    return found


def my_campaigns(token: str, statuses: list[str] | None = None) -> list[dict[str, Any]]:
    found: list[dict[str, Any]] = []
    cursor = ""
    for _ in range(30):
        body: dict[str, Any] = {"page": {"pageSize": 200, "cursor": cursor}}
        if statuses:
            body["statuses"] = statuses
        resp = ac.gw(token, "ListMyCampaigns", body)
        assert resp.status_code == 200, f"ListMyCampaigns {resp.status_code}: {resp.text}"
        found.extend(resp.json().get("campaigns") or [])
        cursor = (resp.json().get("page") or {}).get("nextCursor") or ""
        if not cursor:
            break
    return found


def by_id(items: list[dict[str, Any]], item_id: str) -> dict[str, Any] | None:
    return next((i for i in items if i.get("id") == item_id), None)


def shop_voucher(token: str, prefix: str, **kw: Any) -> tuple[str, str]:
    code = av.fresh_code(prefix)
    return code, av.create_voucher(token, code, scope=av.SHOP, **kw)["id"]


def preview_valid(buyer_token: str, buyer_id: str, seller_id: str, code: str) -> bool:
    resp = av.gw(
        buyer_token,
        "ValidateAndReserve",
        {
            "reservationId": f"preview:{buyer_id}:{uuid.uuid4().hex[:12]}",
            "code": code,
            "buyerId": buyer_id,
            "cartSubtotal": "1000000",
            "sellerId": seller_id,
        },
    )
    assert resp.status_code == 200, f"preview: {resp.status_code} {resp.text}"
    return bool(resp.json().get("valid"))


def start_now(campaign_id: str) -> None:
    fh.psql(
        "UPDATE flash_sale_campaigns SET starts_at = now() - interval '1 hour' "
        f"WHERE id = {fh.sql_lit(campaign_id)}"
    )


def wait_for_my_voucher_code(token: str, deadline_s: float = 60) -> str:
    end = time.monotonic() + deadline_s
    while time.monotonic() < end:
        resp = av.gw(token, "ListMyVouchers", {"page": {"pageSize": 50}})
        vouchers = resp.json().get("vouchers") or [] if resp.status_code == 200 else []
        if vouchers:
            return str(vouchers[0]["code"])
        time.sleep(1)
    raise AssertionError("the buyer holds no voucher within the deadline")
