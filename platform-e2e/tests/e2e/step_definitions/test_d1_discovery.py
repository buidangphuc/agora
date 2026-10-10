"""Binds frontend/discovery_ui.feature."""

from pytest_bdd import scenarios

from tests.e2e.step_definitions.common_steps import *  # noqa: F401,F403
from tests.e2e.step_definitions.d1_common_steps import *  # noqa: F401,F403
from tests.e2e.step_definitions.d1_discovery_steps import *  # noqa: F401,F403
from tests.e2e.step_definitions.discovery_steps import *  # noqa: F401,F403

scenarios("frontend/discovery_ui.feature")
