"""Steps for promotion-plan-billing: plan subscribe, cancel and lapse through the gateway.

Built on the payment-ledger steps (`plp_steps`) for the funded seller. The lapse scenario moves
current_period_end into the past in promotion_db and then waits for the stack's own renewal
sweep; no route is faked.
"""

from __future__ import annotations

import os
import time
from datetime import datetime, timedelta, timezone

from pytest_bdd import parsers, given, then, when

from tests.e2e.step_definitions.plp_steps import *  # noqa: F401,F403
from tests.e2e.step_definitions.plp_steps import _seller
from tests.e2e.support import flash_hold_support as fh
from tests.e2e.support import plp_support as p
from tests.e2e.support.oic_order_support import Actor, code_of, ok, post, register

SUBSCRIPTION = "/platform.promotion.v1.SubscriptionService"
PRO_PRICE = 199000
SWEEP_WAIT_S = float(os.getenv("PLAN_SWEEP_WAIT_S", "150"))


def _my_subscription(w, actor: Actor) -> dict | None:
    return ok(post(w, actor, SUBSCRIPTION, "GetMySubscription", {})).get("subscription") or None


def _entitlement_tier(w, actor: Actor) -> str:
    body = ok(post(w, actor, SUBSCRIPTION, "GetEntitlements", {"sellerId": actor.user_id}))
    return body.get("plan", "")


def _fees(w, actor: Actor) -> list[dict]:
    return p.rows(w, actor, "SUBSCRIPTION_FEE")


@given("a seller with no wallet funds")
def seller_without_funds(plp):
    register(plp, "seller", "seller")


@given(parsers.parse('another seller "{name}"'))
def another_seller(plp, name):
    register(plp, name, "seller")


@given(parsers.parse('the seller subscribes to "{plan}" through the gateway'))
@when(parsers.parse('the seller subscribes to "{plan}" through the gateway'))
def seller_subscribes(plp, plan):
    plp.responses["last"] = post(plp, _seller(plp), SUBSCRIPTION, "Subscribe", {"planId": plan})
    plp.data["subscribe_plan"] = plan


@when("the seller cancels the subscription through the gateway")
def seller_cancels(plp):
    plp.responses["last"] = post(plp, _seller(plp), SUBSCRIPTION, "CancelSubscription", {})
    ok(plp.responses["last"])


@when(parsers.parse('"{name}" cancels the subscription through the gateway'))
def other_cancels(plp, name):
    plp.responses["last"] = post(plp, plp.actors[name], SUBSCRIPTION, "CancelSubscription", {})


@when("the period end of the seller's subscription is brought forward and the sweep runs")
def bring_period_end_forward(plp):
    sub = _my_subscription(plp, _seller(plp))
    assert sub, "no subscription to bring forward"
    fh.psql(
        "UPDATE seller_subscriptions SET current_period_end = now() - interval '1 minute' "
        f"WHERE id = {fh.sql_lit(sub['id'])}"
    )
    deadline = time.monotonic() + SWEEP_WAIT_S
    while time.monotonic() < deadline:
        if _entitlement_tier(plp, _seller(plp)) == "PLAN_TIER_FREE":
            return
        time.sleep(3)


@then("the subscription call succeeds")
def call_succeeds(plp):
    ok(plp.responses["last"])


@then("the subscription call fails with FAILED_PRECONDITION")
def call_failed_precondition(plp):
    got = plp.responses["last"]
    assert code_of(got) == "failed_precondition", f"{got.status_code}: {got.text[:300]}"


@then("the seller's subscription is PRO ACTIVE with auto-renew and a period end about 30 days ahead")
def seller_is_pro(plp):
    sub = _my_subscription(plp, _seller(plp))
    assert sub, "GetMySubscription is empty after a successful Subscribe"
    assert sub.get("tier") == "PLAN_TIER_PRO", sub
    assert sub.get("status") == "SUBSCRIPTION_STATUS_ACTIVE", sub
    assert sub.get("autoRenew") is True, sub
    end = datetime.fromisoformat(sub["currentPeriodEnd"].replace("Z", "+00:00"))
    delta = end - datetime.now(timezone.utc)
    assert timedelta(days=29, hours=23) < delta <= timedelta(days=30, minutes=5), delta


@then(parsers.parse("the seller's ledger holds exactly one SUBSCRIPTION_FEE of {amount:d}"))
def one_fee(plp, amount):
    fees = _fees(plp, _seller(plp))
    assert len(fees) == 1, f"expected one SUBSCRIPTION_FEE, got {fees}"
    assert int(fees[0]["amount"]) == amount, fees[0]


@then("the seller has no subscription")
def seller_has_none(plp):
    assert _my_subscription(plp, _seller(plp)) is None


@then(parsers.parse('"{name}" has no subscription'))
def other_has_none(plp, name):
    assert _my_subscription(plp, plp.actors[name]) is None


@then("the seller's entitlements return FREE")
def entitlements_free(plp):
    assert _entitlement_tier(plp, _seller(plp)) == "PLAN_TIER_FREE"


@then("the first seller's subscription still has auto-renew")
def first_still_renews(plp):
    sub = _my_subscription(plp, _seller(plp))
    assert sub and sub.get("autoRenew") is True, sub
