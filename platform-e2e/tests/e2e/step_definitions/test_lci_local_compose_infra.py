"""Binds ops/lci_local_compose_infra.feature (local-compose-infra)."""

from pytest_bdd import scenarios

from tests.e2e.step_definitions.lci_local_compose_infra_steps import *  # noqa: F401,F403

scenarios("ops/lci_local_compose_infra.feature")
