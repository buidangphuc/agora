"""Binds ai/llm_eval_gate.feature (ai-path-resilience / llm-inference-resilience)."""

from pytest_bdd import scenarios

from tests.e2e.step_definitions.apr_eval_steps import *  # noqa: F401,F403

scenarios("ai/llm_eval_gate.feature")
