"""Binds engagement/listing_rating_events.feature (engagement-listing-rating-events)."""

from pytest_bdd import scenarios

from tests.e2e.step_definitions.listing_rating_events_steps import *  # noqa: F401,F403

scenarios("engagement/listing_rating_events.feature")
