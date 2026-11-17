"""Steps for frontend/seller_shop_customisation_ui.feature (seller-shop-customisation-ui, initiative seller-centre).

The seller works in the browser on the seller app; listings are seeded through the gateway and the
persisted storefront is read back through GetStorefront. The buyer page is read on the buyer app.
"""

from __future__ import annotations

import tempfile
import uuid
from pathlib import Path

from playwright.sync_api import expect
from pytest_bdd import given, parsers, then, when

from src.api.services import ListingService
from src.api.services.base_service import GatewayError
from src.constants import PageName, timeouts
from src.models import Listing
from src.pages import SellerShopPage
from tests.e2e.step_definitions.follow_seller_steps import _principal_id
from tests.e2e.support.world import World

_PNG = (
    b"\x89PNG\r\n\x1a\n\x00\x00\x00\rIHDR\x00\x00\x00\x01\x00\x00\x00\x01\x08\x06\x00\x00\x00"
    b"\x1f\x15\xc4\x89\x00\x00\x00\rIDATx\x9cc\xf8\xff\xff?\x00\x05\xfe\x02\xfe\xa7\x9a\xa0\xa0"
    b"\x00\x00\x00\x00IEND\xaeB`\x82"
)
DESKTOP = {"width": 1280, "height": 720}


def _x(world: World) -> dict:
    return world.state.extra.setdefault("ssc", {})


def _token(world: World) -> str:
    seller = world.state.seeded_seller
    assert seller and seller.token, "No seeded seller in state (needs @needsSeller)"
    return seller.token


def _seller_id(world: World) -> str:
    return _principal_id(_token(world))


def _create(token: str, title: str) -> str:
    return ListingService(token=token).create_listing(
        Listing(
            title=title,
            category_id="cat-electronics",
            price=1_000_000,
            stock=10,
            status="published",
            description="Seed for seller-shop-customisation-ui e2e.",
        )
    )


def _stored(world: World) -> dict:
    return ListingService().get_storefront(_seller_id(world))


def _shop(world: World) -> SellerShopPage:
    return world.get_page(PageName.SELLER_SHOP)  # type: ignore[return-value]


def _open_form(world: World) -> SellerShopPage:
    world.page.set_viewport_size(DESKTOP)
    page: SellerShopPage = world.navigate_to(PageName.SELLER_SHOP)  # type: ignore[assignment]
    expect(page.tagline_input).to_be_visible(timeout=timeouts.NAVIGATION)
    page.wait_until_interactive(page.save_button)
    return page


@given("the seller owns two published listings for the shop")
def seller_two_listings(world: World) -> None:
    tag = uuid.uuid4().hex[:8]
    titles = [f"[E2E] Hoa {tag} Alpha", f"[E2E] Hoa {tag} Beta"]
    ids = [_create(_token(world), t) for t in titles]
    _x(world).update(titles=titles, ids=ids, tag=tag)


@given(
    parsers.parse(
        'the seller has saved a storefront featuring both listings with tagline "{tagline}"'
    )
)
def seller_saved_storefront(world: World, tagline: str) -> None:
    x = _x(world)
    ListingService(token=_token(world)).upsert_full_storefront(
        {
            "slug": f"e2e-ssc-{uuid.uuid4().hex[:10]}",
            "displayName": "Tiệm hoa e2e",
            "tagline": tagline,
            "featuredListingIds": x["ids"],
            "theme": "default",
        }
    )


@given("a second seller owns a published listing for the shop")
def second_seller_listing(world: World) -> None:
    from config.settings import get_settings
    from src.utils import data as fake

    auth = world.service_factory.auth
    token = auth.register(fake.unique_username("seller"), get_settings().seed_password, "seller")
    title = f"[E2E] Foreign {uuid.uuid4().hex[:8]}"
    _x(world).update(foreign_id=_create(token, title), foreign_title=title)


@when(
    parsers.parse(
        'the seller brands the shop in the browser with a banner, tagline "{tagline}", '
        'both listings in order and theme "{theme}"'
    )
)
def brand_shop(world: World, tagline: str, theme: str) -> None:
    x = _x(world)
    page = _open_form(world)
    banner = Path(tempfile.mkdtemp()) / "banner.png"
    banner.write_bytes(_PNG)
    page.banner_file_input.set_input_files(str(banner))
    expect(page.banner_preview).to_be_visible(timeout=timeouts.NAVIGATION)
    if not page.name_input.input_value().strip():
        page.name_input.fill(f"Tiệm hoa {x['tag']}")
    page.tagline_input.fill(tagline)
    for title in x["titles"]:
        page.picker_search.fill(title)
        add = page.add_button_for(title)
        expect(add).to_be_visible(timeout=timeouts.NAVIGATION)
        add.click()
    page.theme_select.select_option(theme)
    page.save_button.click()


@then("a success toast confirms the shop information")
def info_toast(world: World) -> None:
    expect(_shop(world).saved_toast).to_be_visible(timeout=timeouts.NAVIGATION)


@when("the seller reloads the shop form")
def reload_form(world: World) -> None:
    _open_form(world)


@then(
    parsers.parse(
        'the shop form shows the same banner, tagline "{tagline}", both listings in the chosen '
        'order and theme "{theme}"'
    )
)
def form_shows(world: World, tagline: str, theme: str) -> None:
    x = _x(world)
    page = _shop(world)
    expect(page.banner_preview).to_be_visible(timeout=timeouts.NAVIGATION)
    expect(page.tagline_input).to_have_value(tagline)
    expect(page.theme_select).to_have_value(theme)
    selected = page.selected_titles()
    expect(selected).to_have_count(len(x["titles"]), timeout=timeouts.NAVIGATION)
    for i, title in enumerate(x["titles"]):
        expect(selected.nth(i)).to_contain_text(title)


@then(
    parsers.parse(
        'the storefront API returns the banner, tagline "{tagline}", both listings in the chosen '
        'order and theme "{theme}"'
    )
)
def api_returns(world: World, tagline: str, theme: str) -> None:
    got = _stored(world)
    assert got.get("bannerUrl"), got
    assert got.get("tagline") == tagline, got
    assert got.get("featuredListingIds") == _x(world)["ids"], got
    assert got.get("theme") == theme, got


@then(parsers.parse('the buyer app shop page shows the tagline "{tagline}"'))
def buyer_sees_tagline(world: World, tagline: str) -> None:
    world.navigate_to(PageName.SHOP_PROFILE, shop_id=_seller_id(world))
    expect(world.page.get_by_text(tagline, exact=False).first).to_be_visible(
        timeout=timeouts.NAVIGATION
    )


@when("the seller saves a storefront featuring the second seller's listing through the gateway")
def save_foreign(world: World) -> None:
    x = _x(world)
    try:
        ListingService(token=_token(world)).upsert_full_storefront(
            {
                "slug": f"e2e-ssc-{uuid.uuid4().hex[:10]}",
                "displayName": "Tiệm hoa e2e",
                "tagline": "Bị từ chối",
                "featuredListingIds": [*x["ids"], x["foreign_id"]],
                "theme": "default",
            }
        )
        x["error"] = None
    except GatewayError as exc:
        x["error"] = exc


@then("the save fails with invalid_argument")
def save_fails(world: World) -> None:
    err = _x(world)["error"]
    assert err is not None, "saving a foreign featured listing unexpectedly succeeded"
    assert "invalid_argument" in err.body.lower(), err.body


@then(
    parsers.parse(
        'the storefront API still returns tagline "{tagline}" and both listings in the original order'
    )
)
def storefront_unchanged(world: World, tagline: str) -> None:
    got = _stored(world)
    assert got.get("tagline") == tagline, got
    assert got.get("featuredListingIds") == _x(world)["ids"], got


@then("the buyer app shop page shows no listing of the second seller")
def buyer_no_foreign(world: World) -> None:
    x = _x(world)
    world.navigate_to(PageName.SHOP_PROFILE, shop_id=_seller_id(world))
    expect(world.page.get_by_text(x["titles"][0], exact=False).first).to_be_visible(
        timeout=timeouts.NAVIGATION
    )
    expect(world.page.get_by_text(x["foreign_title"], exact=False)).to_have_count(0)
