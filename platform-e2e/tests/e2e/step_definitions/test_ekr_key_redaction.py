"""Binds engagement/consumer_key_redaction.feature to its step definitions."""

from pytest_bdd import scenarios

from tests.e2e.step_definitions.ekr_steps import *  # noqa: F401,F403

scenarios("../features/engagement/consumer_key_redaction.feature")
