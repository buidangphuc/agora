"""Seller wallet access control (IDOR regression).

Drives the wallet RPCs through the gateway Connect API and asserts the status
team-payment produced: 200 owner (and admin reads), 403 anyone else. "a signed-in
seller" (registers a fresh seller, stores its token and id) comes from
seller_analytics_access_steps; "an admin is logged in" from common_steps.
"""

from __future__ import annotations

import uuid

import httpx
from pytest_bdd import given, parsers, then, when

from tests.e2e.support.world import World

PASSWORD = "Sup3r-secret-pass!"


@given("a signed-in buyer acting against that seller")
def buyer_acting_against_seller(world: World) -> None:
    username = f"e2e_buyer_{uuid.uuid4().hex[:10]}"
    token = world.service_factory.auth.register(username, PASSWORD, role="buyer")
    world.service_factory.set_token(token)


def _seller_id(world: World) -> str:
    return world.state.extra["seller_id"]


@when("the seller requests their wallet")
@when("the admin requests the seller's wallet")
def request_wallet(world: World) -> None:
    world.state.extra["wallet_resp"] = world.service_factory.payment.wallet_response(
        _seller_id(world)
    )


@when("the buyer requests a payout from the seller's wallet to their own bank account")
@when("the admin requests a payout from the seller's wallet")
def request_foreign_payout(world: World) -> None:
    world.state.extra["wallet_resp"] = world.service_factory.payment.payout_response(
        _seller_id(world)
    )


@when("the buyer requests the seller's ledger")
def request_foreign_ledger(world: World) -> None:
    world.state.extra["wallet_resp"] = world.service_factory.payment.ledger_response(
        _seller_id(world)
    )


@then(parsers.parse("the wallet call returns {status:d}"))
def wallet_call_returns(world: World, status: int) -> None:
    resp: httpx.Response = world.state.extra["wallet_resp"]
    assert resp.status_code == status, f"expected {status}, got {resp.status_code}: {resp.text}"


@then("the seller's payout history has no payout")
def seller_has_no_payout(world: World) -> None:
    world.service_factory.set_token(world.state.extra["seller_token"])
    history = world.service_factory.payment.list_payout_history(_seller_id(world))
    assert not history.get("payouts"), f"a foreign payout was booked: {history}"
