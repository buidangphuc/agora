"""Binds ai/llm_breaker.feature (ai-path-resilience / llm-inference-resilience, destructive)."""

from pytest_bdd import scenarios

from tests.e2e.step_definitions.apr_breaker_steps import *  # noqa: F401,F403
from tests.e2e.step_definitions.apr_common_steps import *  # noqa: F401,F403

scenarios("ai/llm_breaker.feature")
