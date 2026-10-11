"""Binds analytics/recs_attribution_hardening.feature (recsys-online-evaluation, rah)."""

from pytest_bdd import scenarios

from tests.e2e.step_definitions.rah_steps import *  # noqa: F401,F403

scenarios("analytics/recs_attribution_hardening.feature")
