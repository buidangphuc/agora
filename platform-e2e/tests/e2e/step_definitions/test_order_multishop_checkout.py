"""Binds order/multishop_checkout.feature (change order-multishop-checkout, initiative q11-checkout-coherence)."""

from pytest_bdd import scenarios

from tests.e2e.step_definitions.order_multishop_checkout_steps import *  # noqa: F401,F403

scenarios("order/multishop_checkout.feature")
