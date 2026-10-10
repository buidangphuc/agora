"""Binds ops/roc_recs_on_cluster.feature (recs-on-cluster)."""

from pytest_bdd import scenarios

from tests.e2e.step_definitions.roc_recs_on_cluster_steps import *  # noqa: F401,F403

scenarios("ops/roc_recs_on_cluster.feature")
