"""Steps for recommendations/cart_cross_sell.feature (change recsys-cart-cross-sell), through the gateway.

Black box: the catalogue is seeded in the serving Qdrant collection and Redis popular list with
`visible=true` on every candidate (see support/cart_cross_sell_support.py); answers are read through
the gateway as the logged-in buyer and compared with Qdrant's own similarity scores.
"""

from __future__ import annotations

import json
import time
import uuid
from typing import Any

from playwright.sync_api import expect
from pytest_bdd import given, parsers, then, when

from src.api.services import GatewayError
from src.api.services.recommendation_service import (
    CONTEXT_CART,
    CONTEXT_HOMEPAGE,
    CONTEXT_SIMILAR_ITEMS,
)
from src.constants import PageName, timeouts
from src.pages import CartPage
from tests.e2e.step_definitions.pdp_streaming_steps import _record_beacons
from tests.e2e.support import cart_cross_sell_support as cx
from tests.e2e.support import cart_selection_support as cs
from tests.e2e.support import d2_support as d2
from tests.e2e.support import rss_support as rss
from tests.e2e.support.world import World

SCORE_TOLERANCE = 1e-3
MAX_CONTEXT = 50


def _std(world: World) -> cx.Standard:
    return cx.state(world)["std"]


def _answer(world: World) -> dict[str, Any]:
    return cx.state(world)["answer"]


def _ask(
    world: World,
    payload: dict[str, Any],
    accept,
    what: str,
) -> None:
    c = cx.state(world)
    c["payload"] = payload
    c["answer"] = cx.recommend_until(world, payload, accept, what)


def _cart_payload(context: list[str], **extra: Any) -> dict[str, Any]:
    return {"context": CONTEXT_CART, "contextListingIds": context, "limit": 20, **extra}


def _unknown(count: int) -> list[str]:
    return [f"cx-unknown-{uuid.uuid4().hex[:10]}-{n}" for n in range(count)]


@given(
    "a cross-sell catalogue where cart listings A and B each have ten similar listings from ten different sellers"
)
def cx_catalogue(world: World) -> None:
    cx.state(world)["std"] = cx.Standard(cx.Catalogue(world))


@given(
    "the global popular list holds one hidden listing with the top score and three visible listings of three sellers"
)
def cx_popular_with_hidden(world: World) -> None:
    run = uuid.uuid4().hex[:8]
    hidden = f"cx-{run}-hidden"
    visible = [f"cx-{run}-vis{n}" for n in range(3)]
    entries = [
        {
            "listing_id": hidden,
            "score": 100.0,
            "seller_id": f"cx-{run}-seller-hidden",
            "category_id": f"cx-{run}-cat",
            "visible": False,
        }
    ] + [
        {
            "listing_id": lid,
            "score": 90.0 - n,
            "seller_id": f"cx-{run}-seller-{n}",
            "category_id": f"cx-{run}-cat",
            "visible": True,
        }
        for n, lid in enumerate(visible)
    ]
    redis = rss.keys(world, rss.SERVING_DB)
    redis.put(f"{rss.serving_prefix(redis)}:popular", json.dumps(entries))
    cx.state(world).update(hidden=hidden, visible_popular=set(visible))


@given("every listing similar to A or B is hidden")
def cx_similar_hidden(world: World) -> None:
    std = _std(world)
    std.cat.patch(std.a_cluster + std.b_cluster, visible=False)


@given("the most similar listing of A is a cart line and the second most similar is out of stock")
def cx_second_out_of_stock(world: World) -> None:
    std = _std(world)
    std.cat.patch([std.a_cluster[1]], in_stock=False)
    cx.state(world).update(cart_line=std.a_cluster[0], out_of_stock=std.a_cluster[1])


@given("the three most similar listings of A belong to one seller")
def cx_three_one_seller(world: World) -> None:
    std = _std(world)
    shared = f"cx-{std.cat.run}-seller-shared"
    trio = std.a_cluster[:3]
    std.cat.patch(trio, seller_id=shared)
    cx.state(world)["trio"] = trio


@when(
    "Recommend for cart_cross_sell is called through the gateway with the context listings A and B"
)
def cx_ask_a_b(world: World) -> None:
    std = _std(world)
    contexts = [std.a, std.b]
    _ask(
        world,
        _cart_payload(contexts),
        cx.accepts(std.similar_allowed(contexts)),
        "cart_cross_sell with context A and B",
    )
    cx.state(world)["contexts"] = contexts


@when("Recommend for cart_cross_sell is called through the gateway with the context listing A")
def cx_ask_a(world: World) -> None:
    std = _std(world)
    contexts = [std.a]
    _ask(
        world,
        _cart_payload(contexts),
        cx.accepts(std.similar_allowed(contexts)),
        "cart_cross_sell with context A",
    )
    cx.state(world)["contexts"] = contexts


@when(
    "Recommend for cart_cross_sell is called through the gateway with the context listing A and 49 listings unknown to the index"
)
def cx_ask_fifty(world: World) -> None:
    std = _std(world)
    contexts = [std.a, *_unknown(MAX_CONTEXT - 1)]
    assert len(contexts) == MAX_CONTEXT
    _ask(
        world,
        _cart_payload(contexts),
        cx.accepts(std.similar_allowed([std.a])),
        "cart_cross_sell with exactly 50 context ids",
    )
    cx.state(world)["contexts"] = [std.a]


@when(
    "Recommend for cart_cross_sell is called through the gateway with the context listing A and one listing unknown to the index"
)
def cx_ask_unknown(world: World) -> None:
    std = _std(world)
    contexts = [std.a, *_unknown(1)]
    _ask(
        world,
        _cart_payload(contexts),
        cx.accepts(std.similar_allowed([std.a])),
        "cart_cross_sell with an unknown context listing",
    )
    cx.state(world)["contexts"] = [std.a]


@when(
    "Recommend for cart_cross_sell is called through the gateway with the context listings A and its most similar listing"
)
def cx_ask_a_and_nearest(world: World) -> None:
    std = _std(world)
    contexts = [std.a, cx.state(world)["cart_line"]]
    allowed = std.similar_allowed(contexts) - {cx.state(world)["out_of_stock"]}
    _ask(world, _cart_payload(contexts), cx.accepts(allowed), "cart_cross_sell with cart lines")
    cx.state(world)["contexts"] = contexts


@when(
    "Recommend for cart_cross_sell is called through the gateway with the context listings A and B whose similar listings are all hidden"
)
def cx_ask_categories(world: World) -> None:
    std = _std(world)
    allowed = std.popular_ids(std.category_c, std.category_d)
    _ask(
        world,
        _cart_payload([std.a, std.b]),
        cx.accepts(allowed),
        "cart_cross_sell falling to the category of the cart lines",
    )


@when(
    "Recommend for cart_cross_sell is called through the gateway with the seed listing A and no context listings"
)
def cx_ask_seed_category(world: World) -> None:
    std = _std(world)
    _ask(
        world,
        {"context": CONTEXT_CART, "seedListingId": std.a, "limit": 20},
        cx.accepts(std.popular_ids(std.category_c)),
        "cart_cross_sell with a seed listing",
    )


@when(
    "Recommend for cart_cross_sell is called through the gateway with no context listings and no seed"
)
def cx_ask_empty(world: World) -> None:
    visible = cx.state(world)["visible_popular"]
    _ask(
        world,
        {"context": CONTEXT_CART, "limit": 20},
        cx.accepts(visible),
        "cart_cross_sell with an empty context",
    )


@when("Recommend for similar_items is called through the gateway with the seed listing A")
def cx_ask_similar_items(world: World) -> None:
    std = _std(world)
    contexts = [std.a]
    _ask(
        world,
        {
            "placementId": "similar_items",
            "context": CONTEXT_SIMILAR_ITEMS,
            "seedListingId": std.a,
            "limit": 10,
        },
        cx.accepts(std.similar_allowed(contexts)),
        "similar_items with a seed",
    )
    cx.state(world)["seed"] = std.a


@when("Recommend for cart_cross_sell is called through the gateway with 51 context listings")
def cx_ask_fifty_one(world: World) -> None:
    c = cx.state(world)
    c["since"] = cx.now_iso()
    c["answer"] = None
    c["error"] = None
    try:
        c["answer"] = cx.recommend(world, _cart_payload(_unknown(MAX_CONTEXT + 1)))
    except GatewayError as exc:
        c["error"] = exc


@when(
    "Recommend is called through the gateway for home_feed, similar_items and an undefined placement"
)
def cx_ask_existing(world: World) -> None:
    std = _std(world)
    seed_allowed = std.similar_allowed([std.a])
    popular_all = {e["listing_id"] for e in std.cat.popular if e.get("visible")}
    calls = {
        "home_feed": (
            {"placementId": "home_feed", "context": CONTEXT_HOMEPAGE, "limit": 10},
            cx.accepts(seed_allowed | popular_all),
        ),
        "similar_items": (
            {
                "placementId": "similar_items",
                "context": CONTEXT_SIMILAR_ITEMS,
                "seedListingId": std.a,
                "limit": 10,
            },
            cx.accepts(seed_allowed),
        ),
        "undefined": (
            {"placementId": "e2e-undefined-placement", "context": CONTEXT_HOMEPAGE, "limit": 10},
            cx.accepts(seed_allowed | popular_all),
        ),
    }
    answers: dict[str, dict[str, Any]] = {}
    for name, (payload, accept) in calls.items():
        answers[name] = cx.recommend_until(world, payload, accept, f"placement {name}")
    cx.state(world)["answers"] = answers


@then("the response holds at most 8 items in descending score")
def cx_at_most_eight(world: World) -> None:
    items = cx.ranked(_answer(world))
    assert 1 <= len(items) <= cx.RESULT_LIMIT, f"{len(items)} items: {items}"
    scores = [float(item["score"]) for item in items]
    assert scores == sorted(scores, reverse=True), scores


@then("every item is a neighbour of the context listings")
def cx_neighbours(world: World) -> None:
    std = _std(world)
    contexts = cx.state(world)["contexts"]
    ids = cx.ids_of(_answer(world))
    allowed = std.similar_allowed(contexts)
    assert ids, "the response holds no item"
    assert set(ids) <= allowed, f"not neighbours of {contexts}: {sorted(set(ids) - allowed)}"


@then("every item score is its maximum similarity to the context listings")
def cx_max_score(world: World) -> None:
    std = _std(world)
    contexts = cx.state(world)["contexts"]
    sims = [std.cat.similarities(context) for context in contexts]
    for item in cx.ranked(_answer(world)):
        expected = max(s[item["listingId"]] for s in sims if item["listingId"] in s)
        assert (
            abs(float(item["score"]) - expected) <= SCORE_TOLERANCE
        ), f"{item['listingId']}: score {item['score']}, max similarity {expected}"


@then("the response holds between 1 and 8 items")
def cx_between(world: World) -> None:
    items = cx.ranked(_answer(world))
    assert 1 <= len(items) <= cx.RESULT_LIMIT, f"{len(items)} items: {items}"


@then("the response contains none of the context listings")
def cx_no_context(world: World) -> None:
    ids = set(cx.ids_of(_answer(world)))
    assert ids, "the response holds no item"
    leaked = ids & set(cx.state(world)["contexts"])
    assert not leaked, f"cart lines served back: {sorted(leaked)}"


@then("the out-of-stock listing is not in the response")
def cx_no_oos(world: World) -> None:
    assert cx.state(world)["out_of_stock"] not in cx.ids_of(_answer(world))


@then("exactly one of those three listings is in the response and it is the most similar one")
def cx_one_per_seller(world: World) -> None:
    trio = cx.state(world)["trio"]
    ids = cx.ids_of(_answer(world))
    present = [lid for lid in ids if lid in trio]
    assert present == [trio[0]], f"expected only {trio[0]} of {trio}, got {present} in {ids}"
    assert len(ids) >= 2, f"the other sellers' listings are missing: {ids}"


@then("every item is a visible global popular listing and the hidden one is absent")
def cx_popular_visible(world: World) -> None:
    c = cx.state(world)
    ids = cx.ids_of(_answer(world))
    assert ids, "the response holds no item"
    assert c["hidden"] not in ids, f"hidden listing served: {ids}"
    assert set(ids) <= c["visible_popular"], f"not global popular: {ids}"


@then(parsers.parse("every item is a global popular listing of category {which}"))
def cx_popular_category(world: World, which: str) -> None:
    std = _std(world)
    categories = {"C": [std.category_c], "C or D": [std.category_c, std.category_d]}[which]
    ids = cx.ids_of(_answer(world))
    assert ids, "the response holds no item"
    assert set(ids) <= std.popular_ids(
        *categories
    ), f"items outside categories {which}: {sorted(set(ids) - std.popular_ids(*categories))}"


@then("the seed listing is not in the response")
def cx_no_seed(world: World) -> None:
    ids = cx.ids_of(_answer(world))
    assert ids, "the response holds no item"
    assert cx.state(world)["seed"] not in ids, f"seed served back: {ids}"


@then("the gateway answers INVALID_ARGUMENT")
def cx_invalid_argument(world: World) -> None:
    c = cx.state(world)
    error = c["error"]
    assert error is not None, f"51 context ids were accepted: {c['answer']}"
    assert error.status == 400 and "invalid_argument" in error.body.lower(), (
        error.status,
        error.body,
    )


@then("team-ai served no Recommend call for it")
def cx_no_team_ai_call(world: World) -> None:
    lines = cx.ai_lines_since("recs.served", cx.state(world)["since"])
    assert not lines, f"team-ai served the rejected request: {lines[:3]}"


@then("each of the three placements answers OK with items and names its own placement")
def cx_existing_resolve(world: World) -> None:
    answers = cx.state(world)["answers"]
    expected = {
        "home_feed": "home_feed",
        "similar_items": "similar_items",
        "undefined": "home_feed",
    }
    for name, placement in expected.items():
        answer = answers[name]
        assert cx.ids_of(answer), f"{name}: no items: {answer}"
        assert answer.get("placementId") == placement, f"{name}: {answer}"


def _cart_page(world: World) -> CartPage:
    return world.get_page(PageName.CART)


def _row_ids(world: World) -> list[str]:
    row = _cart_page(world).cross_sell
    hrefs = row.cards.evaluate_all("els => els.map(e => e.getAttribute('href') || '')")
    return [h.rsplit("/", 1)[-1] for h in hrefs if "/listing/" in h]


@given("a buyer has a cart line and two similar listings are indexed for recommendations")
def cx_ui_seed(world: World) -> None:
    user = d2.seed_buyer(world)
    cart_shop, first, second = (d2.seed_shop(price=200_000, stock=50) for _ in range(3))
    cs.add(user.token, cart_shop["listing_id"])
    cat = cx.Catalogue(world)
    category = f"cx-{cat.run}-cat"
    cat.add(
        cart_shop["listing_id"],
        cx.unit(cat.dim, {0: 1.0}),
        seller_id=cart_shop["seller_id"],
        category_id=category,
    )
    for n, shop in enumerate((first, second)):
        cat.add(
            shop["listing_id"],
            cx.unit(cat.dim, {0: 1.0, 2: 0.03 * (n + 1)}),
            seller_id=shop["seller_id"],
            category_id=category,
        )
    cat.set_popular([])
    similar = {first["listing_id"], second["listing_id"]}
    cx.state(world).update(
        cart_listing=cart_shop["listing_id"], similar=similar, buyer=user, cat=cat
    )
    cx.warm_up(world, _cart_payload([cart_shop["listing_id"]]))


@given("the buyer opens the cart and the cross-sell row is shown")
def cx_ui_row_shown_first(world: World) -> None:
    cx_ui_open_cart(world)
    expect(_cart_page(world).cross_sell.heading).to_be_visible(timeout=timeouts.LONG)


@when("the buyer opens the cart")
def cx_ui_open_cart(world: World) -> None:
    world.navigate_to(PageName.CART)


@when("the buyer empties the cart and opens the cart again")
def cx_ui_empty_and_reopen(world: World) -> None:
    d2.buyer_cart(world).clear_cart()
    cx.state(world)["since"] = cx.now_iso()
    world.navigate_to(PageName.CART)
    expect(_cart_page(world).empty_state).to_be_visible(timeout=timeouts.LONG)


@when("the buyer opens the cart recording beacons and clicks a cross-sell listing")
def cx_ui_beacons(world: World) -> None:
    beacons = _record_beacons(world)
    world.navigate_to(PageName.CART)
    row = _cart_page(world).cross_sell
    expect(row.cards.first).to_be_visible(timeout=timeouts.LONG)
    c = cx.state(world)
    c["request_id"] = row.request_id()
    row.cards.first.scroll_into_view_if_needed()
    deadline = time.monotonic() + 20
    while time.monotonic() < deadline and not any(
        b.get("type") == "impression" and b.get("placementId") == cx.PLACEMENT for b in beacons
    ):
        world.page.wait_for_timeout(500)
    row.cards.first.click()
    deadline = time.monotonic() + 20
    while time.monotonic() < deadline and not any(
        b.get("type") == "click" and b.get("placementId") == cx.PLACEMENT for b in beacons
    ):
        world.page.wait_for_timeout(500)
    c["served_version"] = cx.recommend(world, _cart_payload([c["cart_listing"]])).get(
        "modelVersion"
    )


@then("the cross-sell row is shown with between 1 and 8 listings")
def cx_ui_row_shown(world: World) -> None:
    row = _cart_page(world).cross_sell
    expect(row.heading).to_be_visible(timeout=timeouts.LONG)
    expect(row.cards.first).to_be_visible(timeout=timeouts.LONG)
    assert 1 <= row.card_count() <= cx.RESULT_LIMIT, row.card_count()


@then("none of the row listings is a cart line and every one is a similar listing")
def cx_ui_row_content(world: World) -> None:
    c = cx.state(world)
    shown = set(_row_ids(world))
    assert shown, "the cross-sell row rendered no listing card"
    assert c["cart_listing"] not in shown, f"a cart line is in the row: {shown}"
    assert shown <= c["similar"], f"listings outside the similar ones: {shown - c['similar']}"


@then("no cross-sell row is rendered on the empty cart")
def cx_ui_no_row(world: World) -> None:
    assert not _cart_page(world).cross_sell.is_present()


@then("team-ai served no Recommend call since the cart was emptied")
def cx_ui_no_call(world: World) -> None:
    lines = cx.ai_lines_since("recs.served", cx.state(world)["since"])
    assert not lines, f"a Recommend call was made for the empty cart: {lines[:3]}"


@then(
    "the impression and click beacons carry the request_id of the row as impressionId and the model version the gateway serves"
)
def cx_ui_beacon_identity(world: World) -> None:
    c = cx.state(world)
    beacons = world.state.extra["beacons"]
    assert c["request_id"], "the cross-sell row carries no data-recs-request-id"
    assert c["served_version"], "the gateway answered without a modelVersion"
    for kind in ("impression", "click"):
        sent = [
            b for b in beacons if b.get("type") == kind and b.get("placementId") == cx.PLACEMENT
        ]
        assert sent, f"no {kind} beacon for {cx.PLACEMENT} in {beacons}"
        wrong = [
            b
            for b in sent
            if b.get("impressionId") != c["request_id"]
            or b.get("modelVersion") != c["served_version"]
        ]
        assert not wrong, f"{kind} beacons without the recommendation identity: {wrong}"
