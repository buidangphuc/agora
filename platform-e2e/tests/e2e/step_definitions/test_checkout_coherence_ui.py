"""Binds frontend/checkout_coherence_ui.feature (change frontend-checkout-coherence, q11-checkout-coherence)."""

from pytest_bdd import scenarios

from tests.e2e.step_definitions.checkout_coherence_ui_steps import *  # noqa: F401,F403

scenarios("frontend/checkout_coherence_ui.feature")
