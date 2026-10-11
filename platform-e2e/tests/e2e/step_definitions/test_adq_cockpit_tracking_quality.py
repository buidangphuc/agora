"""Binds ops/cockpit_tracking_quality.feature."""

from pytest_bdd import scenarios

from tests.e2e.step_definitions.adq_cockpit_steps import *  # noqa: F401,F403
from tests.e2e.step_definitions.common_steps import *  # noqa: F401,F403

scenarios("ops/cockpit_tracking_quality.feature")
