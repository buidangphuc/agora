"""Steps for recsys/in_stock_recommendations.feature (change recs-in-stock-filter), through the gateway.

Black box: listings are real (created and sold out through the gateway as a seller, projected into
search by listing.events), the serving Qdrant collection and Redis popular list carry their ids with
`in_stock=true`, and team-search is stopped and started with docker. See
flows/recsys_stock_flow.py for the private team-ai without the `search:read` scope.
"""

from __future__ import annotations

import time
from typing import Any

import httpx
from pytest_bdd import given, then, when

from src.api.services import GatewayError
from src.api.services.recommendation_service import (
    CONTEXT_CART,
    CONTEXT_HOMEPAGE,
    CONTEXT_SIMILAR_ITEMS,
)
from src.constants import gateway_endpoints as ep
from tests.e2e.flows import recsys_stock_flow as rsf
from tests.e2e.support import cart_cross_sell_support as cx
from tests.e2e.support import pear_edge_support as pe
from tests.e2e.support import srm_support as s
from tests.e2e.support.world import World

STOCK = 5
NEIGHBOURS = 5
POPULAR = 4
RESULT_LIMIT = 8


def _st(world: World) -> dict[str, Any]:
    return world.state.extra.setdefault("isr", {})


def _publish(world: World, labels: list[str]) -> dict[str, str]:
    for label in labels:
        s.create_listing(world, label, stock=STOCK)
    for label in labels:
        s.wait_indexed(world, label)
        s.wait_stock(world, label, STOCK)
    return {label: s.listing(world, label).id for label in labels}


def _sell_out(world: World, label: str) -> None:
    listed = s.listing(world, label)
    current = s.ok_json(s.post(s.LISTING + "GetListing", {"id": listed.id}))["listing"]
    current["stock"] = 0
    s.ok_json(s.post(s.LISTING + "UpdateListing", {"listing": current}, s.seller_of(world).token))
    s.wait_stock(world, label, 0)


def _neighbourhood(world: World) -> None:
    st = _st(world)
    rsf.ensure_serving_generation(world)
    cat = cx.Catalogue(world)
    ids = _publish(world, ["seed", *[f"n{n}" for n in range(NEIGHBOURS)]])
    run = cat.run
    category = f"isr-{run}-cat"
    cat.add(
        ids["seed"],
        cx.unit(cat.dim, {0: 1.0}),
        seller_id=f"isr-{run}-seller-seed",
        category_id=category,
        in_stock=True,
    )
    for n in range(NEIGHBOURS):
        label = f"n{n}"
        cat.add(
            ids[label],
            cx.unit(cat.dim, {0: 1.0, 2: 0.03 * (n + 1)}),
            seller_id=f"isr-{run}-seller-{label}",
            category_id=category,
            in_stock=True,
        )
    cat.set_popular([])
    st.update(cat=cat, ids=ids, neighbours={ids[f"n{n}"] for n in range(NEIGHBOURS)})


def _cart_payload(world: World) -> dict[str, Any]:
    return {
        "context": CONTEXT_CART,
        "contextListingIds": [_st(world)["ids"]["seed"]],
        "limit": RESULT_LIMIT,
    }


def _ids(answer: dict[str, Any]) -> list[str]:
    return cx.ids_of(answer)


@given(
    "a seed listing and five neighbours, all published with stock 5 in search, each of a different seller and each with Qdrant in_stock true"
)
def isr_similar_catalogue(world: World) -> None:
    _neighbourhood(world)


@given("a cart listing A and five neighbours, all published with stock 5 in search")
def isr_cart_catalogue(world: World) -> None:
    _neighbourhood(world)


@given("the seller sells out one neighbour through the gateway and search shows its stock as 0")
def isr_sell_out_neighbour(world: World) -> None:
    st = _st(world)
    _sell_out(world, "n1")
    st["sold_out"] = st["ids"]["n1"]


@given(
    "the global popular list holds four published listings of four sellers with stock 5 in search"
)
def isr_popular(world: World) -> None:
    st = _st(world)
    rsf.ensure_serving_generation(world)
    cat = cx.Catalogue(world)
    labels = [f"p{n}" for n in range(POPULAR)]
    ids = _publish(world, labels)
    run = cat.run
    cat.set_popular(
        [
            {
                "listing_id": ids[label],
                "score": 100.0 - n,
                "seller_id": f"isr-{run}-seller-{label}",
                "category_id": f"isr-{run}-cat",
                "visible": True,
            }
            for n, label in enumerate(labels)
        ]
    )
    st.update(cat=cat, ids=ids, popular={ids[label] for label in labels})


@given("Recommend for home_feed returns all four listings")
def isr_home_first(world: World) -> None:
    st = _st(world)
    payload = {"context": CONTEXT_HOMEPAGE, "limit": RESULT_LIMIT}
    answer = cx.recommend_until(
        world,
        payload,
        lambda ids: set(ids) == st["popular"],
        "home_feed over the four published popular listings",
    )
    st.update(payload=payload, first=answer)


@given("Recommend for cart_cross_sell with the context listing A returns neighbours")
def isr_cart_control(world: World) -> None:
    st = _st(world)
    payload = _cart_payload(world)
    cx.recommend_until(
        world,
        payload,
        lambda ids: bool(ids) and set(ids) <= st["neighbours"],
        "cart_cross_sell over the in-stock neighbours",
    )
    st["payload"] = payload


@given("a team-ai whose search principal lacks scope search:read serves a private gateway")
def isr_private_gateway(world: World) -> None:
    address = rsf.private_ai_without_search_scope(world)
    _st(world)["private_base"] = pe.private_gateway(
        world, {"UPSTREAM_RECOMMENDATION_ADDR": address}
    )


@when("the seller sells out one of them through the gateway and search shows its stock as 0")
def isr_sell_out_popular(world: World) -> None:
    st = _st(world)
    _sell_out(world, "p1")
    st["sold_out"] = st["ids"]["p1"]


@when("team-search is stopped")
def isr_stop_search(world: World) -> None:
    world.add_cleanup(lambda: (rsf.start_search(), rsf.wait_search_healthy()))
    rsf.stop_search()


@when("team-search is started again")
def isr_start_search(world: World) -> None:
    rsf.start_search()
    rsf.wait_search_healthy()


@when("Recommend for similar_items is called through the gateway with the seed listing")
def isr_ask_similar(world: World) -> None:
    st = _st(world)
    payload = {
        "context": CONTEXT_SIMILAR_ITEMS,
        "seedListingId": st["ids"]["seed"],
        "limit": RESULT_LIMIT,
    }
    st["answer"] = cx.recommend_until(world, payload, lambda ids: bool(ids), "similar_items")


@when("Recommend for home_feed is called again through the gateway")
def isr_home_again(world: World) -> None:
    st = _st(world)
    st["answer"] = cx.recommend(world, st["payload"])


@when("Recommend for cart_cross_sell is called through the gateway with the context listing A")
def isr_cart_call(world: World) -> None:
    st = _st(world)
    try:
        st["answer"] = cx.recommend(world, _cart_payload(world))
    except GatewayError as exc:
        raise AssertionError(
            f"Recommend failed instead of answering OK with no items: {exc}"
        ) from exc


@when(
    "Recommend for cart_cross_sell is called through the private gateway with the context listing A"
)
def isr_cart_private(world: World) -> None:
    st = _st(world)
    token = world.service_factory.recommendation._token  # noqa: SLF001
    deadline = time.monotonic() + cx.SETTLE_S
    resp = None
    while time.monotonic() < deadline:
        try:
            resp = httpx.post(
                st["private_base"] + ep.RECOMMENDATION_RECOMMEND,
                json=_cart_payload(world),
                headers={"Authorization": f"bearer {token}", "Content-Type": "application/json"},
                timeout=30.0,
            )
        except httpx.HTTPError:
            resp = None
        if resp is not None and resp.status_code == httpx.codes.OK:
            break
        time.sleep(cx.POLL_S)
    assert resp is not None and resp.status_code == httpx.codes.OK, (
        f"the private gateway never answered OK: {None if resp is None else (resp.status_code, resp.text)}"
    )
    st["answer"] = resp.json()


@then("the sold-out listing is absent from the response")
def isr_sold_out_absent(world: World) -> None:
    st = _st(world)
    assert st["sold_out"] not in _ids(st["answer"]), (
        f"sold-out listing served: {_ids(st['answer'])}"
    )


@then(
    "every returned listing has search stock greater than 0 and the four in-stock neighbours are returned"
)
def isr_all_in_stock(world: World) -> None:
    st = _st(world)
    ids = _ids(st["answer"])
    by_id = {s.listing(world, label).id: label for label in s.ctx(world).listings}
    assert ids, "the response holds no item"
    for listing_id in ids:
        assert listing_id in by_id, f"{listing_id} is not one of this scenario's listings"
        listed = s.listing(world, by_id[listing_id])
        stock = s.stock_of_hit(s.hit_of(s.search(listed.title), listing_id))
        assert stock is not None and stock > 0, f"{listing_id} served with search stock {stock}"
    expected = st["neighbours"] - {st["sold_out"]}
    assert set(ids) == expected, f"expected {sorted(expected)}, got {ids}"


@then(
    "the second response no longer contains that listing, still contains the other three and carries the same model version"
)
def isr_home_after(world: World) -> None:
    st = _st(world)
    ids = _ids(st["answer"])
    assert st["sold_out"] not in ids, f"sold-out listing still served: {ids}"
    assert set(ids) == st["popular"] - {st["sold_out"]}, f"expected the other three, got {ids}"
    assert st["answer"].get("modelVersion") == st["first"].get("modelVersion"), (
        "a training run happened between the two calls"
    )


@then("the response is OK with zero items")
def isr_zero_items(world: World) -> None:
    ids = _ids(_st(world)["answer"])
    assert ids == [], f"the placement was not empty: {ids}"


@then("a later Recommend for cart_cross_sell with the context listing A returns neighbours again")
def isr_back(world: World) -> None:
    st = _st(world)
    cx.recommend_until(
        world,
        _cart_payload(world),
        lambda ids: bool(ids) and set(ids) <= st["neighbours"],
        "cart_cross_sell after team-search restarted",
        timeout_s=60.0,
    )


@then("the stack's own gateway still returns neighbours for the same request")
def isr_stack_gateway(world: World) -> None:
    st = _st(world)
    answer = cx.recommend(world, _cart_payload(world))
    ids = _ids(answer)
    assert ids and set(ids) <= st["neighbours"], f"the stack's team-ai answered {ids}"
