"""Tracking quality report through the gateway (analytics-data-quality, area adq-e2e)."""

from __future__ import annotations

import uuid
from datetime import UTC, datetime, timedelta

from pytest_bdd import given, parsers, then, when

from tests.e2e.support import adq_support as adq
from tests.e2e.support import tii_support as tii
from tests.e2e.support.world import World

FRESH_WITHIN = timedelta(minutes=5)


def _x(world: World) -> dict:
    return world.state.extra


# ── Givens ───────────────────────────────────────────────────────────────
@given("the seeded admin")
def seeded_admin(world: World) -> None:
    _x(world)["adq_admin"] = adq.admin_token(world)


@given("the seeded admin has read the tracking quality report for the last hour")
def admin_baseline_report(world: World) -> None:
    x = _x(world)
    x["adq_admin"] = adq.admin_token(world)
    x["adq_dups_before"] = adq.count(adq.report_json(x["adq_admin"], 1), "duplicatesSkipped")


@given("a logged-in buyer")
def logged_in_buyer(world: World) -> None:
    _x(world)["adq_buyer"], _ = tii.register_buyer()


# ── Duplicates ───────────────────────────────────────────────────────────
@when(
    "a visitor posts the same view (same eventId and anonymousId) twice and the admin then reads "
    "the tracking quality report for the last hour"
)
def post_same_view_twice_and_read(world: World) -> None:
    x = _x(world)
    beacon = tii.view(
        f"e2e-adq-sess-{tii.run_id()}",
        eventId=str(uuid.uuid4()),
        anonymousId=f"e2e-adq-anon-{uuid.uuid4()}",
    )
    for _ in range(2):
        resp = tii.post_track([beacon])
        assert resp.status_code == 202, (resp.status_code, resp.text[:300])
    minimum = x["adq_dups_before"] + 1
    x["adq_report"] = adq.poll_report(
        x["adq_admin"], lambda r: adq.count(r, "duplicatesSkipped") >= minimum
    )


@then("the report's duplicates_skipped is at least 1 higher than it was before the two posts")
def duplicates_counted(world: World) -> None:
    x = _x(world)
    after = adq.count(x["adq_report"], "duplicatesSkipped")
    assert after >= x["adq_dups_before"] + 1, (x["adq_dups_before"], x["adq_report"])


# ── Fresh views ──────────────────────────────────────────────────────────
@when("a visitor posts three views of a listing and the admin reads the report for the last hour")
def post_three_views_and_read(world: World) -> None:
    x = _x(world)
    listing = f"e2e-adq-{tii.run_id()}"
    # Wait for THESE views, not just any three: older views already satisfy ">= 3", and a
    # freshly (re)started sink may not have written anything new yet.
    before = adq.count(
        adq.type_row(adq.poll_report(x["adq_admin"], lambda r: True), "view"), "events"
    )
    adq.post_views(3, listingId=listing)
    x["adq_report"] = adq.poll_report(
        x["adq_admin"],
        lambda r: adq.count(adq.type_row(r, "view"), "events") >= before + 3 and _fresh(r),
    )


def _fresh(report: dict) -> bool:
    raw = report.get("lastIngestedAt")
    if not raw:
        return False
    age = datetime.now(UTC) - datetime.fromisoformat(raw.replace("Z", "+00:00"))
    return timedelta(seconds=-60) <= age <= FRESH_WITHIN


@then(
    "the view count is at least 3, the latest ingest time is within the last 5 minutes, "
    "and the status is not degraded for stale"
)
def fresh_views_report(world: World) -> None:
    report = _x(world)["adq_report"]
    assert adq.count(adq.type_row(report, "view"), "events") >= 3, report
    last = datetime.fromisoformat(report["lastIngestedAt"].replace("Z", "+00:00"))
    age = datetime.now(UTC) - last
    assert timedelta(seconds=-60) <= age <= FRESH_WITHIN, f"last ingest {age} ago: {report}"
    assert "stale" not in report.get("reasons", []), report


# ── Missing listing ──────────────────────────────────────────────────────
@when(
    "a visitor posts 50 views without a listingId and the admin reads the report for the last hour"
)
def post_listingless_views_and_read(world: World) -> None:
    x = _x(world)
    before = adq.count(adq.type_row(adq.report_json(x["adq_admin"], 1), "view"), "events")
    adq.post_views(50, listingId=None)
    x["adq_report"] = adq.poll_report(
        x["adq_admin"],
        lambda r: adq.count(adq.type_row(r, "view"), "events") >= before + 50
        and float(adq.type_row(r, "view").get("missingListingRatio", 0) or 0) > 0,
    )


@then(
    "the view type's missing-listing ratio is above 0 and, if it exceeds the configured maximum, "
    "the status is DEGRADED with reason incomplete"
)
def missing_listing_ratio(world: World) -> None:
    report = _x(world)["adq_report"]
    row = adq.type_row(report, "view")
    ratio = float(row.get("missingListingRatio", 0) or 0)
    assert ratio > 0, report
    assert row.get("listingScoped") is True, row
    if ratio > adq.missing_listing_max():
        assert report.get("status") == "DEGRADED", report
        assert "incomplete" in report.get("reasons", []), report


# ── Window out of range ──────────────────────────────────────────────────
@when(parsers.parse("the admin reads the report with a window of {hours:d} hours"))
def read_out_of_range(world: World, hours: int) -> None:
    x = _x(world)
    x["adq_resp"] = adq.read_report(x["adq_admin"], hours)


@then("the call fails with invalid_argument")
def invalid_argument(world: World) -> None:
    resp = _x(world)["adq_resp"]
    assert resp.status_code == 400, f"expected 400, got {resp.status_code} {resp.text[:300]}"
    assert resp.json().get("code") == "invalid_argument", resp.text[:300]


# ── Access ───────────────────────────────────────────────────────────────
@when(
    "an anonymous client and then a logged-in buyer call GetTrackingQualityReport through the gateway"
)
def anon_then_buyer(world: World) -> None:
    x = _x(world)
    x["adq_anon_resp"] = adq.read_report(None, 1)
    x["adq_buyer_resp"] = adq.read_report(x["adq_buyer"], 1)


@then("the gateway answers HTTP 401 and then HTTP 403")
def answers_401_then_403(world: World) -> None:
    x = _x(world)
    anon, buyer = x["adq_anon_resp"], x["adq_buyer_resp"]
    assert anon.status_code == 401, f"anonymous: {anon.status_code} {anon.text[:300]}"
    assert buyer.status_code == 403, f"buyer: {buyer.status_code} {buyer.text[:300]}"
