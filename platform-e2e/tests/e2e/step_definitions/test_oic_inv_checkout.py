"""Binds frontend/oic_inv_checkout_idempotency.feature to its step definitions."""

from pytest_bdd import scenarios

from tests.e2e.step_definitions.oic_inv_checkout_steps import *  # noqa: F401,F403

scenarios("frontend/oic_inv_checkout_idempotency.feature")
