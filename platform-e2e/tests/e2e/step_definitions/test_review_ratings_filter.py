from pytest_bdd import scenarios

from tests.e2e.step_definitions.auth_steps import *
from tests.e2e.step_definitions.buyer_steps import *
from tests.e2e.step_definitions.review_ratings_steps import *

scenarios("../features/buyer/review_ratings_filter.feature")
