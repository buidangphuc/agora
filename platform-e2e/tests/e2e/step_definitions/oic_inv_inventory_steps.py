"""Steps for inventory/oic_inv_inventory_reservations.feature (port-order-inventory-correctness).

Black box through the gateway: seller listing, buyer checkout/cancel, stock via GetListing,
`listing.events` via Kafka. TTL scenarios sit out the effective sweep wait read from the
running containers (see tests/e2e/support/oic_inv_stack.py) and are tagged @destructive.
"""

from __future__ import annotations

import time
import uuid

from pytest_bdd import given, parsers, then, when

from src.api.services import OrderService
from src.api.services.base_service import GatewayError
from tests.e2e.flows.oic_inv_events_flow import read_stock_events
from tests.e2e.flows.oic_inv_flow import _svc as svc
from tests.e2e.flows.oic_inv_flow import (
    checkout,
    create_listing,
    fill_cart,
    fixture,
    make_buyer,
    order_status,
    poll,
    register,
    stock_of,
)
from tests.e2e.support import oic_inv_stack as stack
from tests.e2e.support.world import World


# ── Givens ───────────────────────────────────────────────────────────────
@given(parsers.parse("a seller with a listing whose stock is {stock:d}"))
def seller_with_listing(world: World, stock: int) -> None:
    seller = register(world, "seller")
    fixture(world).sellers.append(seller)
    create_listing(world, seller, stock)


@given(parsers.parse("two sellers whose listings have stock {first:d} and {second:d}"))
def two_sellers(world: World, first: int, second: int) -> None:
    """The failing (second) listing belongs to the seller whose id sorts last, so a checkout
    that reserves in sorted seller order has already reserved the first seller's stock."""
    sellers = sorted((register(world, "seller") for _ in range(2)), key=lambda s: s.id)
    fx = fixture(world)
    fx.sellers.extend(sellers)
    create_listing(world, sellers[0], first, "oic-first")
    create_listing(world, sellers[1], second, "oic-second")


@given("a buyer with a saved address")
def buyer_with_address(world: World) -> None:
    make_buyer(world)


@given(
    parsers.parse(
        "the buyer's cart holds quantity {q1:d} of the first listing and quantity {q2:d} of the second"
    )
)
def cart_two_listings(world: World, q1: int, q2: int) -> None:
    fx = fixture(world)
    fill_cart(world, fx.buyer, [(fx.listings[0], q1), (fx.listings[1], q2)])


@given(parsers.parse("the buyer has checked out quantity {qty:d} of the listing"))
def buyer_has_checked_out(world: World, qty: int) -> None:
    fx = fixture(world)
    fill_cart(world, fx.buyer, [(fx.listings[0], qty)])
    checkout(world, fx.buyer)


@given("the reservation sweep wait fits the run")
def sweep_wait_fits(world: World) -> None:
    wait = stack.require_wait_within_budget()
    world.state.extra["oic_inv_wait_s"] = wait
    world.logger.info(f"reservation sweep wait: {wait:.0f}s")


@given("the stack runs with the e2e reservation overlay")
def stack_runs_with_overlay(world: World) -> None:
    expected: dict[str, tuple[str, str]] = {}
    for service in ("team-domain", "team-order"):
        ttl, sweep = stack.configured_cadence(service)
        ttl_s = stack.parse_go_duration(ttl or "")
        sweep_s = stack.parse_go_duration(sweep or "")
        assert (
            ttl_s and sweep_s and ttl_s < stack.DEFAULT_TTL_S and sweep_s < stack.DEFAULT_SWEEP_S
        ), (
            f"{stack.container_name(service)} is not running with the short-TTL overlay "
            f"(RESERVATION_TTL={ttl!r}, RESERVATION_SWEEP_INTERVAL={sweep!r}); start the stack "
            f"with -f {stack.OVERLAY}"
        )
        expected[service] = (stack.go_duration_string(ttl_s), stack.go_duration_string(sweep_s))
    world.state.extra["oic_inv_overlay"] = expected


# ── Whens ────────────────────────────────────────────────────────────────
@when(parsers.parse("the buyer checks out quantity {qty:d} of the listing through the gateway"))
def buyer_checks_out(world: World, qty: int) -> None:
    fx = fixture(world)
    fill_cart(world, fx.buyer, [(fx.listings[0], qty)])
    checkout(world, fx.buyer)


@when("the buyer's checkout fails because the second seller's item is out of stock")
def checkout_fails(world: World) -> None:
    fx = fixture(world)
    try:
        checkout(world, fx.buyer)
    except GatewayError as exc:
        fx.checkout_error = exc
        world.logger.info(f"checkout failed as expected: HTTP {exc.status} {exc.body[:200]}")
        return
    raise AssertionError(f"the checkout succeeded but the second item is out of stock: {fx.orders}")


@when("the buyer cancels the Pending order")
@when("the buyer then cancels that order")
def buyer_cancels(world: World) -> None:
    fx = fixture(world)
    order = fx.orders[0]
    assert order_status(world, fx.buyer, order["id"]) == "ORDER_STATUS_PENDING", order
    svc(world, OrderService, fx.buyer.token).cancel_order(order["id"], "oic-inv")


@when("more than the reservation TTL plus two sweep intervals of both services pass")
def wait_out_sweeps(world: World) -> None:
    wait = world.state.extra.get("oic_inv_wait_s") or stack.require_wait_within_budget()
    deadline = time.monotonic() + wait
    # Poll a cheap read instead of one long sleep so a dead stack surfaces early.
    fx = fixture(world)
    while time.monotonic() < deadline:
        stock_of(world, fx.listings[0])
        time.sleep(min(5.0, max(0.0, deadline - time.monotonic())))


# ── Thens ────────────────────────────────────────────────────────────────
@then(parsers.parse("the listing's stock read through the gateway is {expected:d}"))
def stock_is(world: World, expected: int) -> None:
    listing_id = fixture(world).listings[0]
    actual = stock_of(world, listing_id)
    assert actual == expected, f"listing {listing_id}: stock {actual}, expected {expected}"


@then("the buyer's order is still Pending")
def order_still_pending(world: World) -> None:
    fx = fixture(world)
    status = order_status(world, fx.buyer, fx.orders[0]["id"])
    assert status == "ORDER_STATUS_PENDING", f"order status {status}"


@then(
    "the first seller's listing stock read through the gateway equals its stock before the checkout"
)
def first_stock_restored(world: World) -> None:
    fx = fixture(world)
    first = fx.listings[0]
    actual = stock_of(world, first)
    assert (
        actual == fx.stock_before[first]
    ), f"first listing {first}: stock {actual}, before the checkout {fx.stock_before[first]}"


# ── Configuration (boot a throwaway team-domain) ─────────────────────────
@when(
    parsers.parse(
        'the team-domain image is started with RESERVATION_TTL "{ttl}" and RESERVATION_SWEEP_INTERVAL "{sweep}"'
    )
)
def start_domain_image(world: World, ttl: str, sweep: str) -> None:
    """Run the real team-domain image against a scratch database (never the live one)."""
    tag = uuid.uuid4().hex[:8]
    db = f"oic_inv_boot_{tag}"
    name = f"oic-inv-boot-{tag}"
    url = stack.create_scratch_database(db)
    world.add_cleanup(lambda: stack.drop_scratch_database(db))
    stack.docker(
        "run", "-d", "--name", name, "--network", stack.stack_network(),
        "-e", "ENV=local", "-e", "GRPC_PORT=50061",
        "-e", "DATABASE_ENABLED=true", "-e", f"DATABASE_URL={url}",
        "-e", "KAFKA_ENABLED=false", "-e", "OUTBOX_ENABLED=false",
        "-e", f"RESERVATION_TTL={ttl}", "-e", f"RESERVATION_SWEEP_INTERVAL={sweep}",
        stack.image_name("team-domain"),
    )  # fmt: skip
    world.add_cleanup(lambda: stack.docker("rm", "-f", name, check=False))

    def logs() -> str:
        proc = stack.docker("logs", name, check=False)
        return proc.stdout + proc.stderr

    # Wait for either the effective-values line or the process dying.
    poll(
        lambda: "15m0s" in logs()
        or stack.docker(
            "inspect", name, "--format", "{{.State.Running}}", check=False
        ).stdout.strip()
        != "true",
        timeout_s=30,
    )
    state = stack.docker("inspect", name, "--format", "{{.State.Running}}", check=False)
    world.state.extra["oic_inv_boot"] = {"logs": logs(), "running": state.stdout.strip() == "true"}


@then("it starts and logs a warning naming each variable")
def boot_warns(world: World) -> None:
    boot = world.state.extra["oic_inv_boot"]
    assert boot[
        "running"
    ], f"team-domain did not stay up with invalid values:\n{boot['logs'][-600:]}"
    lines = boot["logs"].splitlines()
    for var in ("RESERVATION_TTL", "RESERVATION_SWEEP_INTERVAL"):
        assert any(
            "warn" in line.lower() and var in line for line in lines
        ), f"no warning names {var}:\n{boot['logs'][-800:]}"


@then(parsers.parse('it logs the effective TTL "{ttl}" and interval "{sweep}"'))
def boot_logs_effective(world: World, ttl: str, sweep: str) -> None:
    logs = world.state.extra["oic_inv_boot"]["logs"]
    assert ttl in logs and sweep in logs, f"effective {ttl}/{sweep} not logged:\n{logs[-800:]}"


@then(
    "team-domain and team-order each log the overlay values as their effective reservation TTL and sweep interval"
)
def overlay_logged(world: World) -> None:
    for service, (ttl, sweep) in world.state.extra["oic_inv_overlay"].items():
        logs = stack.container_logs(service)
        lines = [ln for ln in logs.splitlines() if ttl in ln and sweep in ln]
        assert lines, (
            f"{stack.container_name(service)} never logged effective TTL {ttl} and interval "
            f"{sweep}:\n{logs[-600:]}"
        )


# ── Stock events ─────────────────────────────────────────────────────────
def _events(world: World) -> list[dict]:
    from config.settings import get_settings

    settings = get_settings()
    fx = fixture(world)
    return read_stock_events(
        settings.kafka_brokers, "listing.events", fx.listings[0], since_ms=fx.started_ms
    )


@then(
    parsers.parse(
        "a ListingStockChanged envelope keyed by that listing id with stock {stock:d} appears on listing.events"
    )
)
def stock_event_appears(world: World, stock: int) -> None:
    listing_id = fixture(world).listings[0]
    events = poll(
        lambda: [e for e in _events(world) if e["stock"] == stock and e["key"] == listing_id],
        timeout_s=30,
        interval_s=2,
    )
    assert events, f"no ListingStockChanged with stock {stock} keyed {listing_id}: {_events(world)}"


@then(
    parsers.parse(
        "a later ListingStockChanged envelope for the listing with stock {stock:d} appears on listing.events"
    )
)
def later_stock_event_appears(world: World, stock: int) -> None:
    listing_id = fixture(world).listings[0]

    def later() -> list[dict]:
        events = _events(world)
        reserved = [e for e in events if e["stock"] == 8]
        restored = [e for e in events if e["stock"] == stock]
        if not reserved:
            return []
        first = min((e["partition"], e["offset"]) for e in reserved)
        return [e for e in restored if e["partition"] == first[0] and e["offset"] > first[1]]

    assert poll(later, timeout_s=30, interval_s=2), (
        f"no ListingStockChanged with stock {stock} after the stock-8 envelope for {listing_id}: "
        f"{_events(world)}"
    )
