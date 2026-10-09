"""Real-failure injection for the ui-* failure-path scenarios (area uif).

A failure path is produced by genuinely stopping (or pausing, for "slow") an `agora` stack
container and restoring it in the scenario teardown; nothing in the product is faked. Every
container is checked to belong to the `agora` compose project before it is touched, and the
restore waits until the gateway reaches the upstream again (gRPC peers re-resolve a restarted
container only after ~30 s), so the next scenario starts on a healthy stack.

Env overrides: ``UIF_<SERVICE>_CONTAINER`` (default ``agora-<service>-svc``).
"""

from __future__ import annotations

import time
from collections.abc import Callable

import httpx

from tests.e2e.flows import stop_container
from tests.e2e.support import pear_edge_support as pe
from tests.e2e.support.world import World

PROJECT = "agora"
# Next's own (empty) route announcer is also a role=alert.
ALERTS = '[role="alert"]:not(#__next-route-announcer__)'
_UNREACHED = {"unavailable", "deadline_exceeded", "internal", "unknown"}


def base(world: World) -> str:
    return world.settings.base_url.rstrip("/")


def container(service: str) -> str:
    import os

    key = service.upper().replace("-", "_")
    return os.getenv(f"UIF_{key}_CONTAINER", f"agora-{service}-svc")


def assert_agora(name: str) -> None:
    out = pe.docker(
        "inspect", "-f", '{{index .Config.Labels "com.docker.compose.project"}}', name
    ).stdout.strip()
    assert out == PROJECT, f"{name} belongs to compose project {out!r}, refusing to touch it"


# ── probes: has the gateway reached the upstream again? ──────────────────
def _reached(resp: httpx.Response) -> bool:
    return resp.status_code < 500 and pe.connect_code(resp) not in _UNREACHED


def _probe_with(path: str, body: dict, role: str | None, own_id: str = "") -> Callable[[], bool]:
    def probe() -> bool:
        token = None
        payload = dict(body)
        if role:
            from tests.e2e.support import d2_support as d2

            token = d2.register(role)[1]
            if own_id:
                payload[own_id] = d2.principal_id(token)
        return _reached(pe.post_json(path, payload, token, timeout=10))

    return probe


PROBES: dict[str, Callable[[], bool]] = {
    "team-domain": _probe_with("/platform.listing.v1.ListingService/GetListing", {"id": "x"}, None),
    "team-order": _probe_with("/platform.order.v1.CartService/GetCart", {}, "buyer"),
    "team-ai": _probe_with(
        "/platform.ai.v1.AIService/SummarizeReviews",
        {"listingId": "x", "reviews": [{"rating": 5, "comment": "ok"}]},
        "buyer",
    ),
    "team-identity": _probe_with(
        "/platform.identity.v1.AuthService/Login", {"username": "uif", "password": "uif"}, None
    ),
    "team-notification": _probe_with(
        "/platform.notification.v1.NotificationService/ListNotifications", {}, "buyer"
    ),
    "team-analytics": _probe_with(
        "/platform.analytics.v1.AnalyticsQueryService/GetSellerFunnel",
        {},
        "seller",
        own_id="sellerId",
    ),
    "team-search": _probe_with("/platform.search.v1.SearchService/SearchListings", {}, None),
    "team-engagement": _probe_with(
        "/platform.engagement.v1.EngagementService/GetListingStats", {"listingId": "x"}, None
    ),
}


def wait_reached(service: str, timeout_s: float = 150.0) -> None:
    probe = PROBES[service]
    deadline = time.monotonic() + timeout_s
    last: object = "no answer"
    while time.monotonic() < deadline:
        try:
            if probe():
                return
        except Exception as exc:  # noqa: BLE001 - still coming back up
            last = exc
        time.sleep(2)
    raise TimeoutError(f"the gateway did not reach {service} again within {timeout_s}s ({last})")


def stop(world: World, service: str) -> None:
    """Stop the service's container; teardown starts it again and waits until it answers."""
    name = container(service)
    assert_agora(name)
    restore = stop_container(name)

    def restore_and_wait() -> None:
        restore()
        pe.wait_healthy(name, 120)
        wait_reached(service)

    world.add_cleanup(restore_and_wait)
    world.state.extra.setdefault("uif_restorers", {})[service] = restore_and_wait


def restore_now(world: World, service: str) -> None:
    """Bring a stopped/paused service back mid-scenario (the teardown then finds it healthy)."""
    world.state.extra["uif_restorers"][service]()


def pause(world: World, service: str) -> Callable[[], None]:
    """`docker pause` the container (its callers hang instead of failing fast).

    Returns the function that unpauses it now; teardown unpauses it again (idempotent).
    """
    name = container(service)
    assert_agora(name)
    pe.docker("pause", name)

    def unpause() -> None:
        pe.docker("unpause", name, check=False)

    def restore_and_wait() -> None:
        unpause()
        pe.wait_healthy(name, 120)
        wait_reached(service)

    world.add_cleanup(restore_and_wait)
    world.state.extra.setdefault("uif_restorers", {})[service] = restore_and_wait
    return unpause
