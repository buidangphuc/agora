"""Binds the search and frontend dynamic-facet features (add-tag-classifier-filter-enrichment)."""

from pytest_bdd import scenarios

from tests.e2e.step_definitions.discovery_steps import *  # noqa: F401,F403
from tests.e2e.step_definitions.mls_steps import *  # noqa: F401,F403

scenarios("search/search_dynamic_facets.feature")
scenarios("frontend/search_dynamic_facets.feature")
