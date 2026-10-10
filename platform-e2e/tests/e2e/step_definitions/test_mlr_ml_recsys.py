"""Binds recommendations/mlr_ml_recsys.feature (nearline signals, drift monitoring, two-tower pipeline)."""

from pytest_bdd import scenarios

from tests.e2e.step_definitions.buyer_steps import *  # noqa: F401,F403
from tests.e2e.step_definitions.common_steps import *  # noqa: F401,F403
from tests.e2e.step_definitions.mlr_steps import *  # noqa: F401,F403

scenarios("recommendations/mlr_ml_recsys.feature")
