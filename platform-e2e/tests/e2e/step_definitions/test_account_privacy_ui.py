"""Binds frontend/account_privacy_ui.feature (change frontend-account-privacy)."""

from pytest_bdd import scenarios

from tests.e2e.step_definitions.account_privacy_steps import *  # noqa: F401,F403

scenarios("frontend/account_privacy_ui.feature")
