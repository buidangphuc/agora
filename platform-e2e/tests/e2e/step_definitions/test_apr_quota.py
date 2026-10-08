"""Binds ai/llm_quota_and_limits.feature (ai-path-resilience / llm-inference-resilience)."""

from pytest_bdd import scenarios

from tests.e2e.step_definitions.apr_common_steps import *  # noqa: F401,F403
from tests.e2e.step_definitions.apr_quota_steps import *  # noqa: F401,F403

scenarios("ai/llm_quota_and_limits.feature")
