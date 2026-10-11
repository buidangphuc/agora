"""Binds analytics/tracking_quality_report.feature."""

from pytest_bdd import scenarios

from tests.e2e.step_definitions.adq_report_steps import *  # noqa: F401,F403

scenarios("analytics/tracking_quality_report.feature")
