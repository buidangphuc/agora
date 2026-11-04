"""Black-box helpers for change promotion-admin-campaigns (initiative p10-backend-gaps).

Everything goes through the gateway (Connect JSON). Campaigns that are only set-up are created as
the owning seller (audit-free); admin-path creates, cancels and edits are the behaviour under test.
Service-only hold RPCs go through `flash_hold_support` (grpcurl). Direct SQL is limited to
fixture shaping (ending a campaign) and reading nothing the API does not expose.
"""

from __future__ import annotations

import subprocess
import time
import uuid
from datetime import datetime, timedelta, timezone
from typing import Any

import httpx

from config.settings import get_settings
from tests.e2e.support import admin_vouchers_support as av
from tests.e2e.support import audit_support as aud
from tests.e2e.support import flash_hold_support as fh
from tests.e2e.support.world import World

FLASH = "/platform.promotion.v1.FlashSaleService/"
FLAG_KEY = "flash-sale-enabled"
LISTING_PRICE = 5_000_000
SALE_PRICE = 4_000_000
SCHEDULED = "CAMPAIGN_STATUS_SCHEDULED"
ACTIVE = "CAMPAIGN_STATUS_ACTIVE"
ENDED = "CAMPAIGN_STATUS_ENDED"
CANCELLED = "CAMPAIGN_STATUS_CANCELLED"
AUDIT_ACTIONS = {
    "create": "flash_sale.admin_create",
    "update": "campaign.update",
    "cancel": "campaign.cancel",
}

iso = av.iso
code_of = av.code_of
message_of = av.message_of
error_info = av.error_info


def state(world: World) -> dict:
    return world.state.extra.setdefault("ac", {})


def fresh_key(prefix: str = "ac") -> str:
    return f"{prefix}-{uuid.uuid4().hex[:16]}"


def gw(token: str | None, method: str, body: dict[str, Any]) -> httpx.Response:
    return av.gw_path(token, FLASH + method, body)


def campaign_of(resp: httpx.Response) -> dict[str, Any]:
    assert resp.status_code == 200, f"{resp.status_code} {resp.text}"
    campaign = resp.json().get("campaign") or {}
    assert campaign.get("id"), f"no campaign in response: {resp.text}"
    return campaign


def make_listing(world: World, seller_token: str, price: int = LISTING_PRICE) -> str:
    from src.models import Listing

    world.service_factory.set_token(seller_token)
    listing = Listing(
        title=f"[E2E][AdminCampaign] {uuid.uuid4().hex[:8]}",
        category_id="cat-electronics",
        price=price,
        stock=100,
        status="published",
        description="Sản phẩm seed tự động cho admin campaigns E2E.",
    )
    world.service_factory.listing.create_listing(listing)
    assert listing.listing_id, "listing was not created"
    return str(listing.listing_id)


def seller_campaign(
    seller_token: str,
    listing_id: str,
    *,
    stock_cap: int = 10,
    sale_price: int = SALE_PRICE,
    starts_in_s: float = -3600,
    ends_in_s: float = 86400 * 5,
) -> str:
    resp = gw(
        seller_token,
        "CreateCampaign",
        {
            "listingId": listing_id,
            "salePrice": str(sale_price),
            "stockCap": str(stock_cap),
            "startsAt": iso(offset_s=starts_in_s),
            "endsAt": iso(offset_s=ends_in_s),
        },
    )
    return campaign_of(resp)["id"]


def admin_create_body(
    listing_id: str,
    key: str,
    *,
    sale_price: int = SALE_PRICE,
    stock_cap: int = 10,
    starts_in_s: float = 86400,
    ends_in_s: float = 86400 * 3,
    starts_at: str | None = None,
    ends_at: str | None = None,
) -> dict[str, Any]:
    return {
        "listingId": listing_id,
        "salePrice": str(sale_price),
        "stockCap": str(stock_cap),
        "startsAt": starts_at or iso(offset_s=starts_in_s),
        "endsAt": ends_at or iso(offset_s=ends_in_s),
        "idempotencyKey": key,
    }


def end_now(campaign_id: str) -> None:
    fh.psql(
        "UPDATE flash_sale_campaigns SET starts_at = now() - interval '3 days', "
        f"ends_at = now() - interval '1 minute' WHERE id = {fh.sql_lit(campaign_id)}"
    )


def list_campaigns(
    token: str | None, statuses: list[str] | None = None, page_size: int = 200
) -> httpx.Response:
    body: dict[str, Any] = {"page": {"pageSize": page_size}}
    if statuses:
        body["statuses"] = statuses
    return gw(token, "ListCampaigns", body)


def all_campaigns(
    token: str, statuses: list[str] | None = None, max_pages: int = 60
) -> tuple[list[dict[str, Any]], bool]:
    found: list[dict[str, Any]] = []
    disabled = False
    cursor = ""
    for _ in range(max_pages):
        body: dict[str, Any] = {"page": {"pageSize": 200, "cursor": cursor}}
        if statuses:
            body["statuses"] = statuses
        resp = gw(token, "ListCampaigns", body)
        assert resp.status_code == 200, f"ListCampaigns {resp.status_code}: {resp.text}"
        data = resp.json()
        found.extend(data.get("campaigns") or [])
        disabled = disabled or bool(data.get("flashSaleDisabled"))
        cursor = (data.get("page") or {}).get("nextCursor") or ""
        if not cursor:
            break
    return found, disabled


def find(token: str, campaign_id: str, statuses: list[str] | None = None) -> dict[str, Any] | None:
    campaigns, _ = all_campaigns(token, statuses)
    for c in campaigns:
        if c.get("id") == campaign_id:
            return c
    return None


def for_listing(token: str, listing_id: str) -> list[dict[str, Any]]:
    campaigns, _ = all_campaigns(token)
    return [c for c in campaigns if c.get("listingId") == listing_id]


def admin_create_events(world: World, since: str) -> int:
    token = aud.admin_token(world)
    total = 0
    cursor: str | None = None
    for _ in range(50):
        resp = aud.query_events(
            token, action=AUDIT_ACTIONS["create"], created_after=since, cursor=cursor
        )
        assert resp.status_code == 200, resp.text
        data = resp.json()
        total += len(data.get("events", []))
        cursor = (data.get("page") or {}).get("nextCursor") or None
        if not cursor:
            break
    return total


def audit_events(world: World, campaign_id: str, action: str) -> list[dict]:
    return [
        e
        for e in aud.events_for(world, "flash_sale", campaign_id, action=action)
        if e.get("action") == action
    ]


def _flipt() -> httpx.Client:
    return httpx.Client(base_url=get_settings().flipt_url.rstrip("/"), timeout=15.0)


def set_flash_flag(enabled: bool) -> None:
    base = "/api/v1/namespaces/default/flags"
    payload = {
        "key": FLAG_KEY,
        "name": "Flash sale enabled",
        "type": "BOOLEAN_FLAG_TYPE",
        "enabled": enabled,
    }
    with _flipt() as client:
        current = client.get(f"{base}/{FLAG_KEY}")
        if current.status_code == httpx.codes.OK and current.json().get("enabled") is enabled:
            return
        resp = client.put(f"{base}/{FLAG_KEY}", json=payload)
        if resp.status_code == httpx.codes.NOT_FOUND:
            client.post(base, json=payload)


def active_list(token: str | None = None) -> httpx.Response:
    return gw(token, "ListActiveCampaigns", {"page": {"pageSize": 200}})


def wait_active_disabled(expected: bool, timeout_s: float = 30) -> None:
    deadline = time.monotonic() + timeout_s
    last = ""
    while time.monotonic() < deadline:
        resp = active_list()
        if resp.status_code == 200:
            flag = bool(resp.json().get("flashSaleDisabled"))
            last = resp.text[:200]
            if flag is expected:
                return
        else:
            last = f"{resp.status_code} {resp.text[:200]}"
        time.sleep(1)
    raise AssertionError(f"flash_sale_disabled never became {expected}: {last}")


def stop_listing(world: World, token: str) -> None:
    st = state(world)
    if st.get("listing_stopped"):
        return
    container = get_settings_container()
    _docker("stop", container)
    st["listing_stopped"] = True

    def restore() -> None:
        if st.get("listing_stopped"):
            _docker("start", container)
            st["listing_stopped"] = False
        _wait_listing_up(token)

    world.add_cleanup(restore)


def start_listing(world: World, token: str) -> None:
    st = state(world)
    if st.get("listing_stopped"):
        _docker("start", get_settings_container())
        st["listing_stopped"] = False
    _wait_listing_up(token)


def get_settings_container() -> str:
    import os

    return os.getenv("DOMAIN_CONTAINER", "agora-team-domain-svc")


def _docker(*args: str) -> None:
    proc = subprocess.run(["docker", *args], capture_output=True, text=True, timeout=120)
    assert proc.returncode == 0, f"docker {' '.join(args)} failed: {proc.stderr}"


def _wait_listing_up(token: str) -> None:
    deadline = time.monotonic() + 90
    while time.monotonic() < deadline:
        try:
            resp = httpx.post(
                f"{get_settings().gateway_url.rstrip('/')}/platform.listing.v1.ListingService/ListListings",
                json={},
                headers={"Authorization": f"bearer {token}", "Content-Type": "application/json"},
                timeout=10,
            )
            if resp.status_code < 500:
                time.sleep(aud.RECONNECT_GRACE_S)
                return
        except httpx.HTTPError:
            pass
        time.sleep(1)
    raise TimeoutError("team-domain did not recover after the stop")


def tomorrow_window() -> tuple[str, str]:
    start = datetime.now(timezone.utc) + timedelta(days=2)
    return (
        start.strftime("%Y-%m-%dT%H:%M:%S.000Z"),
        (start + timedelta(days=2)).strftime("%Y-%m-%dT%H:%M:%S.000Z"),
    )
