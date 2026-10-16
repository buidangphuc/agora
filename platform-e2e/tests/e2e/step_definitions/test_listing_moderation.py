"""Binds seller/listing_moderation.feature (change domain-listing-moderation, initiative q6-trust-enforcement).

Black box through the gateway (Connect JSON). Audit assertions go through support/audit_support.
Flagged text holds the seeded keyword `replica` and the team-ai mock screen marker
`screen-review-marker`, so either screen path sends it to review.
"""

from __future__ import annotations

import time
import uuid

import httpx
from pytest_bdd import given, parsers, scenarios, then, when

from tests.e2e.support import audit_support as aud
from tests.e2e.support import srm_support as s
from tests.e2e.support.world import World

scenarios("seller/listing_moderation.feature")

PASSWORD = "Sup3r-secret-pass!"
LISTING = "/platform.listing.v1.ListingService/"
SEARCH = "/platform.search.v1.SearchService/SearchListings"
FLAGGED_MARKERS = "Replica screen-review-marker"
INDEX_S = 90.0
BAN_GONE_S = 60.0


def _x(world: World) -> dict:
    return world.state.extra


def _listing_body(title: str) -> dict:
    return {
        "listing": {
            "title": title,
            "description": "listing moderation e2e",
            "categoryId": "cat-laptop",
            "price": 100000,
            "stock": 3,
            "status": "LISTING_STATUS_PUBLISHED",
            "currency": "VND",
        }
    }


def _create(token: str, title: str) -> str:
    resp = s.post(LISTING + "CreateListing", _listing_body(title), token)
    assert resp.status_code == 200, f"CreateListing {resp.status_code}: {resp.text}"
    return resp.json()["listing"]["id"]


def _get(token: str | None, listing_id: str) -> httpx.Response:
    return s.post(LISTING + "GetListing", {"id": listing_id}, token)


def _status_of(token: str, listing_id: str) -> str | None:
    resp = _get(token, listing_id)
    assert resp.status_code == 200, f"GetListing {resp.status_code}: {resp.text}"
    return (resp.json().get("listing") or {}).get("status")


def _moderate(world: World, decision: str, reason: str = "", note: str = "") -> httpx.Response:
    st = _x(world)
    body = {"listingId": st["mod_listing"], "decision": decision}
    if reason:
        body["reasonCode"] = reason
    if note:
        body["note"] = note
    return s.post(LISTING + "ModerateListing", body, st["mod_admin"])


def _search_hits(query: str) -> list[str]:
    resp = s.post(SEARCH, {"query": query})
    assert resp.status_code == 200, f"SearchListings {resp.status_code}: {resp.text}"
    return [h["listingId"] for h in resp.json().get("hits", [])]


def _cleanup(world: World, listing_id: str) -> None:
    def run() -> None:
        try:
            s.post(LISTING + "DeleteListing", {"id": listing_id}, _x(world)["mod_admin"])
        except Exception:  # best effort: a banned listing may refuse deletes
            pass

    world.add_cleanup(run)


# ── Givens ───────────────────────────────────────────────────────────────
@given("a seller for moderation checks")
def seller(world: World) -> None:
    token = world.service_factory.auth.register(
        f"e2e_seller_{uuid.uuid4().hex[:10]}", PASSWORD, role="seller"
    )
    st = _x(world)
    st["mod_seller"] = token
    st["mod_seller_id"] = s.subject(token)
    st["mod_admin"] = aud.admin_token(world)
    st["mod_admin_id"] = s.subject(st["mod_admin"])
    st["mod_unique"] = f"modq{uuid.uuid4().hex[:12]}"


@given("the seller has a flagged listing awaiting review")
def flagged_pending(world: World) -> None:
    st = _x(world)
    listing_id = _create(st["mod_seller"], f"{FLAGGED_MARKERS} {st['mod_unique']}")
    st["mod_listing"] = listing_id
    _cleanup(world, listing_id)
    got = _status_of(st["mod_seller"], listing_id)
    assert got == "LISTING_STATUS_PENDING_REVIEW", f"flagged publish stored {got!r}, want pending_review"


@given("team-audit is stopped for moderation")
def audit_stopped(world: World) -> None:
    aud.stop_audit(world)


# ── Whens ────────────────────────────────────────────────────────────────
@when("the seller publishes a clean listing and a flagged listing through the gateway")
def publish_both(world: World) -> None:
    st = _x(world)
    st["mod_clean"] = _create(st["mod_seller"], f"Clean laptop stand {st['mod_unique']}")
    st["mod_flagged"] = _create(st["mod_seller"], f"{FLAGGED_MARKERS} {st['mod_unique']}")
    for lid in (st["mod_clean"], st["mod_flagged"]):
        _cleanup(world, lid)


@when(parsers.parse('the admin rejects the listing with reason "{reason}" and note "{note}"'))
def admin_rejects(world: World, reason: str, note: str) -> None:
    _x(world)["mod_resp"] = _moderate(world, "MODERATION_DECISION_REJECT", reason, note)


@when("the admin approves the listing")
def admin_approves(world: World) -> None:
    resp = _moderate(world, "MODERATION_DECISION_APPROVE")
    assert resp.status_code == 200, f"ModerateListing APPROVE {resp.status_code}: {resp.text}"


@when(parsers.parse('the admin bans the listing with reason "{reason}"'))
def admin_bans(world: World, reason: str) -> None:
    resp = _moderate(world, "MODERATION_DECISION_BAN", reason)
    assert resp.status_code == 200, f"ModerateListing BAN {resp.status_code}: {resp.text}"


@when("team-audit is started again for moderation")
def audit_started(world: World) -> None:
    aud.start_audit(world)


# ── Thens ────────────────────────────────────────────────────────────────
@then(parsers.parse('an anonymous reader gets the clean listing with status "{status}"'))
def anon_clean(world: World, status: str) -> None:
    got = _status_of(None, _x(world)["mod_clean"])
    assert got == status, f"anonymous sees clean listing as {got!r}, want {status!r}"


@then("an anonymous reader gets not_found for the flagged listing")
def anon_flagged(world: World) -> None:
    resp = _get(None, _x(world)["mod_flagged"])
    assert resp.status_code == 404 and resp.json().get("code") == "not_found", (
        f"flagged listing visible to anonymous: {resp.status_code} {resp.text}"
    )


@then(parsers.parse('the seller sees the flagged listing with status "{status}"'))
def seller_flagged(world: World, status: str) -> None:
    st = _x(world)
    got = _status_of(st["mod_seller"], st["mod_flagged"])
    assert got == status, f"owner sees flagged listing as {got!r}, want {status!r}"


@then(parsers.parse('the seller sees the listing with status "{status}"'))
def seller_listing_status(world: World, status: str) -> None:
    st = _x(world)
    got = _status_of(st["mod_seller"], st["mod_listing"])
    assert got == status, f"owner sees listing as {got!r}, want {status!r}"


@then("the moderation call succeeded")
def moderation_ok(world: World) -> None:
    resp = _x(world)["mod_resp"]
    assert resp.status_code == 200, f"ModerateListing {resp.status_code}: {resp.text}"


@then(parsers.parse('the moderation call fails with code "{code}"'))
def moderation_fails(world: World, code: str) -> None:
    resp = _x(world)["mod_resp"]
    assert resp.status_code >= 400, f"expected failure, got {resp.status_code} {resp.text}"
    assert resp.json().get("code") == code, f"{resp.status_code} {resp.text}"


@then(
    parsers.parse(
        'the audit log holds exactly one "{action}" event by the admin for the listing with decision "{decision}"'
    )
)
def one_event(world: World, action: str, decision: str) -> None:
    st = _x(world)
    aud.assert_one_event(
        world,
        target_type="listing",
        target_id=st["mod_listing"],
        action=action,
        actor_id=st["mod_admin_id"],
        metadata={
            "seller_id": st["mod_seller_id"],
            "decision": decision,
            "reason_code": "counterfeit",
            "note": "Fake logo",
            "from_status": "pending_review",
            "to_status": "rejected",
        },
    )


@then(parsers.parse('the audit log holds no "{action}" event for the listing'))
def no_event(world: World, action: str) -> None:
    aud.assert_no_event(
        world, target_type="listing", target_id=_x(world)["mod_listing"], action=action
    )


@then(
    parsers.parse(
        'the seller\'s own listings show the listing as "{status}" with reason "{reason}"'
    )
)
def own_listings(world: World, status: str, reason: str) -> None:
    st = _x(world)
    resp = s.post(LISTING + "ListMyListings", {"page": {"pageSize": 100}}, st["mod_seller"])
    assert resp.status_code == 200, f"ListMyListings {resp.status_code}: {resp.text}"
    mine = [lst for lst in resp.json().get("listings", []) if lst.get("id") == st["mod_listing"]]
    assert len(mine) == 1, f"listing missing from ListMyListings: {resp.text}"
    assert mine[0].get("status") == status, f"status={mine[0].get('status')!r}, want {status!r}"
    got = (mine[0].get("moderation") or {}).get("reasonCode")
    assert got == reason, f"moderation.reasonCode={got!r}, want {reason!r}"


@then(parsers.parse("the listing is found by a search within {seconds:d} seconds"))
def found_in_search(world: World, seconds: int) -> None:
    st = _x(world)
    s.eventually(
        lambda: st["mod_listing"] in _search_hits(st["mod_unique"]),
        "approved listing in search",
        timeout_s=float(seconds),
    )


@then(parsers.parse("the listing is absent from search within {seconds:d} seconds"))
def absent_from_search(world: World, seconds: int) -> None:
    st = _x(world)
    # Positive control already shown by the approval step: the same query returned the listing.
    deadline = time.monotonic() + seconds
    hits: list[str] = []
    while time.monotonic() < deadline:
        hits = _search_hits(st["mod_unique"])
        if st["mod_listing"] not in hits:
            return
        time.sleep(1.0)
    raise AssertionError(f"banned listing still in search after {seconds}s; hits={hits}")
