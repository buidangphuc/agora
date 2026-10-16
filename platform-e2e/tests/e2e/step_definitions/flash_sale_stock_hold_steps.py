"""Steps of promo/flash_sale_stock_hold.feature (change promotion-flash-stock-hold).

Black box: campaigns are seeded as admin through the gateway; the service-only hold RPCs are
driven by `support/flash_hold_support` (grpcurl from a throwaway container); the stock meter is
read through the gateway `GetFlashSaleStock`; hold rows and expiry go through psql on promotion_db.
"""

from __future__ import annotations

import uuid
from datetime import datetime, timedelta, timezone

from pytest_bdd import given, parsers, then, when

from config.settings import get_settings
from src.models import Listing
from src.utils import data as fake
from tests.e2e.support import flash_hold_support as fh
from tests.e2e.support.world import World

SETTINGS = get_settings()
_PREFIX = "fh_"


def _x(world: World) -> dict:
    return world.state.extra


def _seed_listing(world: World) -> tuple[str, str]:
    """A published listing of a fresh seller (one per scenario, so stock is never shared).

    Returns (listing id, seller token)."""
    sf = world.service_factory
    token = sf.auth.register(fake.unique_username("flash_hold_seller"), SETTINGS.seed_password, "seller")
    sf.set_token(token)
    listing = Listing(
        title=f"[E2E][FlashHold] {uuid.uuid4().hex[:8]}",
        category_id="cat-electronics",
        price=5_000_000,
        stock=100,
        status="published",
        description="Sản phẩm seed tự động cho Flash-sale stock hold E2E.",
    )
    sf.listing.create_listing(listing)
    return str(listing.listing_id), token


def _seed(world: World, cap: int, price: int, starts: str | None = None, ends: str | None = None) -> None:
    # The owner seeds: an admin CreateCampaign is audited first and fails closed (503) on a stack
    # without team-audit; the owner path is audit-free and is the same CreateCampaign RPC.
    listing_id, seller_token = _seed_listing(world)
    kwargs = {}
    if starts:
        kwargs["starts_at"] = starts
    if ends:
        kwargs["ends_at"] = ends
    campaign_id = fh.seed_flash_campaign(seller_token, listing_id, cap, price, **kwargs)
    _x(world).update(
        fh_listing=listing_id,
        fh_campaign=campaign_id,
        fh_price=price,
        fh_cap=cap,
        fh_reservation=f"e2e-{uuid.uuid4().hex[:12]}:{listing_id}:",
    )


def _do_reserve(world: World, quantity: int, price: int, fresh: bool = False) -> None:
    st = _x(world)
    # A second reserve with the same reservation id is a replay (stored outcome), so a reserve that
    # is meant to compete for stock with an earlier hold uses a fresh id; fh_reservation stays the
    # first hold's id for commit/release.
    rid = f"e2e-{uuid.uuid4().hex[:12]}:{st['fh_listing']}:" if fresh else st["fh_reservation"]
    code, body = fh.reserve(
        rid, st["fh_campaign"], st["fh_listing"], quantity, price
    )
    st.setdefault("fh_reserves", []).append((code, body))
    st["fh_last"] = (code, body)


def _ok_body(result: tuple[str, object]) -> dict:
    code, body = result
    assert code == "ok", f"expected an OK call, got {code}: {body}"
    assert isinstance(body, dict)
    return body


# ── Given ────────────────────────────────────────────────────────────────
@given(parsers.parse("a flash campaign with stock cap {cap:d} and sale price {price:d}"))
def flash_campaign(world: World, cap: int, price: int) -> None:
    _seed(world, cap, price)


@given(parsers.parse("an ended flash campaign with stock cap {cap:d} and sale price {price:d}"))
def ended_flash_campaign(world: World, cap: int, price: int) -> None:
    now = datetime.now(timezone.utc)
    fmt = "%Y-%m-%dT%H:%M:%SZ"
    _seed(
        world, cap, price,
        starts=(now - timedelta(hours=3)).strftime(fmt),
        ends=(now - timedelta(hours=1)).strftime(fmt),
    )  # fmt: skip


@given(parsers.parse("the order service has reserved {qty:d} units at the sale price"))
def order_service_has_reserved(world: World, qty: int) -> None:
    _do_reserve(world, qty, _x(world)["fh_price"])
    assert _ok_body(_x(world)["fh_last"]).get("reserved") is True, _x(world)["fh_last"]


def _labelled(world: World, label: str) -> str:
    st = _x(world)
    return st.setdefault("fh_labels", {}).setdefault(
        label, f"e2e-{label}-{uuid.uuid4().hex[:8]}:{st['fh_listing']}:"
    )


@given(parsers.parse('the order service holds {qty:d} units under reservation "{label}"'))
def holds_labelled(world: World, qty: int, label: str) -> None:
    st = _x(world)
    body = _ok_body(
        fh.reserve(_labelled(world, label), st["fh_campaign"], st["fh_listing"], qty, st["fh_price"])
    )
    assert body.get("reserved") is True, body


@given(parsers.parse('the order service commits reservation "{label}"'))
def commits_labelled(world: World, label: str) -> None:
    assert _ok_body(fh.commit(_labelled(world, label))).get("committed") is True


@given(parsers.parse('the order service releases reservation "{label}"'))
def releases_labelled(world: World, label: str) -> None:
    assert _ok_body(fh.release(_labelled(world, label))).get("released") is True


@given("the order service has committed the reservation")
def order_service_has_committed(world: World) -> None:
    body = _ok_body(fh.commit(_x(world)["fh_reservation"]))
    assert body.get("committed") is True, body


@given("the hold is 15 minutes old")
def hold_is_old(world: World) -> None:
    rid = _x(world)["fh_reservation"]
    assert fh.hold_rows(rid), f"no hold row for {rid} to backdate"
    fh.backdate_hold(rid)


@given("a registered flash-sale buyer")
def flash_buyer(world: World) -> None:
    token = world.service_factory.auth.register(
        fake.unique_username("flash_hold_buyer"), SETTINGS.seed_password, "buyer"
    )
    _x(world)["fh_buyer"] = token


# ── When ─────────────────────────────────────────────────────────────────
@when(parsers.parse("the order service reserves {qty:d} units at the sale price"))
def reserve_at_price(world: World, qty: int) -> None:
    _do_reserve(world, qty, _x(world)["fh_price"], fresh=bool(_x(world).get("fh_reserves")))


@when(parsers.parse("the order service reserves {qty:d} units at a price {delta:d} below the sale price"))
def reserve_below_price(world: World, qty: int, delta: int) -> None:
    _do_reserve(world, qty, _x(world)["fh_price"] - delta)


@when("the order service repeats the same reserve")
def repeat_reserve(world: World) -> None:
    _do_reserve(world, 2, _x(world)["fh_price"])


@when(parsers.parse("{n:d} single-unit reserves with distinct reservation ids run in parallel"))
def parallel_reserves(world: World, n: int) -> None:
    st = _x(world)
    st["fh_parallel"] = fh.reserve_many(
        n,
        campaign_id=st["fh_campaign"],
        listing_id=st["fh_listing"],
        expected_sale_price=st["fh_price"],
    )


@when("the order service commits the reservation")
def do_commit(world: World) -> None:
    _x(world)["fh_last"] = fh.commit(_x(world)["fh_reservation"])


@when("the order service releases the reservation")
def do_release(world: World) -> None:
    _x(world)["fh_last"] = fh.release(_x(world)["fh_reservation"])


@when("the buyer posts reserve, commit and release for the reservation to the gateway")
def buyer_via_gateway(world: World) -> None:
    st = _x(world)
    bodies = {
        "ReserveFlashSaleStock": {
            "reservationId": f"e2e-gw-{uuid.uuid4().hex[:10]}:{st['fh_listing']}:",
            "campaignId": st["fh_campaign"],
            "listingId": st["fh_listing"],
            "quantity": 1,
            "expectedSalePrice": st["fh_price"],
        },
        "CommitFlashSaleStock": {"reservationId": st["fh_reservation"]},
        "ReleaseFlashSaleStock": {"reservationId": st["fh_reservation"]},
    }
    st["fh_gateway"] = {
        m: fh.gateway_hold_call(st["fh_buyer"], m, b) for m, b in bodies.items()
    }


def _direct_all(world: World, principal: str) -> None:
    st = _x(world)
    rid = st["fh_reservation"]
    st["fh_direct"] = [
        fh.reserve(
            f"e2e-direct-{uuid.uuid4().hex[:10]}:{st['fh_listing']}:",
            st["fh_campaign"], st["fh_listing"], 1, st["fh_price"], principal=principal,
        ),
        fh.commit(rid, principal=principal),
        fh.release(rid, principal=principal),
    ]  # fmt: skip


@when("a user principal calls reserve, commit and release directly on team-promotion")
def user_direct(world: World) -> None:
    _direct_all(world, "user")


@when("a user principal carrying the promotion.reserve and admin scopes calls reserve directly")
def user_scoped_direct(world: World) -> None:
    st = _x(world)
    st["fh_direct"] = [
        fh.reserve(
            f"e2e-direct-{uuid.uuid4().hex[:10]}:{st['fh_listing']}:",
            st["fh_campaign"], st["fh_listing"], 1, st["fh_price"], principal="user-with-scope",
        )
    ]  # fmt: skip


@when(
    "the service principal service-team-order with scope promotion.reserve reserves, commits and releases in turn"
)
def order_service_full_cycle(world: World) -> None:
    st = _x(world)
    rid = st["fh_reservation"]
    st["fh_direct"] = [
        fh.reserve(rid, st["fh_campaign"], st["fh_listing"], 1, st["fh_price"], principal="service"),
        fh.commit(rid, principal="service"),
        fh.release(rid, principal="service"),
    ]  # fmt: skip


# ── Then ─────────────────────────────────────────────────────────────────
@then("the reserve is accepted with an empty reason")
def reserve_accepted(world: World) -> None:
    body = _ok_body(_x(world)["fh_last"])
    assert body.get("reserved") is True, body
    assert not body.get("reason"), body


@then("both reserves are accepted")
def both_accepted(world: World) -> None:
    results = _x(world)["fh_reserves"]
    assert len(results) == 2, results
    for r in results:
        body = _ok_body(r)
        assert body.get("reserved") is True and not body.get("reason"), r


@then(parsers.parse('the reserve is refused with reason "{reason}"'))
def reserve_refused(world: World, reason: str) -> None:
    body = _ok_body(_x(world)["fh_last"])
    assert not body.get("reserved"), body
    assert body.get("reason") == reason, body


@then(parsers.parse("the stock meter shows {remaining:d} remaining of {cap:d}"))
def meter_shows(world: World, remaining: int, cap: int) -> None:
    got = fh.stock(_x(world)["fh_campaign"])
    assert got == {"remaining": remaining, "stock_cap": cap}, got


@then(parsers.parse("exactly one hold of {qty:d} units exists for the reservation"))
def one_hold(world: World, qty: int) -> None:
    rows = fh.hold_rows(_x(world)["fh_reservation"])
    assert len(rows) == 1, rows
    assert int(rows[0][1]) == qty, rows


@then("no hold exists for the reservation")
def no_hold(world: World) -> None:
    rows = fh.hold_rows(_x(world)["fh_reservation"])
    assert rows == [], rows


@then(parsers.parse("exactly {n:d} of the parallel reserves are accepted"))
def parallel_accepted(world: World, n: int) -> None:
    results = _x(world)["fh_parallel"]
    bad = [r for r in results if r[0] != "ok"]
    assert not bad, f"parallel reserves failed: {bad[:2]}"
    accepted = [r for r in results if r[1].get("reserved") is True]
    assert len(accepted) == n, results


@then(parsers.parse('the other {n:d} parallel reserves are refused with reason "{reason}"'))
def parallel_refused(world: World, n: int, reason: str) -> None:
    results = _x(world)["fh_parallel"]
    refused = [r for r in results if r[0] == "ok" and not r[1].get("reserved")]
    assert len(refused) == n, results
    assert all(r[1].get("reason") == reason for r in refused), refused


@then("the commit is accepted")
def commit_accepted(world: World) -> None:
    assert _ok_body(_x(world)["fh_last"]).get("committed") is True, _x(world)["fh_last"]


@then("the commit is refused")
def commit_refused(world: World) -> None:
    assert not _ok_body(_x(world)["fh_last"]).get("committed"), _x(world)["fh_last"]


@then("the release is accepted")
def release_accepted(world: World) -> None:
    assert _ok_body(_x(world)["fh_last"]).get("released") is True, _x(world)["fh_last"]


@then("none of the buyer's gateway calls is served")
def gateway_refuses(world: World) -> None:
    for method, resp in _x(world)["fh_gateway"].items():
        assert resp.status_code >= 400, f"{method} was served to a buyer: {resp.status_code} {resp.text}"
        assert '"reserved":true' not in resp.text.replace(" ", ""), (method, resp.text)


@then(parsers.parse('every direct call is refused with "{code}"'))
def direct_refused(world: World, code: str) -> None:
    for got, body in _x(world)["fh_direct"]:
        assert got == code, f"expected {code}, got {got}: {body}"


@then("every direct call returns ok")
def direct_ok(world: World) -> None:
    for got, body in _x(world)["fh_direct"]:
        assert got == "ok", f"expected ok, got {got}: {body}"
