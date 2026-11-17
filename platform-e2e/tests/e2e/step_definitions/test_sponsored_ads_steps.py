"""Binds ads/sponsored_serving.feature and ads/sponsored_click.feature (change promotion-ads-serving)."""

from pytest_bdd import scenarios

from tests.e2e.step_definitions.sponsored_ads_steps import *  # noqa: F401,F403

scenarios("ads/sponsored_serving.feature", "ads/sponsored_click.feature")
