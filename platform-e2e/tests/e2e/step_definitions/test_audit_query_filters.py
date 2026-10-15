"""Binds security/audit_query_filters.feature (change audit-query-filters, initiative q3-audit-log).

Black box through the gateway; rows are seeded by direct SQL via support/audit_support.
"""

from __future__ import annotations

import uuid

import pytest
from pytest_bdd import given, parsers, scenarios, then, when

from tests.e2e.support import audit_support as aud
from tests.e2e.support.world import World

scenarios("security/audit_query_filters.feature")

PASSWORD = "Sup3r-secret-pass!"
DAY = "2026-01-01T"
ACTORS_ACTIONS = [
    ("adm-1", "order.force_fail"),  # matches all three filters
    ("adm-1", "order.admin_status_update"),  # actor + target only
    ("adm-2", "order.force_fail"),  # action + target only
    ("adm-2", "order.admin_status_update"),  # target only
]


def _x(world: World) -> dict:
    return world.state.extra


def _t(world: World, label: str) -> tuple[str, str]:
    return _x(world)["aud_targets"][label]


def _new_target(world: World, label: str) -> tuple[str, str]:
    tt, tid = aud.fresh_target()
    _x(world).setdefault("aud_targets", {})[label] = (tt, tid)
    return tt, tid


def _ts(clock: str) -> str:
    return f"{DAY}{clock}Z"


def _events(resp) -> list[dict]:
    assert resp.status_code == 200, f"{resp.status_code} {resp.text}"
    return resp.json().get("events", [])


def _total(resp) -> int:
    return int((resp.json().get("page") or {}).get("total", 0))


# ── Givens ───────────────────────────────────────────────────────────────
@given("the seeded admin")
def seeded_admin(world: World) -> None:
    _x(world)["aud_admin"] = aud.admin_token(world)


@given("a logged-in seller")
def logged_in_seller(world: World) -> None:
    username = f"e2e_seller_{uuid.uuid4().hex[:10]}"
    _x(world)["aud_seller"] = world.service_factory.auth.register(username, PASSWORD, role="seller")


@given("an anonymous client")
def anonymous(world: World) -> None:
    _x(world)["aud_anon"] = None


@given('a fresh target "A" with 2 events and a fresh target "B" with 1 event')
def two_targets(world: World) -> None:
    tt_a, id_a = _new_target(world, "A")
    # B shares A's target_type: only target_id tells them apart.
    _, id_b = aud.fresh_target()
    tt_b = tt_a
    _x(world)["aud_targets"]["B"] = (tt_b, id_b)
    ids = aud.seed_events(
        [
            {"action": "listing.update", "target_type": tt_a, "target_id": id_a, "created_at": _ts("10:00:00")},
            {"action": "listing.update", "target_type": tt_a, "target_id": id_a, "created_at": _ts("11:00:00")},
            {"action": "listing.update", "target_type": tt_b, "target_id": id_b, "created_at": _ts("12:00:00")},
        ]
    )
    _x(world)["aud_ids"] = ids


@given(
    parsers.parse('a fresh target "A" with events of actions "{a1}", "{a2}" and "{a3}"')
)
def action_variants(world: World, a1: str, a2: str, a3: str) -> None:
    tt, tid = _new_target(world, "A")
    aud.seed_events(
        [{"action": a, "target_type": tt, "target_id": tid} for a in (a1, a2, a3)]
    )


@given('a fresh target "A" with events for actor and action combinations')
def combos(world: World) -> None:
    tt, tid = _new_target(world, "A")
    aud.seed_events(
        [
            {"actor_id": actor, "action": action, "target_type": tt, "target_id": tid}
            for actor, action in ACTORS_ACTIONS
        ]
    )
    # Same actor + action on another target must not leak in.
    tt2, tid2 = aud.fresh_target()
    aud.seed_events(
        [{"actor_id": "adm-1", "action": "order.force_fail", "target_type": tt2, "target_id": tid2}]
    )


@given('a fresh target "A" with events at 10:00:00, 11:00:00 and 12:00:00')
def timed(world: World) -> None:
    tt, tid = _new_target(world, "A")
    aud.seed_events(
        [
            {"action": "listing.update", "target_type": tt, "target_id": tid, "created_at": _ts(c)}
            for c in ("10:00:00", "11:00:00", "12:00:00")
        ]
    )


@given(parsers.parse('a fresh target "A" with {n:d} events'))
def n_events(world: World, n: int) -> None:
    tt, tid = _new_target(world, "A")
    # Decoys: same target_type, other target ids, so only target_id narrows the result.
    aud.seed_events(
        [
            {"action": "listing.update", "target_type": tt, "target_id": f"{tid}-other{i}"}
            for i in range(3)
        ]
    )
    aud.seed_events(
        [
            {
                "action": "listing.update",
                "target_type": tt,
                "target_id": tid,
                "created_at": f"2026-01-01T10:{i:02d}:00Z",
            }
            for i in range(n)
        ]
    )


# ── Whens ────────────────────────────────────────────────────────────────
@when('the admin queries the audit log for target "A"')
def q_target(world: World) -> None:
    tt, tid = _t(world, "A")
    _x(world)["aud_resp"] = aud.query_events(_x(world)["aud_admin"], target_type=tt, target_id=tid)


@when('the helper lists the events of target "A"')
def helper_lists(world: World) -> None:
    tt, tid = _t(world, "A")
    _x(world)["aud_list"] = aud.events_for(world, tt, tid)
    _x(world)["aud_resp"] = None


@when(parsers.parse('the admin queries the audit log for target "A" with action "{action}"'))
def q_target_action(world: World, action: str) -> None:
    tt, tid = _t(world, "A")
    _x(world)["aud_resp"] = aud.query_events(
        _x(world)["aud_admin"], target_type=tt, target_id=tid, action=action
    )


@when(parsers.parse('the admin queries target "A" with actor "{actor}" and action "{action}"'))
def q_actor_action(world: World, actor: str, action: str) -> None:
    tt, tid = _t(world, "A")
    _x(world)["aud_resp"] = aud.query_events(
        _x(world)["aud_admin"], target_type=tt, target_id=tid, actor_id=actor, action=action
    )


@when(parsers.parse('the admin queries target "A" created after {after} and before {before}'))
def q_range(world: World, after: str, before: str) -> None:
    tt, tid = _t(world, "A")
    _x(world)["aud_resp"] = aud.query_events(
        _x(world)["aud_admin"],
        target_type=tt,
        target_id=tid,
        created_after=_ts(after),
        created_before=_ts(before),
    )


@when(parsers.parse('the admin queries the audit log for target id "{target_id}"'))
def q_unknown_target_id(world: World, target_id: str) -> None:
    _x(world)["aud_resp"] = aud.query_events(_x(world)["aud_admin"], target_id=target_id)


@when(parsers.parse('the admin queries target "A" with page size {size:d} and then follows the next cursor'))
def q_paged(world: World, size: int) -> None:
    tt, tid = _t(world, "A")
    tok = _x(world)["aud_admin"]
    first = aud.query_events(tok, target_type=tt, target_id=tid, page_size=size)
    _events(first)
    cursor = (first.json().get("page") or {}).get("nextCursor")
    assert cursor, f"page 1 has no next cursor: {first.text}"
    second = aud.query_events(tok, target_type=tt, target_id=tid, page_size=size, cursor=cursor)
    _x(world)["aud_pages"] = [first, second]


@when(parsers.parse('the seller queries the audit log with target type "{tt}" and target id "{tid}"'))
def seller_queries(world: World, tt: str, tid: str) -> None:
    _x(world)["aud_resp"] = aud.query_events(_x(world)["aud_seller"], target_type=tt, target_id=tid)


@when(parsers.parse('the anonymous client queries the audit log with action "{action}"'))
def anon_queries(world: World, action: str) -> None:
    _x(world)["aud_resp"] = aud.query_events(None, action=action)


@when('the helper queries target "A" with the seller token')
def helper_seller(world: World) -> None:
    tt, tid = _t(world, "A")
    _x(world)["aud_resp"] = aud.query_events(_x(world)["aud_seller"], target_type=tt, target_id=tid)


@when("team-audit is stopped through the helper")
def stop(world: World) -> None:
    aud.stop_audit(world)


@when("team-audit is started again through the helper")
def start(world: World) -> None:
    aud.start_audit(world)


@when("the scenario teardown runs")
def teardown_now(world: World) -> None:
    world.cleanup()


# ── Thens ────────────────────────────────────────────────────────────────
@then(parsers.parse('exactly the {n:d} events of target "A" are returned, newest first'))
def exactly_target_events(world: World, n: int) -> None:
    x = _x(world)
    events = x["aud_list"] if x.get("aud_resp") is None else _events(x["aud_resp"])
    tt, tid = _t(world, "A")
    assert len(events) == n, f"expected {n} events, got {events}"
    assert all(e["targetType"] == tt and e["targetId"] == tid for e in events), events
    stamps = [e["createdAt"] for e in events]
    assert stamps == sorted(stamps, reverse=True), f"not newest first: {stamps}"


@then(parsers.parse("the page total is {n:d}"))
def page_total(world: World, n: int) -> None:
    assert _total(_x(world)["aud_resp"]) == n, _x(world)["aud_resp"].text


@then(parsers.parse('only the events with action "{action}" are returned'))
def only_action(world: World, action: str) -> None:
    events = _events(_x(world)["aud_resp"])
    assert events, "expected at least the exact-match event"
    assert {e["action"] for e in events} == {action}, events


@then(parsers.parse('only the event matching actor "{actor}" and action "{action}" is returned'))
def only_actor_action(world: World, actor: str, action: str) -> None:
    resp = _x(world)["aud_resp"]
    events = _events(resp)
    assert len(events) == 1, f"expected exactly one event, got {events}"
    assert events[0]["actorId"] == actor and events[0]["action"] == action, events
    assert events[0]["targetId"] == _t(world, "A")[1], events
    assert _total(resp) == 1, resp.text


@then("the events at 11:00:00 and 10:00:00 are returned in that order")
def range_events(world: World) -> None:
    stamps = [e["createdAt"] for e in _events(_x(world)["aud_resp"])]
    assert stamps == [_ts("11:00:00"), _ts("10:00:00")], stamps


@then("the call succeeds with no events, total 0 and no next cursor")
def empty_page(world: World) -> None:
    resp = _x(world)["aud_resp"]
    assert _events(resp) == [], resp.text
    page = resp.json().get("page") or {}
    assert int(page.get("total", 0)) == 0, resp.text
    assert not page.get("nextCursor"), resp.text


@then("page 1 holds 20 events with total 25 and a next cursor")
def page_one(world: World) -> None:
    first = _x(world)["aud_pages"][0]
    assert len(_events(first)) == 20, first.text
    assert _total(first) == 25, first.text
    assert (first.json().get("page") or {}).get("nextCursor"), first.text


@then("page 2 holds the 5 oldest events and no next cursor")
def page_two(world: World) -> None:
    first, second = _x(world)["aud_pages"]
    assert len(_events(second)) == 5, second.text
    assert not (second.json().get("page") or {}).get("nextCursor"), second.text
    oldest = sorted(e["createdAt"] for e in _events(first) + _events(second))[:5]
    assert [e["createdAt"] for e in _events(second)] == sorted(oldest, reverse=True)


@then("the events are newest first throughout")
def newest_first(world: World) -> None:
    first, second = _x(world)["aud_pages"]
    stamps = [e["createdAt"] for e in _events(first) + _events(second)]
    assert stamps == sorted(stamps, reverse=True), stamps
    assert len(set(stamps)) == 25, "expected 25 distinct events across both pages"


@then("the gateway answers HTTP 400 with code invalid_argument")
def http_400(world: World) -> None:
    resp = _x(world)["aud_resp"]
    assert resp.status_code == 400, f"{resp.status_code} {resp.text}"
    assert resp.json().get("code") == "invalid_argument", resp.text


@then(parsers.parse("the gateway answers HTTP {status:d} and no event is returned"))
def http_refused(world: World, status: int) -> None:
    resp = _x(world)["aud_resp"]
    assert resp.status_code == status, f"expected {status}, got {resp.status_code} {resp.text}"
    body = resp.json() if resp.content else {}
    assert not body.get("events"), f"events leaked to a refused caller: {resp.text}"


@then("a later admin QueryAuditLog through the gateway returns HTTP 200")
def later_200(world: World) -> None:
    resp = aud.query_events(_x(world)["aud_admin"], page_size=1)
    assert resp.status_code == 200, f"{resp.status_code} {resp.text}"


@then("stopping team-audit twice through the helper is harmless")
def stop_twice(world: World) -> None:
    aud.stop_audit(world)
    aud.stop_audit(world)  # idempotent: no docker error
    aud.start_audit(world)
    later_200(world)


@then("the helper query raises AuditUnavailable instead of returning an empty list")
def query_raises(world: World) -> None:
    tt, tid = _t(world, "A")
    with pytest.raises(aud.AuditUnavailable):
        aud.query_events(_x(world)["aud_admin"], target_type=tt, target_id=tid)


@then("the helper events_for raises AuditUnavailable instead of returning an empty list")
def events_for_raises(world: World) -> None:
    tt, tid = _t(world, "A")
    with pytest.raises(aud.AuditUnavailable):
        aud.events_for(world, tt, tid, token=_x(world)["aud_admin"])
