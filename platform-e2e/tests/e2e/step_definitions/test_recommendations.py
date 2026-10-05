"""Binds recommendations.feature and pipeline_eval_registry.feature to their steps."""

from pytest_bdd import scenarios

from tests.e2e.step_definitions.buyer_steps import *  # noqa: F401,F403
from tests.e2e.step_definitions.common_steps import *  # noqa: F401,F403
from tests.e2e.step_definitions.pdp_streaming_steps import *  # noqa: F401,F403
from tests.e2e.step_definitions.pipeline_eval_registry_steps import *  # noqa: F401,F403
from tests.e2e.step_definitions.recommendations_steps import *  # noqa: F401,F403

scenarios("recommendations/recommendations.feature")
scenarios("recommendations/pipeline_eval_registry.feature")
