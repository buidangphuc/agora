"""Binds feature_flags/checkout_kill_switch.feature to its step definitions."""

from pytest_bdd import scenarios

from tests.e2e.step_definitions.buyer_steps import *  # noqa: F401,F403
from tests.e2e.step_definitions.common_steps import *  # noqa: F401,F403
from tests.e2e.step_definitions.feature_flags_steps import *  # noqa: F401,F403

scenarios("feature_flags/checkout_kill_switch.feature")
