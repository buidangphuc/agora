"""Binds search/search_stock_read_model.feature to its step definitions."""

from pytest_bdd import scenarios

from tests.e2e.step_definitions.srm_stock_steps import *  # noqa: F401,F403

scenarios("search/search_stock_read_model.feature")
