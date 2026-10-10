"""Binds recommendations/trained_recs_local.feature (serve-trained-recs-locally)."""

from pytest_bdd import scenarios

from tests.e2e.step_definitions.buyer_steps import *  # noqa: F401,F403
from tests.e2e.step_definitions.common_steps import *  # noqa: F401,F403
from tests.e2e.step_definitions.tpr_steps import *  # noqa: F401,F403

scenarios("recommendations/trained_recs_local.feature")
