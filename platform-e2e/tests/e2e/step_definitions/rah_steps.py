"""Steps for recs-attribution-hardening (area rah).

Black box through the gateway: beacons via `POST /api/track` (roe_support), orders and payments via
the Connect RPCs (oic_order_support), the report via `GetRecommendationPerformance` as the seeded
admin. Every scenario has its own placement id and model version, so its rows are its own. Purchases
are real paid orders; the warehouse sees them after team-order's OrderPaidEvent is consumed, so the
`Then` steps poll (RAH_POLL_TIMEOUT_S, default 90).
"""

from __future__ import annotations

import os
from dataclasses import dataclass, field

import pytest
from pytest_bdd import then, when

from tests.e2e.support import efe_support as e
from tests.e2e.support import oic_order_support as oic
from tests.e2e.support import roe_support as roe
from tests.e2e.support import tii_support as tii
from tests.e2e.support.oic_order_support import Actor, OicWorld, register

POLL_S = float(os.getenv("RAH_POLL_TIMEOUT_S", "90"))


@dataclass
class Ctx:
    ids: dict = field(default_factory=dict)
    admin: str = ""


@pytest.fixture
def rah():
    c = Ctx()
    c.admin = oic.login_admin(OicWorld()).token
    return c


def _ids(c: Ctx, prefix: str) -> dict:
    c.ids = {
        "placement": roe.unique(f"placement-{prefix}"),
        "model": roe.unique(f"model-{prefix}"),
        "impression": roe.unique("imp"),
        "anon": roe.unique("anon"),
    }
    return c.ids


def _kw(i: dict, placement: str | None = None, model: str | None = None) -> dict:
    return {
        "anonymous_id": i["anon"],
        "impression_id": i["impression"],
        "placement_id": i["placement"] if placement is None else placement,
        "model_version": i["model"] if model is None else model,
    }


def _click_and_pay(c: Ctx) -> None:
    """A buyer sees and clicks a real listing from a recommendation row, then pays an order for it."""
    i = _ids(c, "pay")
    w = OicWorld()
    e.seller_with_listings(w, 1)
    buyer: Actor = register(w, "b1", "buyer")
    listing = e.listing_id(w, "L1")
    roe.post([roe.beacon("impression", listing_id=listing, **_kw(i))], buyer.token)
    roe.post([roe.beacon("click", listing_id=listing, **_kw(i))], buyer.token)
    order_id = oic.place_order(w, buyer, "L1", 1)
    oic.pay_order(w, buyer, order_id)
    oic.wait_status(w, buyer, order_id, oic.PAID)


def _row(report: dict, c: Ctx) -> dict:
    return roe.row(report, c.ids["placement"], c.ids["model"])


# ── Scenario: A purchase after a recommended click is attributed ─────────
@when("a logged-in buyer clicks a recommended listing and then pays an order for that listing")
def rah_click_then_pay(rah):
    _click_and_pay(rah)


@then("the report row for that placement and model counts that purchase")
def rah_row_counts_purchase(rah):
    def ok(report: dict) -> bool:
        return roe.num(_row(report, rah), "purchases") >= 1

    report = roe.poll_report(rah.admin, ok, timeout_s=POLL_S)
    assert ok(report), report


# ── Scenario: A purchase beacon without an order is not counted ──────────
@when(
    "a logged-in buyer clicks a recommended listing and posts a purchase tracking event for it, "
    "but pays no order"
)
def rah_forged_purchase(rah):
    i = _ids(rah, "forged")
    token, _ = tii.register_buyer()
    listing = roe.unique("listing")
    # One batch, so the click and the forged purchase reach the sink in the same flush.
    roe.post(
        [
            roe.beacon("impression", listing_id=listing, **_kw(i)),
            roe.beacon("click", listing_id=listing, **_kw(i)),
            roe.beacon("purchase", listing_id=listing, **_kw(i)),
        ],
        token,
    )


@then("the report row for that placement and model counts the click and 0 purchases")
def rah_click_no_purchase(rah):
    def ok(report: dict) -> bool:
        return roe.num(_row(report, rah), "clicks") >= 1

    report = roe.poll_report(rah.admin, ok, timeout_s=roe.POLL_TIMEOUT_S)
    mine = _row(report, rah)
    assert roe.num(mine, "clicks") == 1 and roe.num(mine, "purchases") == 0, mine


# ── Scenario: A click on a listing the impression did not show ───────────
@when(
    "a visitor posts an impression of listing A and a click on listing B with the same impressionId"
)
def rah_foreign_click(rah):
    i = _ids(rah, "foreign")
    a, b = roe.unique("listing-a"), roe.unique("listing-b")
    roe.post(
        [
            roe.beacon("impression", listing_id=a, **_kw(i)),
            roe.beacon("click", listing_id=b, **_kw(i)),
        ]
    )


@then("the report row for that placement and model counts the impression and 0 clicks")
def rah_impression_no_click(rah):
    def ok(report: dict) -> bool:
        return roe.num(_row(report, rah), "impressions") >= 1

    report = roe.poll_report(rah.admin, ok, timeout_s=roe.POLL_TIMEOUT_S)
    mine = _row(report, rah)
    assert roe.num(mine, "impressions") == 1 and roe.num(mine, "clicks") == 0, mine


# ── Scenario: A reused impression id keeps its placements and models apart
@when(
    "a visitor posts impressions of one listing with the same impressionId under two placements "
    "with two models, and one click carrying that impressionId, the first placement and the first "
    "model"
)
def rah_reused_id(rah):
    i = _ids(rah, "reuse")
    second = {"placement": roe.unique("placement-reuse2"), "model": roe.unique("model-reuse2")}
    rah.ids["second"] = second
    listing = roe.unique("listing")
    roe.post(
        [
            roe.beacon("impression", listing_id=listing, **_kw(i)),
            roe.beacon(
                "impression",
                listing_id=listing,
                **_kw(i, placement=second["placement"], model=second["model"]),
            ),
            roe.beacon("click", listing_id=listing, **_kw(i)),
        ]
    )


@then(
    "the report has a row for each placement and model with at least 1 impression, and only the "
    "first row counts the click"
)
def rah_rows_apart(rah):
    second = rah.ids["second"]

    def rows(report: dict) -> tuple[dict, dict]:
        return _row(report, rah), roe.row(report, second["placement"], second["model"])

    def ok(report: dict) -> bool:
        first, other = rows(report)
        return roe.num(first, "impressions") >= 1 and roe.num(other, "impressions") >= 1

    report = roe.poll_report(rah.admin, ok, timeout_s=roe.POLL_TIMEOUT_S)
    first, other = rows(report)
    assert roe.num(first, "clicks") == 1, (first, other)
    assert roe.num(other, "clicks") == 0, (first, other)


# ── Scenario: A conversion on a still-open attribution window ────────────
@when(
    "a logged-in buyer clicks a recommended listing and pays an order for it within the "
    "attribution window, and the report is read right away"
)
def rah_open_window(rah):
    _click_and_pay(rah)


@then(
    "the report row for that placement and model counts 1 click and 1 purchase, and its "
    "conversion rate is 0"
)
def rah_open_window_rate(rah):
    def ok(report: dict) -> bool:
        return roe.num(_row(report, rah), "purchases") >= 1

    report = roe.poll_report(rah.admin, ok, timeout_s=POLL_S)
    mine = _row(report, rah)
    assert roe.num(mine, "clicks") == 1 and roe.num(mine, "purchases") == 1, mine
    assert float(mine.get("conversionRate", 0) or 0) == 0.0, mine
