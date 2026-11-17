"""Steps for ads/sponsored_serving.feature and ads/sponsored_click.feature (change promotion-ads-serving).

Black box through the gateway (Connect JSON); campaign state is read through ListMyAdCampaigns.
"""

from __future__ import annotations

import uuid

import httpx
from pytest_bdd import given, parsers, then, when

from config.settings import get_settings
from src.models import Listing
from src.utils import data as fake
from tests.e2e.step_definitions.flash_sale_ownership_steps import _register_seller
from tests.e2e.support.world import World

_SPONSORED = "/platform.promotion.v1.SponsoredService/"


def _x(world: World) -> dict:
    return world.state.extra


def _call(token: str | None, method: str, body: dict) -> httpx.Response:
    headers = {"Content-Type": "application/json"}
    if token:
        headers["Authorization"] = f"bearer {token}"
    url = f"{get_settings().gateway_url.rstrip('/')}{_SPONSORED}{method}"
    return httpx.post(url, json=body, headers=headers, timeout=30)


def _ok(resp: httpx.Response) -> dict:
    assert resp.status_code == 200, f"{resp.status_code} {resp.text}"
    return resp.json()


def _slots(resp: httpx.Response) -> list[dict]:
    return _ok(resp).get("slots") or []


def _slot_for(world: World) -> dict | None:
    listing = _x(world)["ad_listing"]
    for slot in _slots(_x(world)["ad_resp"]):
        if slot.get("listingId") == listing:
            return slot
    return None


def _buyer_token(world: World) -> str:
    token = _x(world).get("ad_buyer")
    if not token:
        token = world.service_factory.auth.register(
            fake.unique_username("ads_buyer"), get_settings().seed_password, "buyer"
        )
        _x(world)["ad_buyer"] = token
    return token


def _list(token: str | None, placement: str, category: str, limit: int) -> httpx.Response:
    return _call(
        token,
        "ListSponsoredSlots",
        {"placement": placement, "categoryId": category, "limit": limit},
    )


def _my_campaign(world: World) -> dict:
    st = _x(world)
    data = _ok(_call(st["ad_seller"], "ListMyAdCampaigns", {}))
    for campaign in data.get("campaigns") or []:
        if campaign.get("id") == st["ad_campaign"]:
            return campaign
    raise AssertionError(f"campaign {st['ad_campaign']} not in ListMyAdCampaigns: {data}")


@given("an ads seller with a published listing in a fresh category")
def ads_seller(world: World) -> None:
    token = _register_seller(world, "ads_seller")
    category = f"cat-ads-{uuid.uuid4().hex[:10]}"
    listing = Listing(
        title=f"[E2E][Ads] {uuid.uuid4().hex[:8]}",
        category_id=category,
        price=5_000_000,
        stock=100,
        status="published",
        description="Seed listing for the sponsored ads E2E.",
    )
    world.service_factory.listing.create_listing(listing)
    _x(world).update(ad_seller=token, ad_category=category, ad_listing=listing.listing_id)


@given(
    parsers.parse(
        "the seller has an ACTIVE ad campaign with bid {bid:d} and budget {budget:d} on that listing"
    )
)
def ads_campaign(world: World, bid: int, budget: int) -> None:
    st = _x(world)
    data = _ok(
        _call(
            st["ad_seller"],
            "CreateAdCampaign",
            {"listingId": st["ad_listing"], "budget": budget, "bid": bid},
        )
    )
    campaign = data.get("campaign") or {}
    assert campaign.get("id"), data
    assert campaign.get("status") == "AD_CAMPAIGN_STATUS_ACTIVE", campaign
    st["ad_campaign"] = campaign["id"]


@given("a buyer is served a sponsored slot for that listing in that category")
def buyer_served(world: World) -> None:
    st = _x(world)
    st["ad_resp"] = _list(_buyer_token(world), "search", st["ad_category"], 2)
    slot = _slot_for(world)
    assert slot and slot.get(
        "impressionToken"
    ), f"listing not served with a token: {st['ad_resp'].text}"
    st["ad_token"] = slot["impressionToken"]


@when(
    parsers.parse(
        'a buyer lists the sponsored slots for placement "{placement}" in that category with limit {limit:d}'
    )
)
def buyer_lists(world: World, placement: str, limit: int) -> None:
    st = _x(world)
    st["ad_resp"] = _list(_buyer_token(world), placement, st["ad_category"], limit)


@when(
    parsers.parse(
        'an anonymous caller lists the sponsored slots for placement "{placement}" with limit {limit:d}'
    )
)
def anon_lists(world: World, placement: str, limit: int) -> None:
    _x(world)["ad_resp"] = _list(None, placement, "", limit)


@when("the buyer records a click for the served token")
def buyer_clicks(world: World) -> None:
    st = _x(world)
    st["ad_click"] = _call(
        _buyer_token(world), "RecordAdClick", {"impressionToken": st["ad_token"]}
    )


@then("the slots contain that listing with a non-empty impression token")
def slots_contain(world: World) -> None:
    slot = _slot_for(world)
    assert (
        slot is not None
    ), f"listing {_x(world)['ad_listing']} not in slots: {_x(world)['ad_resp'].text}"
    assert slot.get("impressionToken"), slot
    assert slot.get("campaignId") == _x(world)["ad_campaign"], slot


@then(
    parsers.parse(
        "the slots hold 1 to {limit:d} entries that expose only the listing id, "
        "the campaign id and a non-empty impression token"
    )
)
def slots_bounded(world: World, limit: int) -> None:
    slots = _slots(_x(world)["ad_resp"])
    assert 1 <= len(slots) <= limit, f"expected 1..{limit} slots: {slots}"
    for slot in slots:
        assert set(slot) <= {"listingId", "campaignId", "impressionToken"}, slot
        assert slot.get("listingId") and slot.get("impressionToken"), slot


@then("the slots do not contain that listing")
def slots_absent(world: World) -> None:
    assert _slot_for(world) is None, f"listing still served: {_x(world)['ad_resp'].text}"


@then("the slots expose only the listing id, the campaign id and the impression token")
def slots_minimal(world: World) -> None:
    slot = _slot_for(world)
    assert slot is not None
    assert set(slot) <= {"listingId", "campaignId", "impressionToken"}, slot


@then(parsers.parse("the seller's campaign reads impressions {impressions:d} and spent {spent:d}"))
def campaign_impressions(world: World, impressions: int, spent: int) -> None:
    campaign = _my_campaign(world)
    assert int(campaign.get("impressions", 0)) == impressions, campaign
    assert int(campaign.get("spent", 0)) == spent, campaign


def _click(world: World, charged: bool) -> None:
    body = _ok(_x(world)["ad_click"])
    assert body.get("listingId") == _x(world)["ad_listing"], body
    assert bool(body.get("charged", False)) is charged, body


@then("the click is charged and names the listing")
def click_charged(world: World) -> None:
    _click(world, True)


@then("the click is not charged and names the listing")
def click_not_charged(world: World) -> None:
    _click(world, False)


@then(
    parsers.parse(
        'the seller\'s campaign reads spent {spent:d} and clicks {clicks:d} with status "{status}"'
    )
)
def campaign_spent(world: World, spent: int, clicks: int, status: str) -> None:
    campaign = _my_campaign(world)
    assert int(campaign.get("spent", 0)) == spent, campaign
    assert int(campaign.get("clicks", 0)) == clicks, campaign
    assert campaign.get("status") == status, campaign
