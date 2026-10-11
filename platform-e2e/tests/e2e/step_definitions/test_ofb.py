"""Binds the order-facts-buyer features (order-facts, feature-materialization)."""

from pytest_bdd import scenarios

from tests.e2e.step_definitions.ofb_steps import *  # noqa: F401,F403

scenarios("analytics/order_facts_buyer.feature")
scenarios("featurestore/order_features.feature")
