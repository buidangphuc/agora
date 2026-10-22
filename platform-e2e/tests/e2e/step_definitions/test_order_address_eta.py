"""Binds order/address_eta.feature (change order-address-eta, initiative q11-checkout-coherence)."""

from pytest_bdd import scenarios

from tests.e2e.step_definitions.order_address_eta_steps import *  # noqa: F401,F403

scenarios("order/address_eta.feature")
