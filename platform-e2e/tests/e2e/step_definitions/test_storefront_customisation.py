from pytest_bdd import scenarios

from tests.e2e.step_definitions.storefront_customisation_steps import *  # noqa: F401,F403

scenarios("../features/domain/storefront_customisation.feature")
