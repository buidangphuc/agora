"""Binds ai/tag_classifier_taxonomy.feature (change add-tag-classifier-filter-enrichment)."""

from pytest_bdd import scenarios

from tests.e2e.step_definitions.tax_steps import *  # noqa: F401,F403

scenarios("../features/ai/tag_classifier_taxonomy.feature")
