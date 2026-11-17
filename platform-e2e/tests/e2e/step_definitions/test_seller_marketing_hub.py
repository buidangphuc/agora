"""Binds seller/seller_marketing_hub.feature (change seller-marketing-hub, initiative seller-centre)."""

from pytest_bdd import scenarios

from tests.e2e.step_definitions.seller_marketing_hub_steps import *  # noqa: F401,F403

scenarios("seller/seller_marketing_hub.feature")
