"""Binds search/search_query_correctness.feature to its step definitions."""

from pytest_bdd import scenarios

from tests.e2e.step_definitions.srm_query_steps import *  # noqa: F401,F403

scenarios("search/search_query_correctness.feature")
