"""Binds domain/my_listings_filter.feature (domain-my-listings-filter)."""

from pytest_bdd import scenarios

from tests.e2e.step_definitions.my_listings_filter_steps import *  # noqa: F401,F403

scenarios("domain/my_listings_filter.feature")
