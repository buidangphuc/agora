"""Binds journeys/buyer_full_funnel_journey.feature to its step definitions.

Both scenarios run against the real stack. The recommendations scenario needs the
platform-recsys training job to have run (serve-trained-recs-locally): team-ai then
serves the "Gợi ý cho bạn" row from Qdrant + Redis.
"""

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


@scenario(_FEATURE, "Home page shows AI recommendations with viewable impressions")
def test_home_recommendations_row() -> None:
    pass
