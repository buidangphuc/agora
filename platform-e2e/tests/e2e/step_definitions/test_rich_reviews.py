"""Binds engagement/rich_reviews.feature to step definitions.

Every review is seeded through a completed order (review_order_support), so the
verified-purchase badge scenario runs un-marked: UPSTREAM_ORDER_ADDR is wired on the stack.
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
