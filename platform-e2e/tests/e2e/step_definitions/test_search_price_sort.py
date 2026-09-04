from pytest_bdd import scenarios

from tests.e2e.step_definitions.auth_steps import *
from tests.e2e.step_definitions.buyer_steps import *
from tests.e2e.step_definitions.search_sort_steps import *

scenarios("../features/buyer/search_price_sort.feature")
