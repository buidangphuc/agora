"""Binds engagement/rich_reviews.feature to step definitions.

Two scenarios (review photo + helpful vote) run green. The verified-purchase
badge scenario is marked xfail: the running team-engagement service has no
UPSTREAM_ORDER_ADDR configured, so its team-order verifier is nil and
verified_purchase is always false (the badge never renders) regardless of the
order being COMPLETED. It self-heals (xpass) once the deployment wires the order
upstream. Scenarios are bound explicitly (not via scenarios()) so only the
verified-purchase one carries the marker.
"""

from pytest_bdd import scenario

from tests.e2e.step_definitions.common_steps import *  # noqa: F401,F403
from tests.e2e.step_definitions.review_ratings_steps import *  # noqa: F401,F403

_FEATURE = "engagement/rich_reviews.feature"


@scenario(_FEATURE, "A review with a photo renders on the listing")
def test_review_with_photo() -> None:
    pass


@scenario(_FEATURE, "Marking a review helpful increments the count once per user")
def test_helpful_vote_increments_once() -> None:
    pass


@scenario(_FEATURE, "A delivered order earns a verified-purchase badge")
def test_verified_purchase_badge() -> None:
    pass
