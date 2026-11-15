"""Binds admin/kyc_queue.feature (change admin-kyc-queue, initiative trust-legal)."""

from pytest_bdd import scenarios

from tests.e2e.step_definitions.common_steps import *  # noqa: F401,F403
from tests.e2e.step_definitions.kyc_queue_steps import *  # noqa: F401,F403

scenarios("admin/kyc_queue.feature")
