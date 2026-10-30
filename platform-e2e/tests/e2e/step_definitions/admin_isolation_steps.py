"""Steps for frontend/admin_isolation.feature (team-frontend-admin-app)."""

from __future__ import annotations

import httpx
from pytest_bdd import parsers, then, when

from tests.e2e.support.world import World


@when(
    parsers.parse(
        'the buyer storefront is requested at "{a}" and "{b}" without following redirects'
    )
)
def request_buyer_paths(world: World, a: str, b: str) -> None:
    base = world.settings.base_url.rstrip("/")
    with httpx.Client(follow_redirects=False, timeout=30) as client:
        world.state.extra["admin_isolation"] = {p: client.get(f"{base}{p}") for p in (a, b)}


@then("both buyer storefront responses are HTTP 404")
def both_not_found(world: World) -> None:
    got = {p: r.status_code for p, r in world.state.extra["admin_isolation"].items()}
    assert all(code == 404 for code in got.values()), got
