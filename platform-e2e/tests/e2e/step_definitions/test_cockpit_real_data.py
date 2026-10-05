"""Binds ops/cockpit_real_data.feature (admin gate + real order figures)."""

from pytest_bdd import scenarios

from tests.e2e.step_definitions.cockpit_metrics_steps import *  # noqa: F401,F403
from tests.e2e.step_definitions.common_steps import *  # noqa: F401,F403

scenarios("ops/cockpit_real_data.feature")
