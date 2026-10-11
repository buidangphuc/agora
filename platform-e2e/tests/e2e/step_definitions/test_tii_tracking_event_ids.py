"""Binds frontend/tracking_event_ids.feature."""

from pytest_bdd import scenarios

from tests.e2e.step_definitions.buyer_steps import *  # noqa: F401,F403
from tests.e2e.step_definitions.common_steps import *  # noqa: F401,F403
from tests.e2e.step_definitions.tii_frontend_steps import *  # noqa: F401,F403

scenarios("../features/frontend/tracking_event_ids.feature")
