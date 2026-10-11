"""Binds ops/lcw_local_cluster_image_wiring.feature (local-cluster-image-wiring)."""

from pytest_bdd import scenarios

from tests.e2e.step_definitions.lcw_local_cluster_image_wiring_steps import *  # noqa: F401,F403

scenarios("ops/lcw_local_cluster_image_wiring.feature")
