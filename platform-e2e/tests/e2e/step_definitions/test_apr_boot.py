"""Binds ops/boot_guard_ai.feature (ai-path-resilience / llm-inference-resilience, destructive)."""

from pytest_bdd import scenarios

from tests.e2e.step_definitions.apr_boot_steps import *  # noqa: F401,F403

scenarios("ops/boot_guard_ai.feature")
