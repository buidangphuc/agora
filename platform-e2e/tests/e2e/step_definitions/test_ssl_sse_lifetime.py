"""Binds security/ssl_sse_lifetime.feature (sse-stream-lifetime)."""

from pytest_bdd import scenarios

from tests.e2e.step_definitions.ssl_sse_steps import *  # noqa: F401,F403

scenarios("../features/security/ssl_sse_lifetime.feature")
