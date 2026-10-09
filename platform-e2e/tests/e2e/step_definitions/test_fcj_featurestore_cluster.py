"""Binds ops/fcj_featurestore_cluster.feature (featurestore-cluster-job)."""

from pytest_bdd import scenarios

from tests.e2e.step_definitions.fcj_featurestore_cluster_steps import *  # noqa: F401,F403

scenarios("ops/fcj_featurestore_cluster.feature")
