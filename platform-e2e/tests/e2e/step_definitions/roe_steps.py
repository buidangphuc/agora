"""Recommendation performance report through the gateway (recsys-online-evaluation, area roe-e2e)."""

from __future__ import annotations

import os
import time

from pytest_bdd import given, parsers, then, when

from tests.e2e.support import adq_support as adq
from tests.e2e.support import efe_support as e
from tests.e2e.support import oic_order_support as oic
from tests.e2e.support import roe_support as roe
from tests.e2e.support import tii_support as tii
from tests.e2e.support.oic_order_support import Actor, OicWorld, register
from tests.e2e.support.world import World

NO_PURCHASE_HOLD_S = float(os.getenv("RAH_POLL_TIMEOUT_S", "90"))


def _x(world: World) -> dict:
    return world.state.extra


def _ids(world: World, prefix: str) -> dict:
    """Test-only placement, model version, impression id and anonymous id for this scenario."""
    x = _x(world)
    x["roe"] = {
        "placement": roe.unique(f"placement-{prefix}"),
        "model": roe.unique(f"model-{prefix}"),
        "impression": roe.unique("imp"),
        "anon": roe.unique("anon"),
    }
    return x["roe"]


# ── Givens ───────────────────────────────────────────────────────────────
@given("the seeded admin for recommendation performance")
def seeded_admin(world: World) -> None:
    _x(world)["roe_admin"] = adq.admin_token(world)


@given("a logged-in buyer for recommendation performance")
def logged_in_buyer(world: World) -> None:
    _x(world)["roe_buyer"], _ = tii.register_buyer()


# ── Impressions and clicks ───────────────────────────────────────────────
@when(
    "a visitor posts 2 impressions of a recommendation row and 1 click on one of its listings "
    "with the same impressionId"
)
def post_two_impressions_one_click(world: World) -> None:
    i = _ids(world, "ctr")
    listings = [roe.unique("listing"), roe.unique("listing")]
    kw = dict(
        anonymous_id=i["anon"],
        impression_id=i["impression"],
        placement_id=i["placement"],
        model_version=i["model"],
    )
    roe.post(
        [roe.beacon("impression", listing_id=lid, **kw) for lid in listings]
        + [roe.beacon("click", listing_id=listings[0], **kw)]
    )


@then(
    "the admin's report for the last hour has a row for that placement and model with at least "
    "1 impression, at least 2 item impressions and at least 1 click"
)
def row_has_counts(world: World) -> None:
    x = _x(world)
    i = x["roe"]

    def ok(report: dict) -> bool:
        r = roe.row(report, i["placement"], i["model"])
        return (
            roe.num(r, "impressions") >= 1
            and roe.num(r, "itemImpressions") >= 2
            and roe.num(r, "clicks") >= 1
        )

    report = roe.poll_report(x["roe_admin"], ok)
    assert ok(report), report


# ── Attribution ──────────────────────────────────────────────────────────
@when("a logged-in buyer purchases a listing they never clicked from a recommendation row")
def purchase_without_click(world: World) -> None:
    i = _ids(world, "noattr")
    # A real paid order (purchases come from order facts, recs-attribution-hardening). The row
    # exists (an impression of the listing) but the buyer never clicks it.
    w = OicWorld()
    e.seller_with_listings(w, 1)
    buyer: Actor = register(w, "b1", "buyer")
    listing = e.listing_id(w, "L1")
    roe.post(
        [
            roe.beacon(
                "impression",
                listing_id=listing,
                anonymous_id=i["anon"],
                impression_id=i["impression"],
                placement_id=i["placement"],
                model_version=i["model"],
            )
        ],
        buyer.token,
    )
    order_id = oic.place_order(w, buyer, "L1", 1)
    oic.pay_order(w, buyer, order_id)
    oic.wait_status(w, buyer, order_id, oic.PAID)


@then("no report row's purchase count includes that purchase")
def no_purchase_attributed(world: World) -> None:
    x = _x(world)
    i = x["roe"]
    # Wait until the row is in the report so the purchase (posted after) has had the same flush
    # chance, then check it did not turn into a purchase on that row or any other row of ours.
    report = roe.poll_report(
        x["roe_admin"],
        lambda r: roe.num(roe.row(r, i["placement"], i["model"]), "impressions") >= 1,
    )
    # The paid order reaches the warehouse after OrderPaidEvent is consumed: keep reading for the
    # same window an attributed purchase gets (rah), and fail if one ever shows up.
    deadline = time.monotonic() + NO_PURCHASE_HOLD_S
    while True:
        mine = roe.row(report, i["placement"], i["model"])
        assert roe.num(mine, "purchases") == 0, mine
        if time.monotonic() >= deadline:
            break
        time.sleep(5)
        report = roe.report_json(x["roe_admin"])
    mine_models = [
        r for r in report.get("rows", []) if str(r.get("modelVersion", "")) == i["model"]
    ]
    assert all(roe.num(r, "purchases") == 0 for r in mine_models), mine_models


# ── Fallback share ───────────────────────────────────────────────────────
@when(
    parsers.parse(
        'a visitor posts impressions for a placement with model version "{fallback}" and with a '
        "real model version, 1 each"
    )
)
def post_fallback_and_real(world: World, fallback: str) -> None:
    i = _ids(world, "fb")
    # A dedicated placement: the share is per placement across ALL model versions, so a shared
    # placement such as "cart_cross_sell" could be skewed by other tests.
    for model, imp in ((fallback, f"{i['impression']}-a"), (i["model"], f"{i['impression']}-b")):
        roe.post(
            [
                roe.beacon(
                    "impression",
                    listing_id=roe.unique("listing"),
                    anonymous_id=i["anon"],
                    impression_id=imp,
                    placement_id=i["placement"],
                    model_version=model,
                )
            ]
        )


@then("the report gives that placement a fallback share above 0 and below 1")
def fallback_between(world: World) -> None:
    x = _x(world)
    i = x["roe"]

    def ok(report: dict) -> bool:
        return 0 < roe.fallback_share(report, i["placement"]) < 1

    report = roe.poll_report(x["roe_admin"], ok)
    assert ok(report), report


# ── Access ───────────────────────────────────────────────────────────────
@when(
    "an anonymous client and then a logged-in buyer call GetRecommendationPerformance through "
    "the gateway"
)
def anon_then_buyer(world: World) -> None:
    x = _x(world)
    x["roe_anon_resp"] = roe.read_report(None)
    x["roe_buyer_resp"] = roe.read_report(x["roe_buyer"])


@then("the gateway answers HTTP 401 and then HTTP 403 for recommendation performance")
def answers_401_then_403(world: World) -> None:
    x = _x(world)
    anon, buyer = x["roe_anon_resp"], x["roe_buyer_resp"]
    assert anon.status_code == 401, f"anonymous: {anon.status_code} {anon.text[:300]}"
    assert buyer.status_code == 403, f"buyer: {buyer.status_code} {buyer.text[:300]}"
