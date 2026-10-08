"""Binds tracking/ingest_scrubbing.feature."""

from pytest_bdd import scenarios

from tests.e2e.step_definitions.tii_scrubbing_steps import *  # noqa: F401,F403

scenarios("tracking/ingest_scrubbing.feature")
