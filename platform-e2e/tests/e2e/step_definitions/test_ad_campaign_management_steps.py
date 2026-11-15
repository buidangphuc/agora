"""Binds promotion/ad_campaign_management.feature (change promotion-ads-campaign-mgmt)."""

from pytest_bdd import scenarios

from tests.e2e.step_definitions.ad_campaign_management_steps import *  # noqa: F401,F403

scenarios("promotion/ad_campaign_management.feature")
