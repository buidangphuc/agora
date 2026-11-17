"""Storefront customisation steps (OpenSpec change `domain-storefront-customisation`).

Everything goes through the real gateway: seller tokens, listings created via API and
the banner uploaded through the presigned URL returned by GetImageUploadUrl.
"""

from __future__ import annotations

import uuid
from typing import Any

import httpx
from pytest_bdd import given, parsers, then, when

from src.api.services import ListingService
from src.api.services.base_service import GatewayError
from src.models import Listing
from tests.e2e.step_definitions.follow_seller_steps import _principal_id
from tests.e2e.support.world import World

_PNG = (
    b"\x89PNG\r\n\x1a\n\x00\x00\x00\rIHDR\x00\x00\x00\x01\x00\x00\x00\x01\x08\x06\x00\x00\x00"
    b"\x1f\x15\xc4\x89\x00\x00\x00\rIDATx\x9cc\xf8\xff\xff?\x00\x05\xfe\x02\xfe\xa7\x9a\xa0\xa0"
    b"\x00\x00\x00\x00IEND\xaeB`\x82"
)
_LETTERS = "ABCDEF"


def _token(world: World) -> str:
    seller = world.state.seeded_seller
    assert seller and seller.token, "No seeded seller in state (needs @needsSeller)"
    return seller.token


def _svc(world: World) -> ListingService:
    return ListingService(token=_token(world))


def _seller_id(world: World) -> str:
    return _principal_id(_token(world))


def _slug(world: World) -> str:
    return world.state.extra.setdefault("slug", f"e2e-sf-{uuid.uuid4().hex[:10]}")


def _ids(world: World, letters: str) -> list[str]:
    by_letter = world.state.extra["listing_by_letter"]
    return [by_letter[c.strip()] for c in letters.split(",")]


def _create_listing(token: str, title: str) -> str:
    listing = Listing(
        title=title,
        category_id="cat-electronics",
        price=1_000_000,
        stock=10,
        status="published",
        description="Seed for storefront-customisation e2e.",
    )
    return ListingService(token=token).create_listing(listing)


def _upload_banner(world: World) -> str:
    uploaded = _svc(world).get_image_upload_url("image/png", "banner.png")
    resp = httpx.put(
        uploaded["uploadUrl"], content=_PNG, headers={"Content-Type": "image/png"}, timeout=30
    )
    assert resp.status_code < 300, f"banner upload failed: {resp.status_code} {resp.text[:200]}"
    world.state.extra["banner_url"] = uploaded["publicUrl"]
    return uploaded["publicUrl"]


def _save(world: World, **fields: Any) -> dict[str, Any]:
    body = {"slug": _slug(world), **fields}
    return _svc(world).upsert_full_storefront(body)


def _stored(world: World) -> dict[str, Any]:
    return ListingService().get_storefront(_seller_id(world))


@given(parsers.parse("the seller owns {n:d} published listings"))
def seller_owns_listings(world: World, n: int) -> None:
    token = _token(world)
    world.state.extra["listing_by_letter"] = {
        _LETTERS[i]: _create_listing(token, f"[E2E] Storefront {_LETTERS[i]}") for i in range(n)
    }


@given("a second seller owns a published listing")
def second_seller_listing(world: World) -> None:
    from config.settings import get_settings
    from src.utils import data as fake

    auth = world.service_factory.auth
    token = auth.register(fake.unique_username("seller"), get_settings().seed_password, "seller")
    world.state.extra["foreign_listing"] = _create_listing(token, "[E2E] Foreign listing")
    world.state.extra["foreign_seller_id"] = _principal_id(token)


@given(parsers.parse('the seller features the listings in the order "{order}"'))
@when(parsers.parse('the seller features the listings in the order "{order}"'))
def seller_features(world: World, order: str) -> None:
    _save(world, featuredListingIds=_ids(world, order))


@given(
    parsers.parse(
        "the seller has a stored storefront with an uploaded banner, featured listings "
        '"{order}", tagline "{tagline}" and theme "{theme}"'
    )
)
def seller_has_full_storefront(world: World, order: str, tagline: str, theme: str) -> None:
    banner = _upload_banner(world)
    _save(
        world,
        bannerUrl=banner,
        featuredListingIds=_ids(world, order),
        tagline=tagline,
        theme=theme,
    )


@when("the seller uploads a banner image through the image bucket")
def seller_uploads_banner(world: World) -> None:
    _upload_banner(world)


@when("the seller saves the storefront with the uploaded banner")
def seller_saves_banner(world: World) -> None:
    _save(world, bannerUrl=world.state.extra["banner_url"])


@when("the seller features the second seller's listing")
def seller_features_foreign(world: World) -> None:
    mine = _ids(world, "A,B")
    try:
        _save(world, featuredListingIds=[*mine, world.state.extra["foreign_listing"]])
        world.state.extra["upsert_error"] = None
    except GatewayError as exc:
        world.state.extra["upsert_error"] = exc


@when(parsers.parse('the seller moves listing "{letter}" to draft'))
def seller_moves_to_draft(world: World, letter: str) -> None:
    _svc(world).set_listing_status(_ids(world, letter)[0], "draft")


@when(parsers.parse('the seller resends the same storefront with the tagline "{tagline}"'))
def seller_resends_with_tagline(world: World, tagline: str) -> None:
    current = _stored(world)
    current["tagline"] = tagline
    _svc(world).upsert_full_storefront(current)


@then("the storefront API returns the uploaded banner url")
def api_returns_banner(world: World) -> None:
    assert _stored(world).get("bannerUrl") == world.state.extra["banner_url"]


@then(parsers.parse('the storefront API returns the featured listings in the order "{order}"'))
@then(
    parsers.parse('the storefront API still returns the featured listings in the order "{order}"')
)
@then(
    parsers.parse(
        'an anonymous buyer reading the storefront still gets the featured listings in the order "{order}"'
    )
)
def api_returns_featured(world: World, order: str) -> None:
    assert _stored(world).get("featuredListingIds") == _ids(world, order)


@then("the upsert fails with invalid_argument without naming the owner")
def upsert_invalid_argument(world: World) -> None:
    err = world.state.extra["upsert_error"]
    assert err is not None, "upsert of a foreign listing unexpectedly succeeded"
    assert "invalid_argument" in err.body.lower(), err.body
    assert world.state.extra["foreign_seller_id"] not in err.body, err.body


@then(
    parsers.parse(
        'the storefront API returns tagline "{tagline}", the same banner, featured listings '
        '"{order}" and theme "{theme}"'
    )
)
def api_returns_all(world: World, tagline: str, order: str, theme: str) -> None:
    got = _stored(world)
    assert got.get("tagline") == tagline, got
    assert got.get("bannerUrl") == world.state.extra["banner_url"], got
    assert got.get("featuredListingIds") == _ids(world, order), got
    assert got.get("theme") == theme, got
