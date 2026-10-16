"""Binds seller/listing_admin_audit.feature (change domain-admin-listing-audit, initiative q3-audit-log).

Black box through the gateway (Connect JSON). Audit assertions go through support/audit_support.
"""

from __future__ import annotations

import base64
import json
import uuid

import httpx
from pytest_bdd import given, parsers, scenarios, then, when

from config.settings import get_settings
from tests.e2e.support import audit_support as aud
from tests.e2e.support.world import World

scenarios("seller/listing_admin_audit.feature")

PASSWORD = "Sup3r-secret-pass!"
LISTING = "/platform.listing.v1.ListingService"
TARGET_TYPE = "listing"
ORIGINAL_TITLE = "Admin audit original"


def _x(world: World) -> dict:
    return world.state.extra


def _sub(token: str) -> str:
    payload = token.split(".")[1]
    payload += "=" * (-len(payload) % 4)
    return str(json.loads(base64.urlsafe_b64decode(payload))["sub"])


def _call(token: str | None, method: str, body: dict) -> httpx.Response:
    headers = {"Content-Type": "application/json"}
    if token:
        headers["Authorization"] = f"bearer {token}"
    url = f"{get_settings().gateway_url.rstrip('/')}{LISTING}/{method}"
    return httpx.post(url, json=body, headers=headers, timeout=30)


def _get(token: str, listing_id: str) -> httpx.Response:
    # Read as the admin: a rejected listing is hidden from the public view.
    return _call(token, "GetListing", {"id": listing_id})


def _update(token: str, listing_id: str, **changes) -> httpx.Response:
    current = _get(token, listing_id)
    assert current.status_code == 200, f"GetListing failed: {current.status_code} {current.text}"
    listing = current.json().get("listing") or {}
    listing.update(changes)
    return _call(token, "UpdateListing", {"listing": listing})


def _register_seller(world: World) -> str:
    username = f"e2e_seller_{uuid.uuid4().hex[:10]}"
    return world.service_factory.auth.register(username, PASSWORD, role="seller")


# ── Givens ───────────────────────────────────────────────────────────────
@given("a seller owns a listing for the admin audit check")
def seller_listing(world: World) -> None:
    token = _register_seller(world)
    resp = _call(
        token,
        "CreateListing",
        {
            "listing": {
                "title": ORIGINAL_TITLE,
                "categoryId": "cat-electronics",
                "price": 5_000_000,
                "stock": 10,
                "status": "LISTING_STATUS_PUBLISHED",
                "currency": "VND",
            }
        },
    )
    assert resp.status_code == 200, f"CreateListing failed: {resp.status_code} {resp.text}"
    listing_id = resp.json()["listing"]["id"]
    st = _x(world)
    st["la_before"] = (_get(aud.admin_token(world), listing_id).json().get("listing") or {}).get("status")
    st.update(la_owner=token, la_owner_id=_sub(token), la_listing=listing_id, la_admin=aud.admin_token(world))
    st["la_admin_id"] = _sub(st["la_admin"])
    world.add_cleanup(lambda: _call(token, "DeleteListing", {"id": listing_id}))


@given("another seller without the admin scope")
def other_seller(world: World) -> None:
    _x(world)["la_other"] = _register_seller(world)


@given("team-audit is stopped")
def audit_stopped(world: World) -> None:
    aud.stop_audit(world)


# ── Whens ────────────────────────────────────────────────────────────────
@when("the seeded admin rejects the listing through the gateway")
def admin_rejects(world: World) -> None:
    st = _x(world)
    st["la_resp"] = _update(st["la_admin"], st["la_listing"], status="LISTING_STATUS_REJECTED")


@when("the seeded admin deletes the listing through the gateway")
def admin_deletes(world: World) -> None:
    st = _x(world)
    st["la_resp"] = _call(st["la_admin"], "DeleteListing", {"id": st["la_listing"]})


@when("the seeded admin sends status rejected through UpdateListing")
def admin_sends_rejected(world: World) -> None:
    st = _x(world)
    st["la_resp"] = _update(st["la_admin"], st["la_listing"], status="LISTING_STATUS_REJECTED")


@when(parsers.parse('the owner renames the listing to "{title}"'))
def owner_renames(world: World, title: str) -> None:
    st = _x(world)
    st["la_resp"] = _update(st["la_owner"], st["la_listing"], title=title)


@when(parsers.parse('the other seller renames the listing to "{title}"'))
def other_renames(world: World, title: str) -> None:
    st = _x(world)
    st["la_resp"] = _update(st["la_other"], st["la_listing"], title=title)


@when("team-audit is started again")
def audit_started(world: World) -> None:
    aud.start_audit(world)


# ── Thens ────────────────────────────────────────────────────────────────
@then(parsers.parse('the update succeeded and the listing has status "{status}"'))
def update_status(world: World, status: str) -> None:
    st = _x(world)
    assert st["la_resp"].status_code == 200, f"{st['la_resp'].status_code} {st['la_resp'].text}"
    got = (_get(st["la_admin"], st["la_listing"]).json().get("listing") or {}).get("status")
    assert got == status, f"status={got!r}, want {status!r}"


@then(parsers.parse('the update succeeded and the listing has title "{title}"'))
def update_title(world: World, title: str) -> None:
    st = _x(world)
    assert st["la_resp"].status_code == 200, f"{st['la_resp'].status_code} {st['la_resp'].text}"
    got = (_get(st["la_admin"], st["la_listing"]).json().get("listing") or {}).get("title")
    assert got == title, f"title={got!r}, want {title!r}"


@then("the update succeeded and the listing keeps its stored status")
def update_keeps_status(world: World) -> None:
    st = _x(world)
    assert st["la_resp"].status_code == 200, f"{st['la_resp'].status_code} {st['la_resp'].text}"
    got = (_get(st["la_admin"], st["la_listing"]).json().get("listing") or {}).get("status")
    assert got == st["la_before"], f"status={got!r}, want unchanged {st['la_before']!r} (status is server-owned)"
    assert got != "LISTING_STATUS_REJECTED", "admin UpdateListing set the status"


@then("the delete succeeded and the listing is gone")
def delete_ok(world: World) -> None:
    st = _x(world)
    assert st["la_resp"].status_code == 200, f"{st['la_resp'].status_code} {st['la_resp'].text}"
    after = _get(st["la_admin"], st["la_listing"])
    assert after.status_code == 404 or after.json().get("code") == "not_found", after.text


@then(
    parsers.parse(
        'the audit log holds exactly one "{action}" event for the listing by the admin '
        'with the seller id and status "{status}"'
    )
)
def one_event_status(world: World, action: str, status: str) -> None:
    st = _x(world)
    aud.assert_one_event(
        world,
        target_type=TARGET_TYPE,
        target_id=st["la_listing"],
        action=action,
        actor_id=st["la_admin_id"],
        metadata={"seller_id": st["la_owner_id"], "status": status},
    )


@then(
    parsers.parse(
        'the audit log holds exactly one "{action}" event for the listing by the admin '
        "with the seller id and the stored status"
    )
)
def one_event_stored_status(world: World, action: str) -> None:
    st = _x(world)
    stored = st["la_before"].removeprefix("LISTING_STATUS_").lower()
    aud.assert_one_event(
        world,
        target_type=TARGET_TYPE,
        target_id=st["la_listing"],
        action=action,
        actor_id=st["la_admin_id"],
        metadata={"seller_id": st["la_owner_id"], "status": stored},
    )


@then(
    parsers.parse(
        'the audit log holds exactly one "{action}" event for the listing by the admin with the seller id'
    )
)
def one_event(world: World, action: str) -> None:
    st = _x(world)
    aud.assert_one_event(
        world,
        target_type=TARGET_TYPE,
        target_id=st["la_listing"],
        action=action,
        actor_id=st["la_admin_id"],
        metadata={"seller_id": st["la_owner_id"]},
    )


@then("the audit log holds no event for the listing")
def no_event(world: World) -> None:
    aud.assert_no_event(world, target_type=TARGET_TYPE, target_id=_x(world)["la_listing"])


@then(parsers.parse('the call fails with code "{code}"'))
def call_fails(world: World, code: str) -> None:
    resp = _x(world)["la_resp"]
    assert resp.status_code >= 400, f"expected failure, got {resp.status_code} {resp.text}"
    assert resp.json().get("code") == code, f"{resp.status_code} {resp.text}"


@then("the listing still has its original title")
def title_unchanged(world: World) -> None:
    got = (_get(_x(world)["la_admin"], _x(world)["la_listing"]).json().get("listing") or {}).get("title")
    assert got == ORIGINAL_TITLE, f"title={got!r}, want {ORIGINAL_TITLE!r}"
