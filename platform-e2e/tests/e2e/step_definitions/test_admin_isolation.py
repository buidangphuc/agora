"""Binds frontend/admin_isolation.feature (team-frontend-admin-app)."""

from pytest_bdd import scenarios

from tests.e2e.step_definitions.admin_isolation_steps import *  # noqa: F401,F403

scenarios("../features/frontend/admin_isolation.feature")
