from pytest_bdd import scenarios

from tests.e2e.step_definitions.experiment_reports_steps import *  # noqa: F401,F403

scenarios("analytics/ai_surface_report.feature")
