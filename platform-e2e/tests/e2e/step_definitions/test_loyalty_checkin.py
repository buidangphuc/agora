from pytest_bdd import scenarios

from tests.e2e.step_definitions.common_steps import *  # noqa: F401,F403
from tests.e2e.step_definitions.loyalty_checkin_steps import *  # noqa: F401,F403

scenarios("../features/engagement/loyalty_checkin.feature")
