"""Binds frontend/seller_listing_status_ui.feature (change frontend-listing-moderation, initiative q6-trust-enforcement)."""

from pytest_bdd import scenarios

from tests.e2e.step_definitions.common_steps import *  # noqa: F401,F403
from tests.e2e.step_definitions.listing_moderation_ui_steps import *  # noqa: F401,F403

scenarios("frontend/seller_listing_status_ui.feature")
