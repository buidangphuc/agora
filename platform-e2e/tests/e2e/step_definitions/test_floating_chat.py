from pytest_bdd import scenarios

from tests.e2e.step_definitions.auth_steps import *
from tests.e2e.step_definitions.buyer_steps import *
from tests.e2e.step_definitions.floating_chat_steps import *
from tests.e2e.step_definitions.logout_steps import *

scenarios("../features/chat/floating_chat.feature")
