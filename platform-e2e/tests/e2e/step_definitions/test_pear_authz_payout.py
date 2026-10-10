"""Binds payment/payout_scope.feature (port-edge-authz-residuals / payment-access-control)."""

from pytest_bdd import scenarios

from tests.e2e.step_definitions.pear_authz_payout_steps import *  # noqa: F401,F403

scenarios("payment/payout_scope.feature")
