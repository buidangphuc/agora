"""Binds security/gateway_principal_log_fingerprint.feature to its step definitions."""

from pytest_bdd import scenarios

from tests.e2e.step_definitions.gateway_principal_log_fingerprint_steps import *  # noqa: F401,F403

scenarios("../features/security/gateway_principal_log_fingerprint.feature")
