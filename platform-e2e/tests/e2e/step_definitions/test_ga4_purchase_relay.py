"""Binds tracking/ga4_purchase_relay.feature (add-ga4-purchase-relay, capability ga4-purchase-relay)."""

from pytest_bdd import scenarios

from tests.e2e.step_definitions.ga4_purchase_relay_steps import *  # noqa: F401,F403

scenarios("tracking/ga4_purchase_relay.feature")
