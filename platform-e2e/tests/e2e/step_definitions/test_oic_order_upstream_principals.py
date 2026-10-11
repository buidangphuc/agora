"""Binds order/oic_upstream_principals.feature (port-order-inventory-correctness)."""

from pytest_bdd import scenarios

from tests.e2e.step_definitions.oic_order_steps import *  # noqa: F401,F403

scenarios("order/oic_upstream_principals.feature")
