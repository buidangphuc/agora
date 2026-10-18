"""Binds frontend/order_lifecycle_ui.feature (change frontend-order-lifecycle, q10-order-lifecycle)."""

from pytest_bdd import scenarios

from tests.e2e.step_definitions.order_lifecycle_ui_steps import *  # noqa: F401,F403

scenarios("frontend/order_lifecycle_ui.feature")
