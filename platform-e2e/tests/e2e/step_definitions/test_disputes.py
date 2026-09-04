from pytest_bdd import scenarios

from tests.e2e.step_definitions.common_steps import *  # noqa: F403
from tests.e2e.step_definitions.group_b_steps import *  # noqa: F403

scenarios("engagement/disputes.feature")
