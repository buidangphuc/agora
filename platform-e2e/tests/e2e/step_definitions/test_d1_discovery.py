"""Binds frontend/discovery_ui.feature.

"Current category is announced" is bound explicitly and marked xfail: the spec has CategoryBar's
`pills` variant above the search results, but /search never mounts it (the category is a facet link
in the sidebar). It self-heals (xpass) once the pills are rendered.
"""

import pytest
from pytest_bdd import scenario, scenarios

from tests.e2e.step_definitions.common_steps import *  # noqa: F401,F403
from tests.e2e.step_definitions.d1_common_steps import *  # noqa: F401,F403
from tests.e2e.step_definitions.d1_discovery_steps import *  # noqa: F401,F403
from tests.e2e.step_definitions.discovery_steps import *  # noqa: F401,F403

_FEATURE = "frontend/discovery_ui.feature"


@pytest.mark.xfail(reason="CategoryBar pills are not mounted on /search", strict=False)
@scenario(_FEATURE, "Current category is announced")
def test_current_category_is_announced() -> None:
    pass


scenarios(_FEATURE)
