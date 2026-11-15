"""Binds identity/account_deletion_*.feature (change identity-account-deletion, capability identity-account-deletion)."""

from pytest_bdd import scenarios

from tests.e2e.step_definitions.account_deletion_steps import *  # noqa: F401,F403

scenarios(
    "identity/account_deletion_request.feature",
    "identity/account_deletion_obligations.feature",
    "identity/account_deletion_sessions.feature",
    "identity/account_deletion_gateway.feature",
    "identity/account_deletion_erasure.feature",
)
