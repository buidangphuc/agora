"""Binds buyer/consumer_pages.feature."""

from pytest_bdd import scenarios

from tests.e2e.step_definitions.buyer_steps import *  # noqa: F401,F403
from tests.e2e.step_definitions.common_steps import *  # noqa: F401,F403
from tests.e2e.step_definitions.frontend_steps import *  # noqa: F401,F403

scenarios("buyer/consumer_pages.feature")
