"""Binds engagement/dispute_audit.feature (change engagement-dispute-audit, initiative q3-audit-log).

Black box through the gateway: CreateDispute / ResolveDispute / GetDispute are Connect JSON calls,
the audit trail is read with QueryAuditLog as admin via support/audit_support (import only).
"""

from __future__ import annotations

import httpx
from pytest_bdd import given, parsers, scenarios, then, when

from tests.e2e.step_definitions.common_steps import *  # noqa: F401,F403
from tests.e2e.step_definitions.disputes_access_steps import (  # noqa: F401
    _create_dispute,
    _order_seller_id,
    _post,
)
from tests.e2e.step_definitions.seller_analytics_access_steps import _subject
from tests.e2e.support import audit_support as aud
from tests.e2e.support.world import World

scenarios("engagement/dispute_audit.feature")


def _admin(world: World) -> tuple[str, str]:
    token = aud.admin_token(world)
    return token, _subject(token)


def _resolve(
    world: World, token: str, resolution: str, status: str = "DISPUTE_STATUS_RESOLVED"
) -> httpx.Response:
    resp = _post(
        world,
        token,
        "ResolveDispute",
        {
            "disputeId": world.state.extra["dispute_id"],
            "status": status,
            "resolution": resolution,
        },
    )
    world.state.extra["resp"] = resp
    return resp


@given("the buyer has an open dispute on the order")
def open_dispute(world: World) -> None:
    buyer = world.state.extra["seeded_buyer"]
    resp = _create_dispute(world, buyer.token, _order_seller_id(world))
    assert resp.status_code == 200, f"{resp.status_code}: {resp.text}"
    dispute = resp.json()["dispute"]
    world.state.extra["dispute_id"] = dispute["id"]
    world.state.extra["dispute_order_id"] = world.state.order_id


@given("an admin has moved the dispute to investigating")
def admin_investigates(world: World) -> None:
    token, admin_id = _admin(world)
    world.state.extra["admin_id"] = admin_id
    resp = _resolve(world, token, "looking into it", "DISPUTE_STATUS_INVESTIGATING")
    assert resp.status_code == 200, f"{resp.status_code}: {resp.text}"


@given("team-audit is stopped")
def audit_stopped(world: World) -> None:
    aud.stop_audit(world)


@when(parsers.parse('an admin resolves the dispute with resolution "{resolution}"'))
def admin_resolves(world: World, resolution: str) -> None:
    token, admin_id = _admin(world)
    world.state.extra["admin_id"] = admin_id
    _resolve(world, token, resolution)


@when(parsers.parse('an admin resolves the dispute again with resolution "{resolution}"'))
def admin_resolves_again(world: World, resolution: str) -> None:
    token, _ = _admin(world)
    _resolve(world, token, resolution)


@when("the buyer tries to resolve the dispute")
def buyer_resolves(world: World) -> None:
    _resolve(world, world.state.extra["seeded_buyer"].token, "buyer self-service")


@then("the dispute audit call succeeds")
def call_succeeds(world: World) -> None:
    resp: httpx.Response = world.state.extra["resp"]
    assert resp.status_code == 200, f"expected 200, got {resp.status_code}: {resp.text}"


@then(parsers.parse('the dispute audit call is refused with code "{code}"'))
def call_refused(world: World, code: str) -> None:
    resp: httpx.Response = world.state.extra["resp"]
    assert resp.status_code != 200, f"expected refusal, got 200: {resp.text}"
    assert resp.json().get("code") == code, f"expected {code}, got {resp.status_code}: {resp.text}"


@then(parsers.parse('the dispute audit call message is "{message}"'))
def call_message(world: World, message: str) -> None:
    resp: httpx.Response = world.state.extra["resp"]
    assert resp.json().get("message") == message, resp.text


@then(
    parsers.parse(
        "exactly one dispute.resolve audit event exists for the dispute with the admin as actor "
        'and resolution "{resolution}" and status "{status}"'
    )
)
def one_event(world: World, resolution: str, status: str) -> None:
    found = aud.events_for(
        world, "dispute", world.state.extra["dispute_id"], action="dispute.resolve"
    )
    found = [
        e
        for e in found
        if e.get("action") == "dispute.resolve"
        and (e.get("metadata") or {}).get("status") == status
    ]
    assert len(found) == 1, f"expected exactly 1 '{status}' dispute.resolve event, got {found}"
    event = found[0]
    assert event.get("actorId") == world.state.extra["admin_id"], event
    meta = event.get("metadata") or {}
    assert meta.get("resolution") == resolution, event
    assert meta.get("order_id") == world.state.extra["dispute_order_id"], event


def _assert_unresolved(world: World, statuses: tuple) -> None:
    buyer = world.state.extra["seeded_buyer"]
    resp = _post(world, buyer.token, "GetDispute", {"disputeId": world.state.extra["dispute_id"]})
    assert resp.status_code == 200, f"{resp.status_code}: {resp.text}"
    dispute = resp.json()["dispute"]
    assert dispute.get("status") in statuses, dispute
    assert dispute.get("status") != "DISPUTE_STATUS_RESOLVED", dispute


@then("the dispute is still open with an empty resolution")
def still_open(world: World) -> None:
    _assert_unresolved(world, (None, "", "DISPUTE_STATUS_OPEN"))
    buyer = world.state.extra["seeded_buyer"]
    resp = _post(world, buyer.token, "GetDispute", {"disputeId": world.state.extra["dispute_id"]})
    assert not resp.json()["dispute"].get("resolution"), resp.text


@then("the dispute is still investigating")
def still_investigating(world: World) -> None:
    _assert_unresolved(world, ("DISPUTE_STATUS_INVESTIGATING",))


@then("no dispute.resolve audit event exists for the dispute")
def no_event(world: World) -> None:
    aud.assert_no_event(
        world,
        target_type="dispute",
        target_id=world.state.extra["dispute_id"],
        action="dispute.resolve",
    )


@then("after team-audit is started again no dispute.resolve audit event exists for the dispute")
def no_event_after_restart(world: World) -> None:
    aud.start_audit(world)
    no_event(world)


@then(parsers.parse("the dispute has {count:d} dispute.resolve audit events in total"))
def event_total(world: World, count: int) -> None:
    found = aud.events_for(
        world, "dispute", world.state.extra["dispute_id"], action="dispute.resolve"
    )
    found = [e for e in found if e.get("action") == "dispute.resolve"]
    assert len(found) == count, f"want {count} dispute.resolve event(s), got {found}"


@then(
    "after team-audit is started again no resolved dispute.resolve audit event exists for the dispute"
)
def no_resolved_event_after_restart(world: World) -> None:
    aud.start_audit(world)
    found = aud.events_for(
        world, "dispute", world.state.extra["dispute_id"], action="dispute.resolve"
    )
    resolved = [
        e
        for e in found
        if e.get("action") == "dispute.resolve"
        and (e.get("metadata") or {}).get("status") == "resolved"
    ]
    assert not resolved, f"unexpected resolved event(s): {resolved}"
