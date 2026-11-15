"""Binds seller/seller_terms_consent.feature (change frontend-legal-pages)."""

from pytest_bdd import scenarios

from tests.e2e.step_definitions.common_steps import *  # noqa: F401,F403
from tests.e2e.step_definitions.legal_consent_steps import *  # noqa: F401,F403,E402
from tests.e2e.step_definitions.seller_onboarding_ui_steps import *  # noqa: F401,F403
from tests.e2e.step_definitions.seller_terms_consent_steps import *  # noqa: F401,F403
from tests.e2e.step_definitions.user_enforcement_steps import *  # noqa: F401,F403

scenarios("seller/seller_terms_consent.feature")
