"""Extra shop-display-name steps (area c2): validation, spoofing, batch edge cases, UI lists.

Builds on shop_display_name_steps (seeding a named seller, second seller, cart).
"""

from __future__ import annotations

import re
import time
import uuid

import httpx
from playwright.sync_api import expect
from pytest_bdd import given, parsers, then, when

from config.settings import get_settings
from src.api.services import AuthService, ListingService
from src.constants import gateway_endpoints as ep
from src.constants import timeouts
from src.models import Listing
from tests.e2e.flows import login_via_api, scenario_buyer, stop_container
from tests.e2e.step_definitions.follow_seller_steps import _principal_id
from tests.e2e.support import oic_order_support as oic
from tests.e2e.support import pear_edge_support as pe
from tests.e2e.support.world import World

SETTINGS = get_settings()
CONTROL_NAME = "Tiem\u0007Hoa"
FOLLOW = "/platform.engagement.v1.EngagementService/FollowSeller"


def _seller_id(world: World) -> str:
    seller = world.state.seeded_seller
    assert seller and seller.token, "No seeded seller in state (needs @needsSeller)"
    return _principal_id(seller.token)


def _seller_token(world: World) -> str:
    return world.state.seeded_seller.token


def _upsert_raw(token: str, storefront: dict) -> httpx.Response:
    return httpx.post(
        f"{SETTINGS.gateway_url.rstrip('/')}{ep.LISTING_UPSERT_STOREFRONT}",
        json={"storefront": storefront},
        headers={"Authorization": f"Bearer {token}", "Content-Type": "application/json"},
        timeout=15,
    )


def _batch_raw(ids: list[str]) -> httpx.Response:
    return httpx.post(
        f"{SETTINGS.gateway_url.rstrip('/')}{ep.LISTING_BATCH_GET_STOREFRONTS}",
        json={"sellerIds": ids},
        headers={"Content-Type": "application/json"},
        timeout=15,
    )


def _assert_invalid(resp: httpx.Response) -> None:
    assert resp.status_code == httpx.codes.BAD_REQUEST, f"{resp.status_code}: {resp.text}"
    assert resp.json().get("code") == "invalid_argument", resp.text


# ── Invalid names ────────────────────────────────────────────────────────
@when(parsers.parse("the seller upserts a storefront whose display name is {n:d} characters long"))
def upsert_too_long(world: World, n: int) -> None:
    slug = world.state.extra["slug"]
    world.state.extra["invalid_resps"] = [
        _upsert_raw(_seller_token(world), {"slug": slug, "displayName": "x" * n})
    ]


@when("the seller upserts a storefront whose display name contains a control character")
def upsert_control_char(world: World) -> None:
    slug = world.state.extra["slug"]
    world.state.extra["invalid_resps"].append(
        _upsert_raw(_seller_token(world), {"slug": slug, "displayName": CONTROL_NAME})
    )


@then(
    parsers.parse('both upserts fail with invalid_argument and the stored name is still "{name}"')
)
def both_rejected_name_unchanged(world: World, name: str) -> None:
    for resp in world.state.extra["invalid_resps"]:
        _assert_invalid(resp)
    stored = ListingService().get_storefront(_seller_id(world))
    assert stored.get("displayName") == name, stored


# ── Spoofed seller id ────────────────────────────────────────────────────
@when(
    parsers.parse(
        "the first seller sends an UpsertStorefront naming the second seller with the "
        'display name "{name}"'
    )
)
def spoof_upsert(world: World, name: str) -> None:
    second_id = _principal_id(world.state.extra["second_seller_token"])
    resp = _upsert_raw(
        _seller_token(world),
        {"sellerId": second_id, "slug": world.state.extra["slug"], "displayName": name},
    )
    assert resp.status_code == httpx.codes.OK, f"{resp.status_code}: {resp.text}"
    world.state.extra["second_seller_id"] = second_id


@then(
    parsers.parse(
        'the first seller\'s storefront is named "{mine}" and the second seller\'s is still "{theirs}"'
    )
)
def spoof_had_no_effect(world: World, mine: str, theirs: str) -> None:
    svc = ListingService()
    assert svc.get_storefront(_seller_id(world)).get("displayName") == mine
    assert svc.get_storefront(world.state.extra["second_seller_id"]).get("displayName") == theirs


# ── Batch edge cases ─────────────────────────────────────────────────────
@when("BatchGetStorefronts is called with the seller's id and an id that has no storefront")
def batch_with_unknown(world: World) -> None:
    unknown = f"no-such-seller-{uuid.uuid4().hex[:8]}"
    world.state.extra["unknown_id"] = unknown
    world.state.extra["batch_resp"] = _batch_raw([_seller_id(world), unknown])


@then("the call succeeds with an entry for the seller only and none for the unknown id")
def batch_only_known(world: World) -> None:
    resp: httpx.Response = world.state.extra["batch_resp"]
    assert resp.status_code == httpx.codes.OK, resp.text
    ids = [s.get("sellerId") for s in resp.json().get("shops", [])]
    assert ids == [_seller_id(world)], ids


@when("BatchGetStorefronts is called with the seller's id three times")
def batch_with_duplicates(world: World) -> None:
    sid = _seller_id(world)
    world.state.extra["batch_resp"] = _batch_raw([sid, sid, sid])


@then("the call succeeds with a single entry for the seller")
def batch_single_entry(world: World) -> None:
    resp: httpx.Response = world.state.extra["batch_resp"]
    assert resp.status_code == httpx.codes.OK, resp.text
    ids = [s.get("sellerId") for s in resp.json().get("shops", [])]
    assert ids == [_seller_id(world)], ids


@when(parsers.parse("BatchGetStorefronts is called with {n:d} distinct ids"))
def batch_oversize(world: World, n: int) -> None:
    world.state.extra["batch_resp"] = _batch_raw(
        [f"seller-{i}-{uuid.uuid4().hex[:6]}" for i in range(n)]
    )


@then("the call fails with invalid_argument")
def batch_invalid(world: World) -> None:
    _assert_invalid(world.state.extra["batch_resp"])


# ── Following list ───────────────────────────────────────────────────────
@given("a buyer follows three sellers, two with a display name and one without")
def buyer_follows_three(world: World) -> None:
    auth = AuthService()
    followed: list[tuple[str, str | None]] = []
    for name in ("Shop Alpha", "Shop Beta", None):
        token = auth.register(
            f"e2e_fseller_{uuid.uuid4().hex[:10]}", SETTINGS.seed_password, "seller"
        )
        if name:
            ListingService(token=token).upsert_storefront(f"e2e-shop-{uuid.uuid4().hex[:10]}", name)
        followed.append((_principal_id(token), name))
    buyer = scenario_buyer(world)
    login_via_api(world, buyer)
    from src.api.services.base_service import BaseService

    api = BaseService(token=buyer.token)
    for seller_id, _ in followed:
        api.post(FOLLOW, {"sellerId": seller_id})
    world.state.extra["followed"] = followed


@when("the buyer opens the following page")
def open_following(world: World) -> None:
    world.page.goto(f"{world.settings.base_url}/account/following", wait_until="domcontentloaded")


@then('each followed shop row shows its display name or the "Shop #" fallback')
def following_rows(world: World) -> None:
    for seller_id, name in world.state.extra["followed"]:
        link = world.page.locator(f'a[href="/shop/{seller_id}"]').first
        expect(link).to_be_visible(timeout=timeouts.NAVIGATION)
        expected = name or f"Shop #{seller_id[:6]}"
        expect(link).to_contain_text(expected, timeout=timeouts.DEFAULT)
    # the display-named rows never fall back to the id label
    names = [n for _, n in world.state.extra["followed"] if n]
    assert len(names) == 2


# ── Product detail ───────────────────────────────────────────────────────
@given("the seller has a published listing")
def seller_has_listing(world: World) -> None:
    listing = Listing(
        title=f"[E2E] Shop header listing {uuid.uuid4().hex[:6]}",
        category_id="cat-electronics",
        price=1_000_000,
        stock=5,
        status="published",
        description="Seed for the PDP shop header e2e.",
    )
    ListingService(token=_seller_token(world)).create_listing(listing)
    world.state.extra["pdp_listing_id"] = listing.listing_id


@when("a visitor opens that listing")
def visitor_opens_listing(world: World) -> None:
    world.page.goto(
        f"{world.settings.base_url}/listing/{world.state.extra['pdp_listing_id']}",
        wait_until="domcontentloaded",
    )


@then(parsers.parse('the product page shop header shows "{name}" and links to the seller\'s shop'))
def pdp_shop_header(world: World, name: str) -> None:
    card = world.page.get_by_test_id("shop-header-card")
    expect(card).to_be_visible(timeout=timeouts.NAVIGATION)
    expect(card.get_by_test_id("shop-name")).to_have_text(name, timeout=timeouts.DEFAULT)
    link = card.get_by_role("link", name=re.compile("Xem Shop"))
    expect(link).to_have_attribute("href", f"/shop/{_seller_id(world)}")


# ── Lookup failure (destructive) ─────────────────────────────────────────
@when("team-domain is stopped so the batch lookup fails")
def stop_domain(world: World) -> None:
    name = oic.domain_container()
    restore = stop_container(name)

    def _restore() -> None:
        restore()
        pe.wait_healthy(name)
        deadline = time.monotonic() + 60
        while time.monotonic() < deadline:
            try:
                if ListingService().get_storefront(_seller_id(world)) is not None:
                    return
            except Exception:  # noqa: BLE001 - still coming back up
                time.sleep(1)

    world.add_cleanup(_restore)


@then('the cart still renders both items with the "Shop #" fallback header of each seller')
def cart_fallback_headers(world: World) -> None:
    page = world.page
    for seller_id in world.state.extra["cart_seller_ids"]:
        header = page.get_by_test_id("cart-shop-group").get_by_role(
            "link", name=f"Shop #{seller_id[:6]}", exact=True
        )
        expect(header).to_be_visible(timeout=timeouts.NAVIGATION)
    expect(page.get_by_test_id("cart-shop-group")).to_have_count(2)
