"""Binds tracking/consent.feature and tracking/consent_cls.feature (add-consent-mode, capability consent)."""

from pytest_bdd import scenarios

from tests.e2e.step_definitions.buyer_steps import *  # noqa: F401,F403
from tests.e2e.step_definitions.common_steps import *  # noqa: F401,F403
from tests.e2e.step_definitions.consent_steps import *  # noqa: F401,F403

scenarios("tracking/consent.feature", "tracking/consent_cls.feature")
