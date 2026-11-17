"""Steps for promotion/seller_erasure_monetization.feature (change promotion-seller-erasure-monetization).

A `UserErased` envelope is produced to the real `identity.events` topic and the team-promotion erasure
group is awaited; the plan is observed through GetMySubscription, GetEntitlements and the wallet ledger,
the campaign through ListSponsoredSlots, RecordAdClick and ListMyAdCampaigns. The only database write
moves the old period end into the past so the stack's own renewal sweep sees the row as due.
"""

from __future__ import annotations

import os
import time

from pytest_bdd import when

from tests.e2e.step_definitions.ad_campaign_management_steps import *  # noqa: F401,F403
from tests.e2e.step_definitions.seller_fee_charge_steps import *  # noqa: F401,F403
from tests.e2e.step_definitions.seller_plan_billing_steps import *  # noqa: F401,F403
from tests.e2e.step_definitions.plp_steps import _seller
from tests.e2e.step_definitions.seller_plan_billing_steps import _my_subscription
from tests.e2e.step_definitions.sponsored_ads_steps import _x
from tests.e2e.support import erasure_events_support as ev
from tests.e2e.support import flash_hold_support as fh
from tests.e2e.support.tii_support import jwt_subject
from tests.e2e.support.world import World

SWEEP_IDLE_S = float(os.getenv("PLAN_SWEEP_IDLE_S", "75"))


def _erase(user_id: str) -> None:
    ev.publish_user_erased(user_id, roles=("seller",))
    ev.wait_group_caught_up(ev.PROMOTION_GROUP)


@when("a UserErased event for the plan seller is consumed")
def erase_plan_seller(plp) -> None:
    sub = _my_subscription(plp, _seller(plp))
    assert sub and sub.get("id"), f"seller holds no subscription before erasure: {sub}"
    plp.data["erased_sub_id"] = sub["id"]
    _erase(_seller(plp).user_id)


@when("the old period end of the seller's subscription is brought forward and a sweep interval passes")
def bring_old_period_forward(plp) -> None:
    fh.psql(
        "UPDATE seller_subscriptions SET current_period_end = now() - interval '1 minute' "
        f"WHERE id = {fh.sql_lit(plp.data['erased_sub_id'])}"
    )
    time.sleep(SWEEP_IDLE_S)


@when("a UserErased event for the ads seller is consumed")
def erase_ads_seller(world: World) -> None:
    _erase(jwt_subject(_x(world)["ad_seller"]))
