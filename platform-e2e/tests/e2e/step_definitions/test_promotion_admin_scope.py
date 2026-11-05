from pytest_bdd import scenarios

from tests.e2e.step_definitions.promotion_admin_scope_steps import *  # noqa: F401,F403

scenarios("../features/security/promotion_admin_scope.feature")
