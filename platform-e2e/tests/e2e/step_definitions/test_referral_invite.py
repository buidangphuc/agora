from pytest_bdd import scenarios

from tests.e2e.step_definitions.referral_invite_steps import *  # noqa: F403

scenarios("../features/frontend/referral_invite.feature")
