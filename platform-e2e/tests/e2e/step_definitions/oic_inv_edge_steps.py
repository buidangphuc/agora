"""Steps for security/oic_inv_edge_listing_commit.feature (port-order-inventory-correctness).

The listing stock commit RPC is service-to-service; the gateway answers 501 for every caller
without contacting team-domain. The reservation id of the buyer's live order is not exposed
by any edge RPC, so it is read best-effort from team-order's `order_reservations` (falling
back to the order id: the answer does not depend on the id because the route does not exist).
"""

from __future__ import annotations

import httpx
from pytest_bdd import given, parsers, then, when

from config.settings import get_settings
from tests.e2e.flows.oic_inv_flow import (
    checkout,
    create_listing,
    fill_cart,
    fixture,
    make_buyer,
    register,
    stock_of,
)
from tests.e2e.support import oic_inv_stack as stack
from tests.e2e.support.world import World

COMMIT = "/platform.listing.v1.ListingService/CommitReservation"


def _reservation_id(order_id: str) -> str:
    try:
        out = stack.docker(
            "exec", stack.postgres_container(), "psql", "-U", "postgres", "-d", "order_db",
            "-tAc", f"select id from order_reservations where order_id = '{order_id}' limit 1",
        ).stdout.strip()  # fmt: skip
    except Exception:  # noqa: BLE001 - best effort, see module docstring
        out = ""
    return out or order_id


@given(
    parsers.parse(
        "a buyer's live order for quantity {qty:d} of a seller's listing with stock {stock:d}"
    )
)
def live_order(world: World, qty: int, stock: int) -> None:
    fx = fixture(world)
    seller = register(world, "seller")
    fx.sellers.append(seller)
    listing_id = create_listing(world, seller, stock, "oic-edge")
    buyer = make_buyer(world)
    fill_cart(world, buyer, [(listing_id, qty)])
    orders = checkout(world, buyer)
    world.state.extra["oic_inv_edge"] = {
        "reservation_id": _reservation_id(orders[0]["id"]),
        "stock_after_checkout": stock_of(world, listing_id),
    }


@when(
    "a logged-in seller, then an anonymous caller, calls ListingService/CommitReservation through the gateway with the reservation id of that order"
)
def call_commit(world: World) -> None:
    fx = fixture(world)
    body = {"reservationId": world.state.extra["oic_inv_edge"]["reservation_id"]}
    url = f"{get_settings().gateway_url.rstrip('/')}{COMMIT}"
    responses = {}
    for caller, token in (("seller", fx.sellers[0].token), ("anonymous", None)):
        headers = {"Content-Type": "application/json"}
        if token:
            headers["Authorization"] = f"bearer {token}"
        responses[caller] = httpx.post(url, json=body, headers=headers, timeout=15)
    world.state.extra["oic_inv_edge"]["responses"] = responses


@then("each commit call answers HTTP 501 with code unimplemented")
def each_501(world: World) -> None:
    for caller, resp in world.state.extra["oic_inv_edge"]["responses"].items():
        assert (
            resp.status_code == 501
        ), f"{caller}: expected 501, got {resp.status_code} {resp.text}"
        assert resp.json().get("code") == "unimplemented", f"{caller}: {resp.text}"


@then(parsers.parse("the listing's stock is still the {stock:d} left by the checkout"))
def stock_unchanged(world: World, stock: int) -> None:
    listing_id = fixture(world).listings[0]
    expected = world.state.extra["oic_inv_edge"]["stock_after_checkout"]
    assert expected == stock, f"checkout left {expected}, expected {stock}"
    actual = stock_of(world, listing_id)
    assert actual == stock, f"listing {listing_id}: stock {actual}, expected {stock}"
