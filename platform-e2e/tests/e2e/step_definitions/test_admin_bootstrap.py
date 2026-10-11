"""Binds auth/admin_bootstrap.feature (secure-seller-analytics-and-admin-seed)."""

from pytest_bdd import scenarios

from tests.e2e.step_definitions.c2_admin_bootstrap_steps import *  # noqa: F401,F403

scenarios("../features/auth/admin_bootstrap.feature")
