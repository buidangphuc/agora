"""Binds frontend/buyer_sponsored_slots.feature (change buyer-sponsored-slots, initiative monetization)."""

from pytest_bdd import scenarios

from tests.e2e.step_definitions.buyer_sponsored_slots_steps import *  # noqa: F401,F403
from tests.e2e.step_definitions.common_steps import *  # noqa: F401,F403

scenarios("frontend/buyer_sponsored_slots.feature")
