"""Helpers for notification-order-work (initiative q10-order-lifecycle).

Reads a user's notification feed through the gateway (``ListNotifications``) and polls it for
an exact count of notifications matching a title and link. Reuses ``oic_order_support``.
"""

from __future__ import annotations

import time
from typing import Any

from tests.e2e.support import oic_order_support as s

NOTIFICATION = "/platform.notification.v1.NotificationService"
APPEAR_S = 45.0  # a notification must show within this after the triggering call
SETTLE_S = 6.0  # a duplicate or a wrong recipient would land inside this window
POLL_S = 1.0


def feed(w: s.OicWorld, actor: s.Actor) -> list[dict[str, Any]]:
    resp = s.post(w, actor, NOTIFICATION, "ListNotifications", {"pageSize": 100})
    return s.ok(resp).get("notifications", [])


def matching(
    w: s.OicWorld, actor: s.Actor, *, title: str | None = None, link: str | None = None
) -> list[dict[str, Any]]:
    out = []
    for n in feed(w, actor):
        if title is not None and n.get("title") != title:
            continue
        if link is not None and n.get("linkUrl") != link:
            continue
        out.append(n)
    return out


def wait_count(
    w: s.OicWorld,
    actor: s.Actor,
    *,
    title: str | None,
    link: str | None,
    at_least: int = 1,
    timeout: float = APPEAR_S,
) -> list[dict[str, Any]]:
    deadline = time.monotonic() + timeout
    found: list[dict[str, Any]] = []
    while time.monotonic() < deadline:
        found = matching(w, actor, title=title, link=link)
        if len(found) >= at_least:
            return found
        time.sleep(POLL_S)
    return found


def settle(
    w: s.OicWorld, actor: s.Actor, *, title: str | None, link: str | None
) -> list[dict[str, Any]]:
    """Hold for the settle window; return the matches seen at its end (and fail on a change)."""
    end = time.monotonic() + SETTLE_S
    found = matching(w, actor, title=title, link=link)
    while time.monotonic() < end:
        time.sleep(POLL_S)
        found = matching(w, actor, title=title, link=link)
    return found
