from pytest_bdd import scenarios

from tests.e2e.step_definitions.identity_q7_steps import *  # noqa: F403

scenarios("identity/register_referral.feature", "identity/get_my_account.feature")
