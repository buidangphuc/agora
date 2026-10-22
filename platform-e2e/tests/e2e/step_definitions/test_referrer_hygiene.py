"""Binds frontend/referrer_hygiene.feature."""

from pytest_bdd import scenarios

from tests.e2e.step_definitions.common_steps import *  # noqa: F401,F403
from tests.e2e.step_definitions.ga4_datalayer_steps import *  # noqa: F401,F403
from tests.e2e.step_definitions.login_return_steps import *  # noqa: F401,F403
from tests.e2e.step_definitions.referrer_hygiene_steps import *  # noqa: F401,F403

scenarios("../features/frontend/referrer_hygiene.feature")
