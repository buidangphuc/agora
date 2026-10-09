"""Binds engagement/engagement_facts.feature (engagement-fact-events / engagement-facts)."""

from pytest_bdd import scenarios

from tests.e2e.step_definitions.efe_steps import *  # noqa: F401,F403

scenarios("engagement/engagement_facts.feature")
