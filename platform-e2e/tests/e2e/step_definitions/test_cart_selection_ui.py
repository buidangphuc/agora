"""Binds frontend/cart_selection_ui.feature (change frontend-cart-selection, q11-checkout-coherence)."""

from pytest_bdd import scenarios

from tests.e2e.step_definitions.cart_selection_ui_steps import *  # noqa: F401,F403

scenarios("frontend/cart_selection_ui.feature")
