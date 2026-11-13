"""Binds forecast/nightly_forecast.feature (forecast-nightly-job / demand-forecast-job)."""

from pytest_bdd import scenarios

from tests.e2e.step_definitions.fnj_steps import *  # noqa: F401,F403

scenarios("forecast/nightly_forecast.feature")
