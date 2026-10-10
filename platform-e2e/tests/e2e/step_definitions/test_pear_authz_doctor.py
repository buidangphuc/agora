"""Binds ops/repo_doctor.feature (port-edge-authz-residuals / repo-coherence)."""

from pytest_bdd import scenarios

from tests.e2e.step_definitions.pear_authz_doctor_steps import *  # noqa: F401,F403

scenarios("ops/repo_doctor.feature")
