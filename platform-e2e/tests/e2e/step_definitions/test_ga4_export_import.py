"""Binds tracking/ga4_export_import.feature (add-ga4-export-import, capability ga4-export-import)."""

from pytest_bdd import scenarios

from tests.e2e.step_definitions.adq_report_steps import *  # noqa: F401,F403
from tests.e2e.step_definitions.common_steps import *  # noqa: F401,F403
from tests.e2e.step_definitions.consent_contract_steps import *  # noqa: F401,F403
from tests.e2e.step_definitions.ga4_export_import_steps import *  # noqa: F401,F403

scenarios("tracking/ga4_export_import.feature")
