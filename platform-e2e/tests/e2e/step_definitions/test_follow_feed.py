"""Binds engagement/follow_feed.feature.

API-level: the follow feed is fed asynchronously from listing.events, so the
Then steps poll ListFollowedListings with a bounded timeout.
"""

from pytest_bdd import scenarios

from tests.e2e.step_definitions.common_steps import *  # noqa: F401,F403
from tests.e2e.step_definitions.follow_feed_steps import *  # noqa: F401,F403

scenarios("../features/engagement/follow_feed.feature")
