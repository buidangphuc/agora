"""Binds search/search_best_selling_rating.feature to its step definitions."""

from pytest_bdd import scenarios

from tests.e2e.step_definitions.search_best_selling_rating_steps import *  # noqa: F401,F403

scenarios("search/search_best_selling_rating.feature")
