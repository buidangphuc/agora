"""Binds search/search_read_model_deletes.feature to its step definitions."""

from pytest_bdd import scenarios

from tests.e2e.step_definitions.srm_deletes_steps import *  # noqa: F401,F403

scenarios("search/search_read_model_deletes.feature")
