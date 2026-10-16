"""Gateway helpers for listing reports (slice engagement-listing-reports, initiative q6).

Thin Connect JSON wrappers over EngagementService.CreateReport / ListReports / ResolveReport,
black box through the gateway. Reuses the oic actor helpers and audit_support (never edits them).
"""

from __future__ import annotations

import time

import httpx

from tests.e2e.support import audit_support as aud
from tests.e2e.support import review_order_support as ros
from tests.e2e.support.oic_order_support import Actor, OicWorld

ENGAGEMENT = ros.ENGAGEMENT
LISTING_TYPE = "REPORT_TARGET_TYPE_LISTING"
OPEN = "REPORT_STATUS_OPEN"
ACTIONED = "REPORT_STATUS_ACTIONED"


def _token(actor: Actor | None) -> str | None:
    return actor.token if actor else None


def create_report(
    actor: Actor | None, listing_id: str, reason: str = "scam", note: str = ""
) -> httpx.Response:
    return ros.post(
        _token(actor),
        ENGAGEMENT,
        "CreateReport",
        {"targetType": LISTING_TYPE, "targetId": listing_id, "reasonCode": reason, "note": note},
    )


def list_reports(
    actor: Actor | None, *, target_id: str | None = None, status: str | None = None
) -> httpx.Response:
    body: dict = {"page": {"pageSize": 100}}
    if target_id:
        body["targetId"] = target_id
    if status:
        body["status"] = status
    return ros.post(_token(actor), ENGAGEMENT, "ListReports", body)


def resolve_report(
    actor: Actor | None, report_id: str, status: str, note: str
) -> httpx.Response:
    return ros.post(
        _token(actor),
        ENGAGEMENT,
        "ResolveReport",
        {"reportId": report_id, "status": status, "resolutionNote": note},
    )


def reports_of(admin: Actor, listing_id: str, status: str | None = None) -> list[dict]:
    return ros.ok(list_reports(admin, target_id=listing_id, status=status)).get("reports", [])


def wait_seller_listing_known(listing_id: str, timeout: float = 30.0) -> None:
    """The engagement service learns listing ownership from events; wait until it has the row."""
    deadline = time.monotonic() + timeout
    while time.monotonic() < deadline:
        out = ros.engagement_psql(
            f"SELECT count(*) FROM seller_listings WHERE listing_id = {ros.sql_lit(listing_id)}"
        )
        if out == "1":
            return
        time.sleep(1)
    raise AssertionError(f"engagement never learned the owner of listing {listing_id}")


def audit_events(admin: Actor, report_id: str) -> list[dict]:
    found = aud.events_for(None, "report", report_id, action="report.resolve", token=admin.token)  # type: ignore[arg-type]
    return [e for e in found if e.get("action") == "report.resolve"]
