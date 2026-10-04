from pytest_bdd import scenarios

from tests.e2e.step_definitions.auth_steps import *
from tests.e2e.step_definitions.buyer_steps import *
from tests.e2e.step_definitions.pdp_reviews_steps import *  # noqa: F401,F403
from tests.e2e.step_definitions.review_ratings_steps import *

scenarios("../features/buyer/review_ratings_filter.feature")
