"""Binds journeys/buyer_full_funnel_journey.feature to its step definitions.

The funnel scenario runs green against the real stack. The recommendations scenario is bound
explicitly and marked xfail(strict) on a documented gap:
  * team-ai runs with RECS_ENABLED=false (docker-compose.services.yaml), so the gateway's
    RecommendationService/Recommend answers unimplemented and team-frontend renders no
    "Gợi ý cho bạn" row (RecommendationsRow.tsx returns null on an empty list), hence no
    row impressions either.
strict=True so the marker is removed (XPASS fails) once recommendations are served.
"""

import pytest
from pytest_bdd import scenario

from tests.e2e.step_definitions.buyer_steps import *  # noqa: F401,F403
from tests.e2e.step_definitions.common_steps import *  # noqa: F401,F403
from tests.e2e.step_definitions.journey_steps import *  # noqa: F401,F403
from tests.e2e.step_definitions.promo_steps import *  # noqa: F401,F403

_FEATURE = "journeys/buyer_full_funnel_journey.feature"


@scenario(
    _FEATURE,
    "Complete buyer full funnel journey from search to checkout and GA4 purchase",
)
def test_buyer_full_funnel_journey() -> None:
    pass


@pytest.mark.xfail(
    strict=True,
    reason="backend gap: RECS_ENABLED=false in team-ai, so Recommend is unimplemented at the "
    "gateway and the home 'Gợi ý cho bạn' row (and its impressions) is never rendered",
)
@scenario(_FEATURE, "Home page shows AI recommendations with viewable impressions")
def test_home_recommendations_row() -> None:
    pass
