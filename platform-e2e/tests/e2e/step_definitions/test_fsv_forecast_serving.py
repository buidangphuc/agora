"""Binds analytics/forecast_serving.feature (analytics-forecast-serving)."""

from pytest_bdd import scenarios

from tests.e2e.step_definitions.fsv_steps import *  # noqa: F401,F403

scenarios("analytics/forecast_serving.feature")
