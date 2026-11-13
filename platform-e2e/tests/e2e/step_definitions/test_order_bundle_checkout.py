"""Binds order/bundle_checkout.feature (change order-bundle-checkout, initiative seller-centre)."""

from pytest_bdd import scenarios

from tests.e2e.step_definitions.order_bundle_checkout_steps import *  # noqa: F401,F403

scenarios("order/bundle_checkout.feature")
