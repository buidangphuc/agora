"""Binds order/checkout_principal.feature (port-edge-authz-residuals / order-checkout-correctness)."""

from pytest_bdd import scenarios

from tests.e2e.step_definitions.pear_authz_checkout_steps import *  # noqa: F401,F403

scenarios("order/checkout_principal.feature")
