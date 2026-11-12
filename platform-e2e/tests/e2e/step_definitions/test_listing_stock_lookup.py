"""Binds search/listing_stock_lookup.feature to its step definitions."""

from pytest_bdd import scenarios

from tests.e2e.step_definitions.listing_stock_lookup_steps import *  # noqa: F401,F403

scenarios("search/listing_stock_lookup.feature")
