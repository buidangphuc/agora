"""Binds security/ar2_order_admin.feature (authz-residuals-2)."""

from pytest_bdd import scenarios

from tests.e2e.step_definitions.ar2_order_steps import *  # noqa: F401,F403

scenarios("../features/security/ar2_order_admin.feature")
