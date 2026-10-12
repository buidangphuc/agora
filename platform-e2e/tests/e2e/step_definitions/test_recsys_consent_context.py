"""Binds recommendations/recsys_consent_context.feature (recsys-consent-and-context, capability recommendations)."""

from pytest_bdd import scenarios

from tests.e2e.step_definitions.buyer_steps import *  # noqa: F401,F403
from tests.e2e.step_definitions.common_steps import *  # noqa: F401,F403
from tests.e2e.step_definitions.ga4_export_import_steps import *  # noqa: F401,F403
from tests.e2e.step_definitions.recsys_consent_context_steps import *  # noqa: F401,F403

scenarios("../features/recommendations/recsys_consent_context.feature")
