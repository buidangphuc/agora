"""Binds frontend/review_integrity_ui.feature."""

from pytest_bdd import scenarios

from tests.e2e.step_definitions.common_steps import *  # noqa: F401,F403
from tests.e2e.step_definitions.d1_common_steps import *  # noqa: F401,F403
from tests.e2e.step_definitions.review_integrity_steps import *  # noqa: F401,F403

scenarios("frontend/review_integrity_ui.feature")
