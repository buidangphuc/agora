"""Binds promo/flash_sale_ownership.feature to its step definitions."""

from pytest_bdd import scenarios

from tests.e2e.step_definitions.flash_sale_ownership_steps import *  # noqa: F401,F403

scenarios("promo/flash_sale_ownership.feature")
