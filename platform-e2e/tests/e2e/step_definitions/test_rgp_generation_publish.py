"""Binds recommendations/generation_publish.feature (recsys-generation-publish, destructive)."""

import pytest
from pytest_bdd import scenarios

from tests.e2e.flows.recsys_job_flow import run_recsys_job
from tests.e2e.step_definitions.buyer_steps import *  # noqa: F401,F403
from tests.e2e.step_definitions.common_steps import *  # noqa: F401,F403
from tests.e2e.step_definitions.rgp_steps import *  # noqa: F401,F403

scenarios("recommendations/generation_publish.feature")


@pytest.fixture(scope="module", autouse=True)
def rgp_restore_serving():
    """Restore the serving state after the module: a clean "good A" publish.

    The scenarios rewrite and reset the shared generation state. Teardown re-runs the "good A"
    publish from a clean slate, so what is left serving is one fresh `als-e2e-good-a-*`
    generation (no previous). The data the stack held before (the real tracking dataset) is not
    recreated; the platform-gitops recsys CronJob republishes it.
    """
    yield
    try:
        run_recsys_job([], [{"@fixture": "good_a"}], live=True, reset=True)
    except Exception as exc:  # noqa: BLE001 - cleanup must not mask the scenarios' result
        print(f"[rgp] could not restore the serving state: {exc}")
