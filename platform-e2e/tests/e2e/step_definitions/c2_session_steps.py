"""Extra session-revocation steps (area c2): sid binding and denylist expiry."""

from __future__ import annotations

import time
import uuid

import pytest_bdd
from pytest_bdd import parsers, then, when

from config.settings import get_settings
from src.api.services import AuthService, SessionService
from tests.e2e.flows import session_id_of
from tests.e2e.support import c2_identity_events as ev
from tests.e2e.support.world import World

SETTINGS = get_settings()
PRUNE_DEADLINE_S = 150.0


@when("the buyer logs in directly at the gateway")
def c2_login_directly(world: World) -> None:
    extra = world.state.extra
    token = AuthService().login(extra["username"], extra["password"])
    assert token, "login returned no token"
    extra["login_token"] = token


@then("the token's sid equals the id of a session ListSessions returns for the buyer")
def c2_sid_is_listed(world: World) -> None:
    token = world.state.extra["login_token"]
    sid = session_id_of(token)
    assert sid, "the user token carries no sid claim"
    svc = SessionService(token=token)
    try:
        listed = {s["id"] for s in svc.list_sessions()}
    finally:
        svc.close()
    assert sid in listed, f"sid {sid} is not among the buyer's sessions {listed}"


def _wait_size(predicate, deadline_s: float) -> int:
    end = time.monotonic() + deadline_s
    last = None
    while time.monotonic() < end:
        last = ev.denylist_size()
        if last is not None and predicate(last):
            return last
        time.sleep(2)
    raise AssertionError(f"denylist size never satisfied the condition (last {last})")


@pytest_bdd.given("the gateway's denylist size is recorded")
def c2_record_size(world: World) -> None:
    size = ev.denylist_size()
    assert size is not None, "Prometheus has no gateway_revocation_denylist_size sample"
    world.state.extra["denylist_before"] = size


@when(
    parsers.parse(
        "a SessionRevoked event expiring in {seconds:d} seconds is published to identity.events"
    )
)
def c2_publish_event(world: World, seconds: int) -> None:
    ev.publish_session_revoked(
        SETTINGS.kafka_brokers,
        f"e2e-sid-{uuid.uuid4().hex}",
        f"e2e-user-{uuid.uuid4().hex[:8]}",
        seconds,
    )
    world.state.extra["published_at"] = time.monotonic()


@then("the gateway denylist grows by that entry")
def c2_grows(world: World) -> None:
    before = world.state.extra["denylist_before"]
    peak = _wait_size(lambda n: n > before, 30)
    world.state.extra["denylist_peak"] = peak


@then("once the expiry has passed the gateway denylist no longer holds it")
def c2_pruned(world: World) -> None:
    peak = world.state.extra["denylist_peak"]
    # The gateway sweeps once a minute; the entry is gone within one sweep after expiry.
    size = _wait_size(lambda n: n < peak, PRUNE_DEADLINE_S)
    world.logger.info(
        f"denylist {peak} -> {size} after {time.monotonic() - world.state.extra['published_at']:.0f}s"
    )
