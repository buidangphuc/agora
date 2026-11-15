"""Binds promotion/ad_campaign_management.feature (change promotion-ads-campaign-mgmt).

Black box through the gateway (Connect JSON); campaign state is read through ListMyAdCampaigns.
"""

from __future__ import annotations

import httpx
from pytest_bdd import given, parsers, scenarios, then, when

from config.settings import get_settings
from src.utils import data as fake
from tests.e2e.step_definitions.test_sponsored_ads_steps import (  # noqa: F401
    _call,
    _my_campaign,
    _x,
    ads_campaign,
    ads_seller,
    buyer_clicks,
    buyer_lists,
    buyer_served,
    slots_absent,
    slots_contain,
)
from tests.e2e.support.world import World

scenarios("promotion/ad_campaign_management.feature")


def _manage(world: World, token: str, method: str) -> None:
    _x(world)["mgmt_resp"] = _call(token, method, {"campaignId": _x(world)["ad_campaign"]})


@given("a second seller")
def second_seller(world: World) -> None:
    _x(world)["ad_other"] = world.service_factory.auth.register(
        fake.unique_username("ads_other"), get_settings().seed_password, "seller"
    )


@when("the seller lists their campaigns in a new request")
def seller_lists(world: World) -> None:
    resp = _call(_x(world)["ad_seller"], "ListMyAdCampaigns", {})
    assert resp.status_code == 200, f"{resp.status_code} {resp.text}"


@when("the seller pauses the campaign")
def seller_pauses(world: World) -> None:
    _manage(world, _x(world)["ad_seller"], "PauseAdCampaign")


@when("the seller resumes the campaign")
def seller_resumes(world: World) -> None:
    _manage(world, _x(world)["ad_seller"], "ResumeAdCampaign")


@when("the second seller pauses the first seller's campaign")
def other_pauses(world: World) -> None:
    _manage(world, _x(world)["ad_other"], "PauseAdCampaign")


@when("the second seller resumes the first seller's campaign")
def other_resumes(world: World) -> None:
    _manage(world, _x(world)["ad_other"], "ResumeAdCampaign")


@then(parsers.parse('the campaign call succeeds with status "{status}"'))
def call_ok(world: World, status: str) -> None:
    resp: httpx.Response = _x(world)["mgmt_resp"]
    assert resp.status_code == 200, f"{resp.status_code} {resp.text}"
    campaign = resp.json().get("campaign") or {}
    assert campaign.get("id") == _x(world)["ad_campaign"], campaign
    assert campaign.get("status") == status, campaign


@then(parsers.parse('the campaign call fails with code "{code}"'))
def call_fails(world: World, code: str) -> None:
    resp: httpx.Response = _x(world)["mgmt_resp"]
    assert resp.status_code != 200, resp.text
    assert resp.json().get("code") == code, f"{resp.status_code} {resp.text}"


@then(parsers.parse('the seller\'s listed campaign reads status "{status}" and spent {spent:d}'))
def listed(world: World, status: str, spent: int) -> None:
    campaign = _my_campaign(world)
    assert campaign.get("status") == status, campaign
    assert int(campaign.get("spent", 0)) == spent, campaign
