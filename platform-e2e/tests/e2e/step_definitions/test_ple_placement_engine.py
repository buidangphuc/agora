"""Binds the non-destructive scenario of recommendations/placement_engine.feature."""

from pytest_bdd import scenario

from tests.e2e.step_definitions.buyer_steps import *  # noqa: F401,F403
from tests.e2e.step_definitions.common_steps import *  # noqa: F401,F403
from tests.e2e.step_definitions.ple_steps import *  # noqa: F401,F403
from tests.e2e.step_definitions.tpr_steps import *  # noqa: F401,F403

_FEATURE = "recommendations/placement_engine.feature"


@scenario(_FEATURE, "Similar items placement retrieves item similarities")
def test_similar_items_placement() -> None:
    pass
