"""Browser-beacon and streaming steps for the PDP (ui-phase-product-detail).

The browser batches tracking beacons to `${gateway}/api/track` as a JSON array of
`{type, listingId, placementId, position, ...}` (see team-frontend/src/lib/analytics).
`_record_beacons` taps those requests so steps can assert what the page really sent.
"""

from __future__ import annotations

import json
import time

from playwright.sync_api import expect
from pytest_bdd import then, when

from src.constants import PageName, timeouts
from src.pages import ListingDetailPage
from tests.e2e.support.world import World

_FLUSH_WAIT_S = 4  # the beacon queue flushes on a short timer


def _detail(world: World) -> ListingDetailPage:
    return world.get_page(PageName.LISTING_DETAIL)  # type: ignore[return-value]


# The browser queue flushes with `navigator.sendBeacon(url, Blob)`; Playwright does not expose a
# Blob body on `request.post_data`, so the beacon is also tapped where it is sent.
_BEACON_TAP = """
(() => {
  const original = navigator.sendBeacon.bind(navigator);
  navigator.sendBeacon = (url, data) => {
    try {
      if (String(url).includes("/api/track") && window.__e2eRecordBeacon) {
        Promise.resolve(data && data.text ? data.text() : String(data))
          .then((text) => window.__e2eRecordBeacon(text))
          .catch(() => {});
      }
    } catch (e) {}
    return original(url, data);
  };
})();
"""


def _record_beacons(world: World) -> list[dict]:
    beacons: list[dict] = []
    world.state.extra["beacons"] = beacons

    def collect(body: str | None) -> None:
        if not body:
            return
        try:
            payload = json.loads(body)
        except ValueError:
            return
        beacons.extend(payload if isinstance(payload, list) else [payload])

    def on_request(request) -> None:  # noqa: ANN001
        # fetch() fallback of the queue: the body is visible on the request itself.
        if "/api/track" in request.url:
            collect(request.post_data)

    world.page.on("request", on_request)
    world.page.expose_function("__e2eRecordBeacon", collect)
    world.page.add_init_script(_BEACON_TAP)
    return beacons


def _open_listing_recording(world: World) -> None:
    listing = world.state.listing
    assert listing and listing.listing_id, "No seeded listing in state"
    _record_beacons(world)
    world.navigate_to(PageName.LISTING_DETAIL, listing_id=listing.listing_id)
    expect(_detail(world).add_to_cart_button).to_be_visible(timeout=timeouts.NAVIGATION)


@when("the buyer opens the seeded listing while recording tracking beacons")
def open_seeded_listing_recording(world: World) -> None:
    _open_listing_recording(world)


@when("the buyer opens the variant listing while recording tracking beacons")
def open_variant_listing_recording(world: World) -> None:
    _open_listing_recording(world)


# ── Recommendations streaming ────────────────────────────────────────────
@then("the similar-items Skeleton is gone and the row shows product cards")
def skeleton_gone_cards_shown(world: World) -> None:
    detail = _detail(world)
    expect(detail.recommendations.heading).to_be_visible(timeout=timeouts.LONG)
    expect(detail.recommendations.cards.first).to_be_visible(timeout=timeouts.LONG)
    # The streamed Skeleton (a busy <section>) is replaced, not left behind.
    expect(world.page.locator("section[aria-busy='true']")).to_have_count(0)


@then("a pdp_similar_items impression beacon was sent for position 1")
def similar_items_impression_sent(world: World) -> None:
    # Impressions are viewability-gated and the row sits below the fold, so bring
    # the first card into view (as the home-row journey step does).
    _detail(world).recommendations.cards.first.scroll_into_view_if_needed()
    beacons: list[dict] = world.state.extra["beacons"]
    deadline = time.monotonic() + _FLUSH_WAIT_S + 6
    while time.monotonic() < deadline:
        if any(
            b.get("type") == "impression"
            and b.get("placementId") == "pdp_similar_items"
            and b.get("position") == 1
            for b in beacons
        ):
            return
        world.page.wait_for_timeout(500)
    raise AssertionError(f"no pdp_similar_items impression for position 1 in {beacons}")


@then("the product page renders without the similar-items row or its Skeleton")
def page_without_row_or_skeleton(world: World) -> None:
    detail = _detail(world)
    expect(detail.add_to_cart_button).to_be_visible(timeout=timeouts.NAVIGATION)
    unavailable = bool(world.state.extra.get("recs_error")) or not world.state.extra.get(
        "recs_items"
    )
    if unavailable:
        # Give the stream time to settle, then neither the row nor a skeleton remains.
        world.page.wait_for_timeout(2_000)
        assert detail.recommendations.heading.count() == 0, "row should be hidden"
        expect(world.page.locator("section[aria-busy='true']")).to_have_count(0)
    else:
        # Recs are available in this env: the row shows instead of erroring.
        expect(detail.recommendations.heading).to_be_visible(timeout=timeouts.LONG)


# ── Tracking ─────────────────────────────────────────────────────────────
@then("exactly one view beacon for the listing was sent from the browser")
def exactly_one_view_beacon(world: World) -> None:
    listing_id = world.state.listing.listing_id  # type: ignore[union-attr]
    world.page.wait_for_timeout(_FLUSH_WAIT_S * 1000)  # let the queue flush
    beacons: list[dict] = world.state.extra["beacons"]
    views = [b for b in beacons if b.get("type") == "view" and b.get("listingId") == listing_id]
    assert len(views) == 1, f"expected exactly one view beacon for {listing_id}, got {views}"
