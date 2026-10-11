"""Binds security/ar2_stream_lifetime.feature (authz-residuals-2)."""

from pytest_bdd import scenarios

from tests.e2e.step_definitions.ar2_stream_steps import *  # noqa: F401,F403

scenarios("../features/security/ar2_stream_lifetime.feature")
