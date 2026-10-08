"""Binds ops/gitops_render.feature (port-edge-authz-residuals / deploy-runtime)."""

from pytest_bdd import scenarios

from tests.e2e.step_definitions.pear_authz_gitops_steps import *  # noqa: F401,F403

scenarios("ops/gitops_render.feature")
