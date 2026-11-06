from pytest_bdd import scenarios

from tests.e2e.step_definitions.new_user_voucher_steps import *  # noqa: F401,F403

scenarios(
    "promotion/new_user_vouchers.feature",
    "promotion/voucher_per_user.feature",
    "promotion/my_vouchers.feature",
)
