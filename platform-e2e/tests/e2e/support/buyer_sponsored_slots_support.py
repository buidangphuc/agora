"""Helpers of frontend/buyer_sponsored_slots.feature (change buyer-sponsored-slots).

Black box: accounts, listings and campaigns are seeded through the gateway; spent is read back
through the real ListMyAdCampaigns; the browser is the system under test.
"""

from __future__ import annotations

import uuid
from typing import Any

import httpx

from config.settings import get_settings
from src.models import Listing
from src.utils import data as fake
from tests.e2e.flows.oic_inv_flow import poll
from tests.e2e.support.world import World

SPONSORED = "/platform.promotion.v1.SponsoredService/"
SEARCH = "/platform.search.v1.SearchService/SearchListings"
CLICK_PATH = "/api/ads/click"


def state(world: World) -> dict[str, Any]:
    return world.state.extra.setdefault("bss", {})


def post(token: str | None, path: str, body: dict) -> dict:
    headers = {"Content-Type": "application/json"}
    if token:
        headers["Authorization"] = f"bearer {token}"
    url = f"{get_settings().gateway_url.rstrip('/')}{path}"
    resp = httpx.post(url, json=body, headers=headers, timeout=30)
    assert resp.status_code == 200, f"{path}: {resp.status_code} {resp.text}"
    return resp.json()


def seed(world: World) -> None:
    st = state(world)
    token = world.service_factory.auth.register(
        fake.unique_username("bss_seller"), get_settings().seed_password, "seller"
    )
    world.service_factory.set_token(token)
    category = f"cat-bss-{uuid.uuid4().hex[:10]}"
    ids = []
    for label in ("sponsored", "organic"):
        listing = Listing(
            title=f"[E2E][Bss] {label} {uuid.uuid4().hex[:8]}",
            category_id=category,
            price=5_000_000,
            stock=100,
            status="published",
            description="Seed listing for the buyer sponsored slots E2E.",
        )
        world.service_factory.listing.create_listing(listing)
        ids.append(listing.listing_id)
    st.update(seller=token, category=category, sponsored=ids[0], organic=ids[1])


def create_campaign(world: World, bid: int, budget: int) -> None:
    st = state(world)
    data = post(
        st["seller"],
        SPONSORED + "CreateAdCampaign",
        {"listingId": st["sponsored"], "budget": budget, "bid": bid},
    )
    campaign = data.get("campaign") or {}
    assert campaign.get("id"), data
    assert campaign.get("status") == "AD_CAMPAIGN_STATUS_ACTIVE", campaign
    st["campaign"] = campaign["id"]


def wait_searchable(world: World) -> None:
    st = state(world)

    def both() -> bool:
        data = post(None, SEARCH, {"categoryId": st["category"]})
        found = {h.get("listingId") for h in data.get("hits") or []}
        return {st["sponsored"], st["organic"]} <= found

    assert poll(both, timeout_s=60), "listings were not indexed for search in time"


def spent(world: World) -> int:
    st = state(world)
    data = post(st["seller"], SPONSORED + "ListMyAdCampaigns", {})
    for campaign in data.get("campaigns") or []:
        if campaign.get("id") == st["campaign"]:
            return int(campaign.get("spent", 0))
    raise AssertionError(f"campaign {st['campaign']} not in ListMyAdCampaigns: {data}")
