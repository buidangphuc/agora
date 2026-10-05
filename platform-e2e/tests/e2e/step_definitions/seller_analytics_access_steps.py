"""Seller analytics access control (secure-seller-analytics-and-admin-seed).

Drives GetSellerFunnel / GetRevenueBreakdown / GetDemandForecast through the
gateway Connect API and asserts the status the data owner (team-analytics)
produced: 200 owner/admin, 403 other seller, 401 anonymous. The seller id is the
JWT subject (the principal id the service compares against).
Login steps ("an admin is logged in", "no one is logged in") live in common_steps.
"""

from __future__ import annotations

import base64
import json
import uuid

import httpx
from pytest_bdd import given, parsers, then, when

from tests.e2e.support.world import World

PASSWORD = "Sup3r-secret-pass!"


def _subject(token: str) -> str:
    """The JWT `sub` claim (the principal id). Unverified decode: ids only."""
    payload = token.split(".")[1]
    payload += "=" * (-len(payload) % 4)
    return json.loads(base64.urlsafe_b64decode(payload))["sub"]


def _register_seller(world: World) -> tuple[str, str]:
    """Register a fresh seller; returns (token, seller_id). Leaves the token active."""
    username = f"e2e_seller_{uuid.uuid4().hex[:10]}"
    token = world.service_factory.auth.register(username, PASSWORD, role="seller")
    return token, _subject(token)


@given("a signed-in seller")
def signed_in_seller(world: World) -> None:
    token, seller_id = _register_seller(world)
    world.service_factory.set_token(token)
    world.state.extra["seller_token"] = token
    world.state.extra["seller_id"] = seller_id


@given("another seller exists")
def another_seller_exists(world: World) -> None:
    own_token = world.state.extra["seller_token"]
    _, other_id = _register_seller(world)
    world.state.extra["other_seller_id"] = other_id
    world.service_factory.set_token(own_token)  # registering must not change who is acting


@when("the seller requests GetSellerFunnel for their own id")
def seller_reads_own_funnel(world: World) -> None:
    world.state.extra["analytics_resp"] = world.service_factory.analytics.funnel_response(
        world.state.extra["seller_id"]
    )


@when("the seller requests GetRevenueBreakdown for the other seller's id")
def seller_reads_other_revenue(world: World) -> None:
    world.state.extra["analytics_resp"] = world.service_factory.analytics.revenue_response(
        world.state.extra["other_seller_id"]
    )


@when("GetDemandForecast is requested for some seller without a token")
def anonymous_forecast(world: World) -> None:
    world.state.extra["analytics_resp"] = world.service_factory.analytics.forecast_response(
        f"seller-{uuid.uuid4().hex[:8]}"
    )


@when("the admin requests GetSellerFunnel for that seller's id")
def admin_reads_seller_funnel(world: World) -> None:
    world.state.extra["analytics_resp"] = world.service_factory.analytics.funnel_response(
        world.state.extra["seller_id"]
    )


@then(parsers.parse("the analytics gateway call returns {status:d} with a funnel"))
def returns_funnel(world: World, status: int) -> None:
    resp: httpx.Response = world.state.extra["analytics_resp"]
    assert resp.status_code == status, f"expected {status}, got {resp.status_code}: {resp.text}"
    # Connect/JSON omits zero-valued fields; the funnel object is simply a JSON object.
    assert isinstance(resp.json(), dict)


@then(parsers.parse("the analytics gateway call returns {status:d} and no revenue figures"))
def returns_denied_without_data(world: World, status: int) -> None:
    resp: httpx.Response = world.state.extra["analytics_resp"]
    assert resp.status_code == status, f"expected {status}, got {resp.status_code}: {resp.text}"
    body = resp.text
    assert "days" not in body and "topSkus" not in body and "top_skus" not in body


@then(parsers.parse("the analytics gateway call returns {status:d}"))
def returns_status(world: World, status: int) -> None:
    resp: httpx.Response = world.state.extra["analytics_resp"]
    assert resp.status_code == status, f"expected {status}, got {resp.status_code}: {resp.text}"
