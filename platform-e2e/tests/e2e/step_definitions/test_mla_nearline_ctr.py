"""Binds recommendations/nearline_ctr.feature (add-recsys-nearline-signals, wire-debiased-ctr-ranker-features)."""

from pytest_bdd import scenarios

from tests.e2e.step_definitions.buyer_steps import *  # noqa: F401,F403
from tests.e2e.step_definitions.common_steps import *  # noqa: F401,F403
from tests.e2e.step_definitions.mla_steps import *  # noqa: F401,F403

scenarios("../features/recommendations/nearline_ctr.feature")
