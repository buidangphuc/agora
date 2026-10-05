"""Notification delivery hardening (OpenSpec change `notification-delivery-hardening`).

Reuses the journey / alert / shop-display-name steps; adds only the sender-name
assertion and the team-notification restart. The restart scenario is `@destructive`
(serial only): it restarts the real container named by NOTIFICATION_CONTAINER and the
durable consumer state in Postgres must carry the price baseline across it.
"""

from __future__ import annotations

import time

import httpx
from pytest_bdd import given, parsers, then, when

from config.settings import get_settings
from tests.e2e.flows import restart_container
from tests.e2e.step_definitions.journey_steps import _poll_buyer_notification
from tests.e2e.support.world import World

SETTINGS = get_settings()

# Time for the consumer to read the listing's creation snapshot and record its price
# in listing_last_seen before the restart (a redelivered record is also safe: the
# ledger is durable).
_BASELINE_SETTLE_S = 8
_RESTART_READY_S = 60


@then(parsers.parse('the buyer\'s chat notification title contains "{name}"'))
def chat_title_contains(world: World, name: str) -> None:
    thread_id = world.state.extra["chat_thread_id"]
    found = _poll_buyer_notification(world, "NOTIFICATION_TYPE_CHAT", f"/chat/{thread_id}")
    assert found, "no chat notification for the buyer after the seller's reply"
    title = found[0].get("title", "")
    assert name in title, f"chat notification title {title!r} does not name the shop {name!r}"


@given("the notification consumer has recorded the listing's price")
def consumer_recorded_price(world: World) -> None:
    time.sleep(_BASELINE_SETTLE_S)


def _wait_notifications_ready(world: World) -> None:
    """Block until ListNotifications works through the gateway again."""
    token = world.state.current_user.token
    url = f"{SETTINGS.gateway_url.rstrip('/')}/platform.notification.v1.NotificationService/ListNotifications"
    deadline = time.monotonic() + _RESTART_READY_S
    last = "no response"
    while time.monotonic() < deadline:
        try:
            r = httpx.post(
                url,
                json={"pageSize": 1},
                headers={"Authorization": f"bearer {token}", "Content-Type": "application/json"},
                timeout=5,
            )
            if r.status_code == httpx.codes.OK:
                return
            last = f"HTTP {r.status_code}"
        except httpx.HTTPError as exc:
            last = str(exc)
        time.sleep(1)
    raise TimeoutError(f"team-notification not ready within {_RESTART_READY_S}s ({last})")


@when("team-notification is restarted")
def restart_notification(world: World) -> None:
    """Its in-memory state would start empty; the durable stores must not."""
    world.add_cleanup(lambda: _wait_notifications_ready(world))
    restart_container(SETTINGS.notification_container)
    _wait_notifications_ready(world)
