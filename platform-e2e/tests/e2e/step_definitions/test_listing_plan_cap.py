"""Binds listing/listing_plan_cap.feature to its step definitions."""

from pytest_bdd import scenarios

from tests.e2e.step_definitions.listing_plan_cap_steps import *  # noqa: F401,F403

scenarios("listing/listing_plan_cap.feature")
