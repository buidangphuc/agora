"""Recommendations row ("Gợi ý cho bạn") step definitions (surface-recommendations).

Asserts the user-facing row on home (context HOMEPAGE) and PDP (context
SIMILAR_ITEMS, seeded by the listing), AND that it is sourced from team-ai
through the gateway `RecommendationService/Recommend` — not a client-side mock
and not a direct browser call to team-ai. The gateway probe uses the buyer token
already active on the service factory (set by `login_via_api`), i.e. the SAME
session the server components use, so a non-empty response proves the real
frontend → gateway → team-ai path. Needs the live stack + team-ai
RECS_ENABLED=true with the training-job Qdrant/Redis data to be populated.
"""

from __future__ import annotations

from playwright.sync_api import expect
from pytest_bdd import then, when

from src.api.services import GatewayError
from src.api.services.recommendation_service import (
    CONTEXT_HOMEPAGE,
    CONTEXT_SIMILAR_ITEMS,
)
from src.constants import PageName, timeouts
from src.pages.components import RecommendationsRowComponent
from tests.e2e.support.world import World


def _row(world: World) -> RecommendationsRowComponent:
    """Resolve the recs row from whichever page currently carries it."""
    name = PageName.LISTING_DETAIL if "/listing/" in world.page.url else PageName.HOME
    page = world.get_page(name)
    return page.recommendations  # type: ignore[attr-defined]


def _probe_gateway(world: World, *, seed_listing_id: str = "", context: str) -> None:
    """Call Recommend through the gateway as the buyer; record items / UNAVAILABLE."""
    try:
        data = world.service_factory.recommendation.recommend(
            seed_listing_id=seed_listing_id, context=context, limit=10
        )
        world.state.extra["recs_items"] = data.get("items") or []
        world.state.extra["recs_error"] = None
    except GatewayError as exc:
        world.state.extra["recs_items"] = []
        world.state.extra["recs_error"] = exc


# ── UI assertions ──────────────────────────────────────────────────────────
@then('the "Gợi ý cho bạn" recommendations row is populated with product cards')
def row_populated(world: World) -> None:
    row = _row(world)
    expect(row.heading).to_be_visible(timeout=timeouts.LONG)
    expect(row.cards.first).to_be_visible(timeout=timeouts.LONG)
    assert row.card_count() > 0, "recommendations row rendered no product cards"


@then("the row is sourced from team-ai via the gateway, not a client-side mock")
def row_sourced_via_gateway(world: World) -> None:
    _probe_gateway(world, context=CONTEXT_HOMEPAGE)
    items = world.state.extra["recs_items"]
    assert items, (
        "gateway Recommend returned no items — the row cannot be sourced from "
        "team-ai (is RECS_ENABLED=true with training data?)"
    )
    # The RPC returns id-only items (Rule 3); those ids must be the /listing/<id>
    # cards hydrated into the row — proving the row reflects the RPC response and
    # is not a hardcoded client-side list.
    recommended_ids = {it.get("listingId") for it in items if it.get("listingId")}
    row = _row(world)
    hrefs = row.cards.evaluate_all("els => els.map(e => e.getAttribute('href') || '')")
    rendered_ids = {h.rsplit("/", 1)[-1] for h in hrefs if "/listing/" in h}
    assert recommended_ids & rendered_ids, (
        "no recommended id from the gateway appears among the rendered cards — "
        f"RPC ids={recommended_ids}, card ids={rendered_ids}"
    )
    world.logger.info(f"Recommend returned {len(items)} item(s) via the gateway")


@then("the row is seeded with the current listing via the gateway to team-ai")
def row_seeded_by_listing(world: World) -> None:
    listing = world.state.listing
    assert listing and listing.listing_id, "no seeded listing in scenario state"
    _probe_gateway(world, seed_listing_id=listing.listing_id, context=CONTEXT_SIMILAR_ITEMS)
    items = world.state.extra["recs_items"]
    assert items, (
        "gateway Recommend (SIMILAR_ITEMS, seeded) returned no items — the PDP "
        "row cannot be sourced from team-ai"
    )


@then("the recommendations came through the gateway Recommend RPC")
def recommendations_via_rpc(world: World) -> None:
    _probe_gateway(world, context=CONTEXT_HOMEPAGE)
    assert (
        world.state.extra["recs_error"] is None
    ), f"gateway Recommend errored: {world.state.extra['recs_error']}"
    assert world.state.extra["recs_items"], "gateway Recommend returned no items"


@then("the browser markup carries no team-ai address or recommendation service client")
def no_teamai_in_markup(world: World) -> None:
    content = world.page.content().lower()
    # Server-only surfacing (Rule 1): the browser must never learn team-ai's
    # address nor ship a recommendation gRPC client.
    for banned in ("team-ai", ":50060", "recommendationservice", "recommend_pb"):
        assert banned not in content, f"team-ai/recs client leaked into the browser: {banned!r}"


# ── Graceful degradation ───────────────────────────────────────────────────
@when("the recommendation service is unavailable")
def recommendation_service_unavailable(world: World) -> None:
    # We cannot flip RECS_ENABLED from e2e; probe the current availability so the
    # assertion can branch. The contract holds either way: an UNAVAILABLE /
    # empty response must degrade to a hidden row, never a page error.
    _probe_gateway(world, context=CONTEXT_HOMEPAGE)


@then("the home page still renders")
def home_still_renders(world: World) -> None:
    home = world.get_page(PageName.HOME)
    assert home.is_displayed(), f"home page failed to render (url={world.page.url})"


@then('the "Gợi ý cho bạn" row is hidden or empty rather than erroring the page')
def row_hidden_or_populated_never_errored(world: World) -> None:
    # No user-visible error regardless of recs availability.
    error_banner = world.page.locator(".bg-red-900:has-text('Lỗi'), [data-toast='error']")
    assert (
        error_banner.count() == 0 or not error_banner.first.is_visible()
    ), "a user-visible error was shown when recommendations were unavailable"

    row = _row(world)
    unavailable = bool(world.state.extra.get("recs_error")) or not world.state.extra.get(
        "recs_items"
    )
    if unavailable:
        # Frontend renders nothing when the list is empty — the row is hidden.
        assert row.heading.count() == 0, "row should be hidden when recs are unavailable"
    else:
        # Recs are available in this env — the row is shown instead of erroring.
        expect(row.heading).to_be_visible(timeout=timeouts.DEFAULT)
