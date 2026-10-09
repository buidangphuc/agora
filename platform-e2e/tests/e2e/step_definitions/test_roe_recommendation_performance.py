"""Binds analytics/recommendation_performance.feature."""

from pytest_bdd import scenarios

from tests.e2e.step_definitions.roe_steps import *  # noqa: F401,F403

scenarios("analytics/recommendation_performance.feature")
