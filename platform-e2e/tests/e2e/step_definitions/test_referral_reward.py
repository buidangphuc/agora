from pytest_bdd import scenarios

from tests.e2e.step_definitions.oic_order_steps import oic  # noqa: F401
from tests.e2e.step_definitions.referral_reward_steps import *  # noqa: F401,F403

scenarios("referral/referral_reward.feature")
