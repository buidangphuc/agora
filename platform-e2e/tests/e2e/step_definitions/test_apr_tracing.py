"""Binds ai/llm_tracing.feature (ai-path-resilience / llm-inference-resilience, destructive)."""

from pytest_bdd import scenarios

from tests.e2e.step_definitions.apr_tracing_steps import *  # noqa: F401,F403

scenarios("ai/llm_tracing.feature")
